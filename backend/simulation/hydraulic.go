package simulation

import (
	"context"
	"karez-system/db"
	"karez-system/models"
	"math"
	"time"
)

type HydraulicSimulator struct {
	database *db.Database
}

func New(database *db.Database) *HydraulicSimulator {
	return &HydraulicSimulator{database: database}
}

type ChannelParams struct {
	Width           float64
	Height          float64
	Slope           float64
	RoughnessCoeff  float64
	SeepageCoeff    float64
	Length          float64
	Temperature     float64
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

func (s *HydraulicSimulator) SimulateSegment(params ChannelParams, inflowRate float64) *SimulationOutput {
	output := &SimulationOutput{
		InflowRate: inflowRate,
	}

	waterDepth := s.calculateNormalDepth(params, inflowRate)
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
	output.OutflowRate = math.Max(0, inflowRate - output.TotalLoss)

	headLoss := s.calculateHeadLoss(params, velocity, hydraulicRadius)
	output.HeadLoss = headLoss

	return output
}

func (s *HydraulicSimulator) calculateNormalDepth(params ChannelParams, flowRate float64) float64 {
	if flowRate <= 0 {
		return 0
	}

	minDepth := 0.01
	maxDepth := params.Height
	tolerance := 0.0001

	for i := 0; i < 100; i++ {
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
	wettedPerimeter := params.Width + 2*depth
	seepageVelocity := params.SeepageCoeff * depth / params.Width
	seepageRate := wettedPerimeter * params.Length * seepageVelocity
	return seepageRate
}

func (s *HydraulicSimulator) calculateEvaporationLoss(params ChannelParams, depth float64) float64 {
	if depth <= 0 {
		return 0
	}
	surfaceArea := params.Width * params.Length
	evaporationRate := 0.005 * surfaceArea / 3600.0
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

func (s *HydraulicSimulator) RunFullSimulation(ctx context.Context, karezID int) error {
	segments, err := s.database.GetAqueductSegments(ctx, karezID)
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		return nil
	}

	inflowRate := 0.08

	currentFlow := inflowRate
	for i, segment := range segments {
		params := ChannelParams{
			Width:          segment.Width,
			Height:         segment.Height,
			Slope:          segment.Slope,
			RoughnessCoeff: segment.RoughnessCoeff,
			SeepageCoeff:   segment.SeepageCoeff,
			Length:         segment.Length,
			Temperature:    25.0,
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
			return err
		}

		currentFlow = result.OutflowRate

		if i < len(segments)-1 {
			currentFlow *= 1.02
		}
	}

	return nil
}

func (s *HydraulicSimulator) EstimateSedimentationRisk(velocity float64) float64 {
	if velocity >= 0.8 {
		return 0.0
	} else if velocity >= 0.5 {
		return 0.3
	} else if velocity >= 0.3 {
		return 0.6
	} else {
		return 0.9
	}
}
