package hydraulicsim

import (
	"context"
	"fmt"
	"karez-system/config"
	"karez-system/db"
	"karez-system/models"
	"log"
	"math"
	"time"
)

type SimRequest struct {
	KarezID  int
	ForceRun bool
}

type SimResult struct {
	KarezID   int
	SegmentID int
	Result    *SimulationOutput
	Timestamp time.Time
}

type ChannelParams struct {
	Width                float64
	Height               float64
	Slope                float64
	RoughnessCoeff       float64
	SeepageCoeff         float64
	SoilType             string
	SoilCorrectionFactor float64
	Length               float64
	Temperature          float64
}

type SimulationOutput struct {
	InflowRate      float64
	OutflowRate     float64
	SeepageLoss     float64
	EvaporationLoss float64
	TotalLoss       float64
	WaterDepth      float64
	FlowVelocity    float64
	ReynoldsNumber  float64
	FroudeNumber    float64
	HeadLoss        float64
}

type HydraulicSimulator struct {
	cfg              *config.Config
	database         *db.Database
	inputChan        chan SimRequest
	outputChan       chan<- SimResult
	soilPermeability map[string]config.SoilConfig
}

func New(cfg *config.Config, database *db.Database,
	inputChan chan SimRequest, outputChan chan<- SimResult) *HydraulicSimulator {

	return &HydraulicSimulator{
		cfg:              cfg,
		database:         database,
		inputChan:        inputChan,
		outputChan:       outputChan,
		soilPermeability: cfg.HydraulicParams.SoilPermeability,
	}
}

func (s *HydraulicSimulator) Start(ctx context.Context) {
	go s.run(ctx)
	log.Println("Hydraulic Simulator: started")
}

func (s *HydraulicSimulator) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Hydraulic Simulator: stopped")
			return
		case req := <-s.inputChan:
			s.runFullSimulation(ctx, req.KarezID)
		}
	}
}

func (s *HydraulicSimulator) runFullSimulation(ctx context.Context, karezID int) {
	segments, err := s.database.GetAqueductSegments(ctx, karezID)
	if err != nil {
		log.Printf("Hydraulic Simulator: failed to get segments for karez %d: %v", karezID, err)
		return
	}

	if len(segments) == 0 {
		return
	}

	defaults := s.cfg.HydraulicParams.DefaultChannel
	simCfg := s.cfg.HydraulicParams.Simulation
	currentFlow := simCfg.DefaultInflowRate

	for i, segment := range segments {
		soilType := segment.SoilType
		if soilType == "" {
			soilType = defaults.DefaultSoilType
		}
		soilCorrection := segment.SoilCorrectionFactor
		if soilCorrection == 0 {
			soilCorrection = defaults.DefaultSoilCorrection
		}

		params := ChannelParams{
			Width:                segment.Width,
			Height:               segment.Height,
			Slope:                segment.Slope,
			RoughnessCoeff:       segment.RoughnessCoeff,
			SeepageCoeff:         segment.SeepageCoeff,
			SoilType:             soilType,
			SoilCorrectionFactor: soilCorrection,
			Length:               segment.Length,
			Temperature:          defaults.DefaultTemperature,
		}

		result := s.SimulateSegment(params, currentFlow)

		simResult := &models.SimulationResult{
			Time:            time.Now(),
			KarezID:         karezID,
			SegmentID:       segment.ID,
			SimulationType:  "hydraulic",
			InflowRate:      result.InflowRate,
			OutflowRate:     result.OutflowRate,
			SeepageLoss:     result.SeepageLoss,
			EvaporationLoss: result.EvaporationLoss,
			TotalLoss:       result.TotalLoss,
			WaterDepth:      result.WaterDepth,
			FlowVelocity:    result.FlowVelocity,
			ReynoldsNumber:  result.ReynoldsNumber,
			FroudeNumber:    result.FroudeNumber,
			HeadLoss:        result.HeadLoss,
		}

		if err := s.database.InsertSimulationResult(ctx, simResult); err != nil {
			log.Printf("Hydraulic Simulator: failed to insert result for segment %d: %v", segment.ID, err)
		}

		simOutput := *result
		select {
		case s.outputChan <- SimResult{
			KarezID:   karezID,
			SegmentID: segment.ID,
			Result:    &simOutput,
			Timestamp: time.Now(),
		}:
		default:
			log.Printf("Hydraulic Simulator: output channel full, skipping segment %d", segment.ID)
		}

		currentFlow = result.OutflowRate
		if i < len(segments)-1 {
			currentFlow *= simCfg.DownstreamInflowGain
		}
	}

	log.Printf("Hydraulic Simulator: completed simulation for karez %d", karezID)
}

func (s *HydraulicSimulator) SimulateSegment(params ChannelParams, inflowRate float64) *SimulationOutput {
	output := &SimulationOutput{
		InflowRate: inflowRate,
	}

	simCfg := s.cfg.HydraulicParams.Simulation
	waterDepth := s.calculateNormalDepth(params, inflowRate, simCfg.MaxIterations, simCfg.BisectionTolerance)
	output.WaterDepth = waterDepth

	velocity := s.calculateVelocity(params, waterDepth)
	output.FlowVelocity = velocity

	hydraulicRadius := s.calculateHydraulicRadius(params, waterDepth)
	reynoldsNumber := s.calculateReynoldsNumber(velocity, hydraulicRadius, params.Temperature)
	output.ReynoldsNumber = reynoldsNumber

	froudeNumber := s.calculateFroudeNumber(velocity, waterDepth)
	output.FroudeNumber = froudeNumber

	seepageLoss := s.calculateSeepageLoss(params, waterDepth)
	output.SeepageLoss = seepageLoss

	evaporationLoss := s.calculateEvaporationLoss(params, waterDepth)
	output.EvaporationLoss = evaporationLoss

	output.TotalLoss = seepageLoss + evaporationLoss
	output.OutflowRate = math.Max(0, inflowRate-output.TotalLoss)

	headLoss := s.calculateHeadLoss(params, velocity, hydraulicRadius)
	output.HeadLoss = headLoss

	return output
}

func (s *HydraulicSimulator) calculateNormalDepth(params ChannelParams, flowRate float64, maxIter int, tolerance float64) float64 {
	if flowRate <= 0 {
		return 0
	}

	minDepth := 0.01
	maxDepth := params.Height

	for i := 0; i < maxIter; i++ {
		midDepth := (minDepth + maxDepth) / 2
		computedFlow := s.calculateFlowByDepth(params, midDepth)

		if math.Abs(computedFlow-flowRate) < tolerance {
			return midDepth
		}

		if computedFlow < flowRate {
			minDepth = midDepth
		} else {
			maxDepth = midDepth
		}
	}

	return (minDepth + maxDepth) / 2
}

func (s *HydraulicSimulator) calculateFlowByDepth(params ChannelParams, depth float64) float64 {
	area := params.Width * depth
	wettedPerimeter := params.Width + 2*depth
	hydraulicRadius := area / wettedPerimeter
	flowVelocity := (1.0 / params.RoughnessCoeff) * math.Pow(hydraulicRadius, 2.0/3.0) * math.Sqrt(params.Slope)
	return area * flowVelocity
}

func (s *HydraulicSimulator) calculateVelocity(params ChannelParams, depth float64) float64 {
	if depth <= 0 {
		return 0
	}
	area := params.Width * depth
	wettedPerimeter := params.Width + 2*depth
	hydraulicRadius := area / wettedPerimeter
	return (1.0 / params.RoughnessCoeff) * math.Pow(hydraulicRadius, 2.0/3.0) * math.Sqrt(params.Slope)
}

func (s *HydraulicSimulator) calculateHydraulicRadius(params ChannelParams, depth float64) float64 {
	if depth <= 0 {
		return 0
	}
	area := params.Width * depth
	wettedPerimeter := params.Width + 2*depth
	return area / wettedPerimeter
}

func (s *HydraulicSimulator) calculateReynoldsNumber(velocity, hydraulicRadius, temperature float64) float64 {
	if velocity <= 0 || hydraulicRadius <= 0 {
		return 0
	}
	kinematicViscosity := s.getKinematicViscosity(temperature)
	return velocity * 4 * hydraulicRadius / kinematicViscosity
}

func (s *HydraulicSimulator) getKinematicViscosity(temperature float64) float64 {
	t := temperature
	if t < 0 {
		t = 0
	}
	if t > 40 {
		t = 40
	}
	return (1.775 - 0.057*t + 0.0011*t*t) * 1e-6
}

func (s *HydraulicSimulator) calculateFroudeNumber(velocity, depth float64) float64 {
	if depth <= 0 || velocity <= 0 {
		return 0
	}
	gravity := 9.81
	hydraulicDepth := depth
	return velocity / math.Sqrt(gravity*hydraulicDepth)
}

func (s *HydraulicSimulator) calculateSeepageLoss(params ChannelParams, depth float64) float64 {
	if depth <= 0 {
		return 0
	}

	effectiveSeepageCoeff := s.getEffectiveSeepageCoeff(params)
	wettedPerimeter := params.Width + 2*depth
	seepageVelocity := effectiveSeepageCoeff * depth / params.Width
	seepageRate := wettedPerimeter * params.Length * seepageVelocity
	return seepageRate
}

func (s *HydraulicSimulator) getEffectiveSeepageCoeff(params ChannelParams) float64 {
	baseCoeff := params.SeepageCoeff
	soilEntry, exists := s.soilPermeability[params.SoilType]

	if !exists {
		if params.SoilCorrectionFactor > 0 {
			return baseCoeff * params.SoilCorrectionFactor
		}
		return baseCoeff
	}

	defaults := s.cfg.HydraulicParams.DefaultChannel
	if baseCoeff <= 0 || baseCoeff == defaults.SeepageCoeff {
		return soilEntry.BasePermeability * soilEntry.CorrectionFactor
	}

	if params.SoilCorrectionFactor > 0 {
		return baseCoeff * params.SoilCorrectionFactor
	}

	return baseCoeff * soilEntry.CorrectionFactor
}

func (s *HydraulicSimulator) calculateEvaporationLoss(params ChannelParams, depth float64) float64 {
	if depth <= 0 {
		return 0
	}
	evapCfg := s.cfg.HydraulicParams.Evaporation
	surfaceArea := params.Width * params.Length
	evaporationRate := evapCfg.BaseRate * surfaceArea / 3600.0
	return evaporationRate
}

func (s *HydraulicSimulator) calculateHeadLoss(params ChannelParams, velocity, hydraulicRadius float64) float64 {
	if velocity <= 0 || hydraulicRadius <= 0 {
		return 0
	}
	gravity := 9.81
	darcyFriction := 8 * params.RoughnessCoeff * params.RoughnessCoeff * gravity /
		math.Pow(hydraulicRadius, 1.0/3.0)
	return darcyFriction * params.Length * velocity * velocity / (8 * gravity * hydraulicRadius)
}

func (s *HydraulicSimulator) EstimateSedimentationRisk(velocity float64) float64 {
	thresh := s.cfg.HydraulicParams.SedimentationThresholds
	if velocity >= thresh.LowRiskVelocity {
		return thresh.LowRiskScore
	} else if velocity >= thresh.MediumRiskVelocity {
		return thresh.MediumRiskScore
	} else if velocity >= thresh.HighRiskVelocity {
		return thresh.HighRiskScore
	} else {
		return thresh.CriticalRiskScore
	}
}

func (s *HydraulicSimulator) RunFullSimulation(ctx context.Context, karezID int) error {
	s.runFullSimulation(ctx, karezID)
	return nil
}

func (s *HydraulicSimulator) SimulateSegmentDirect(params ChannelParams, inflowRate float64) *SimulationOutput {
	return s.SimulateSegment(params, inflowRate)
}

func (s *HydraulicSimulator) RequestSimulation(karezID int) error {
	select {
	case s.inputChan <- SimRequest{KarezID: karezID, ForceRun: true}:
		return nil
	default:
		return fmt.Errorf("simulation channel full")
	}
}
