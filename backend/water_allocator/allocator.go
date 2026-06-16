package wateralloc

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

type AllocationRequest struct {
	KarezID           int
	TotalAvailableFlow float64
	ForceRun          bool
}

type AllocationResponse struct {
	KarezID        int
	Allocations    map[int]float64
	ObjectiveValue float64
	TotalAllocated float64
	DemandMet      map[int]float64
	Timestamp      time.Time
}

type OasisDemand struct {
	ID            int
	Name          string
	BranchID      int
	Demand        float64
	Priority      int
	MaxAllocation float64
	MinAllocation float64
}

type WaterAllocator struct {
	cfg        *config.Config
	database   *db.Database
	inputChan  chan AllocationRequest
	outputChan chan<- AllocationResponse
}

func New(cfg *config.Config, database *db.Database,
	inputChan chan AllocationRequest, outputChan chan AllocationResponse) *WaterAllocator {
	return &WaterAllocator{
		cfg:        cfg,
		database:   database,
		inputChan:  inputChan,
		outputChan: outputChan,
	}
}

func (wa *WaterAllocator) Start(ctx context.Context) {
	go wa.run(ctx)
	log.Println("Water Allocator: started")
}

func (wa *WaterAllocator) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Water Allocator: stopped")
			return
		case req := <-wa.inputChan:
			solution, err := wa.OptimizeAllocation(ctx, req.KarezID, req.TotalAvailableFlow)
			if err != nil {
				log.Printf("Water Allocator: optimization failed for karez %d: %v", req.KarezID, err)
				continue
			}
			if wa.outputChan != nil {
				select {
				case wa.outputChan <- *solution:
				default:
					log.Printf("Water Allocator: output channel full for karez %d", req.KarezID)
				}
			}
		}
	}
}

func (wa *WaterAllocator) OptimizeAllocation(ctx context.Context, karezID int, totalAvailableFlow float64) (*AllocationResponse, error) {
	oases, err := wa.database.GetOases(ctx, karezID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oases: %w", err)
	}

	branches, err := wa.database.GetBranchChannels(ctx, karezID)
	if err != nil {
		return nil, fmt.Errorf("failed to get branch channels: %w", err)
	}

	branchMaxFlow := make(map[int]float64)
	for _, b := range branches {
		branchMaxFlow[b.ID] = b.MaxFlow
	}

	oasisDefaults := wa.cfg.AgricultureDemand.OasisDefaults
	var demands []OasisDemand
	for _, o := range oases {
		dailyDemand := o.DailyWaterDemand / 86400.0
		demands = append(demands, OasisDemand{
			ID:            o.ID,
			Name:          o.Name,
			BranchID:      o.BranchChannelID,
			Demand:        dailyDemand,
			Priority:      o.Priority,
			MaxAllocation: branchMaxFlow[o.BranchChannelID],
			MinAllocation: dailyDemand * oasisDefaults.DefaultMinAllocationRatio,
		})
	}

	solution := wa.solveLinearProgramming(demands, totalAvailableFlow)

	now := time.Now()
	for _, oasis := range oases {
		allocated := solution.Allocations[oasis.ID]
		demandMet := 0.0
		dailyDemand := oasis.DailyWaterDemand / 86400.0
		if dailyDemand > 0 {
			demandMet = allocated / dailyDemand
		}

		allocRatio := 0.0
		if totalAvailableFlow > 0 {
			allocRatio = allocated / totalAvailableFlow
		}

		result := &models.AllocationResult{
			Time:               now,
			KarezID:            karezID,
			BranchChannelID:    oasis.BranchChannelID,
			OasisID:            oasis.ID,
			AllocatedFlow:      allocated,
			AllocationRatio:    allocRatio,
			DemandMet:          demandMet,
			OptimizationMethod: "linear_programming",
			ObjectiveValue:     solution.ObjectiveValue,
		}

		if err := wa.database.InsertAllocationResult(ctx, result); err != nil {
			return nil, err
		}
	}

	solution.KarezID = karezID
	solution.Timestamp = now
	return solution, nil
}

func (wa *WaterAllocator) solveLinearProgramming(oases []OasisDemand, totalAvailableFlow float64) *AllocationResponse {
	n := len(oases)

	solution := &AllocationResponse{
		Allocations: make(map[int]float64),
		DemandMet:   make(map[int]float64),
	}

	if n == 0 || totalAvailableFlow <= 0 {
		for _, o := range oases {
			solution.Allocations[o.ID] = 0
			solution.DemandMet[o.ID] = 0
		}
		solution.ObjectiveValue = 0
		return solution
	}

	algoCfg := wa.cfg.AgricultureDemand.AllocationAlgorithm

	hasMinConstraint := false
	for _, o := range oases {
		if o.MinAllocation > 0 {
			hasMinConstraint = true
			break
		}
	}

	numSlack := 0
	if hasMinConstraint {
		numSlack = n
	}

	c := make([]float64, n+numSlack)
	for i, o := range oases {
		weight := algoCfg.PriorityWeightBase / float64(o.Priority)
		c[i] = weight * o.Demand
	}

	bigM := algoCfg.BigMPenalty
	for i := 0; i < numSlack; i++ {
		c[n+i] = -bigM
	}

	A := make([][]float64, 0)
	b := make([]float64, 0)

	totalRow := make([]float64, n+numSlack)
	for i := 0; i < n; i++ {
		totalRow[i] = 1.0
	}
	A = append(A, totalRow)
	b = append(b, totalAvailableFlow)

	for i, o := range oases {
		row := make([]float64, n+numSlack)
		row[i] = 1.0
		A = append(A, row)
		b = append(b, math.Min(o.MaxAllocation, o.Demand))
	}

	for i, o := range oases {
		row := make([]float64, n+numSlack)
		row[i] = -1.0
		if hasMinConstraint && o.MinAllocation > 0 {
			row[n+i] = 1.0
		}
		A = append(A, row)
		if hasMinConstraint && o.MinAllocation > 0 {
			b = append(b, 0)
		} else {
			b = append(b, -o.MinAllocation)
		}
	}

	lp := NewSimplexSolver(c, A, b, algoCfg)
	lp.SetNumOriginalVars(n)
	optVal, x := lp.Solve()

	totalAllocated := 0.0
	for i, o := range oases {
		allocated := math.Max(0, math.Min(o.Demand, x[i]))
		solution.Allocations[o.ID] = allocated
		totalAllocated += allocated

		if o.Demand > 0 {
			solution.DemandMet[o.ID] = allocated / o.Demand
		}
	}

	solution.ObjectiveValue = optVal
	solution.TotalAllocated = totalAllocated

	return solution
}

type SimplexSolver struct {
	c               []float64
	A               [][]float64
	b               []float64
	numVars         int
	numOriginalVars int
	numConstraints  int
	tableau         [][]float64
	basicVars       []int
	nonBasicVars    []int
	algoCfg         config.AllocationAlgoConfig
}

func NewSimplexSolver(c []float64, A [][]float64, b []float64, algoCfg config.AllocationAlgoConfig) *SimplexSolver {
	numVars := len(c)
	numConstraints := len(b)

	tableau := make([][]float64, numConstraints+1)
	for i := range tableau {
		tableau[i] = make([]float64, numVars+numConstraints+1)
	}

	for i := 0; i < numConstraints; i++ {
		for j := 0; j < numVars; j++ {
			tableau[i][j] = A[i][j]
		}
		tableau[i][numVars+i] = 1.0
		tableau[i][numVars+numConstraints] = b[i]
	}

	for j := 0; j < numVars; j++ {
		tableau[numConstraints][j] = -c[j]
	}

	basicVars := make([]int, numConstraints)
	for i := range basicVars {
		basicVars[i] = numVars + i
	}

	nonBasicVars := make([]int, numVars)
	for i := range nonBasicVars {
		nonBasicVars[i] = i
	}

	return &SimplexSolver{
		c:               c,
		A:               A,
		b:               b,
		numVars:         numVars,
		numOriginalVars: numVars,
		numConstraints:  numConstraints,
		tableau:         tableau,
		basicVars:       basicVars,
		nonBasicVars:    nonBasicVars,
		algoCfg:         algoCfg,
	}
}

func (s *SimplexSolver) SetNumOriginalVars(n int) {
	s.numOriginalVars = n
}

func (s *SimplexSolver) Solve() (float64, []float64) {
	maxIterations := s.algoCfg.SimplexMaxIter
	epsilon := s.algoCfg.Epsilon

	for iter := 0; iter < maxIterations; iter++ {
		entering := -1
		minCoeff := -epsilon
		for j := 0; j < s.numVars+s.numConstraints; j++ {
			if s.tableau[s.numConstraints][j] < minCoeff {
				minCoeff = s.tableau[s.numConstraints][j]
				entering = j
			}
		}

		if entering == -1 {
			break
		}

		leaving := -1
		minRatio := math.Inf(1)
		for i := 0; i < s.numConstraints; i++ {
			if s.tableau[i][entering] > epsilon {
				ratio := s.tableau[i][s.numVars+s.numConstraints] / s.tableau[i][entering]
				if ratio < minRatio {
					minRatio = ratio
					leaving = i
				}
			}
		}

		if leaving == -1 {
			break
		}

		s.pivot(leaving, entering)

		s.basicVars[leaving], s.nonBasicVars[entering%s.numVars] = entering, s.basicVars[leaving]
	}

	x := make([]float64, s.numOriginalVars)
	for i := 0; i < s.numConstraints; i++ {
		if s.basicVars[i] < s.numOriginalVars {
			x[s.basicVars[i]] = s.tableau[i][s.numVars+s.numConstraints]
		}
	}

	optVal := s.tableau[s.numConstraints][s.numVars+s.numConstraints]
	return optVal, x
}

func (s *SimplexSolver) pivot(row, col int) {
	pivotVal := s.tableau[row][col]

	for j := 0; j < s.numVars+s.numConstraints+1; j++ {
		s.tableau[row][j] /= pivotVal
	}

	for i := 0; i < s.numConstraints+1; i++ {
		if i != row {
			factor := s.tableau[i][col]
			for j := 0; j < s.numVars+s.numConstraints+1; j++ {
				s.tableau[i][j] -= factor * s.tableau[row][j]
			}
		}
	}
}

func (wa *WaterAllocator) RequestAllocation(karezID int, totalFlow float64) error {
	select {
	case wa.inputChan <- AllocationRequest{
		KarezID:           karezID,
		TotalAvailableFlow: totalFlow,
		ForceRun:          true,
	}:
		return nil
	default:
		return fmt.Errorf("allocation channel full")
	}
}
