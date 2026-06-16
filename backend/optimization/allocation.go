package optimization

import (
	"context"
	"fmt"
	"karez-system/db"
	"karez-system/models"
	"math"
	"time"
)

type WaterAllocator struct {
	database *db.Database
}

type AllocationProblem struct {
	TotalAvailableFlow float64
	Oases              []OasisDemand
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

type AllocationSolution struct {
	Allocations    map[int]float64
	ObjectiveValue float64
	TotalAllocated float64
	DemandMet      map[int]float64
}

func New(database *db.Database) *WaterAllocator {
	return &WaterAllocator{database: database}
}

func (wa *WaterAllocator) OptimizeAllocation(ctx context.Context, karezID int, totalAvailableFlow float64) (*AllocationSolution, error) {
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

	var demands []OasisDemand
	for _, o := range oases {
		demands = append(demands, OasisDemand{
			ID:            o.ID,
			Name:          o.Name,
			BranchID:      o.BranchChannelID,
			Demand:        o.DailyWaterDemand / 86400.0,
			Priority:      o.Priority,
			MaxAllocation: branchMaxFlow[o.BranchChannelID],
			MinAllocation: 0,
		})
	}

	problem := AllocationProblem{
		TotalAvailableFlow: totalAvailableFlow,
		Oases:              demands,
	}

	solution := wa.solveLinearProgramming(problem)

	now := time.Now()
	for _, oasis := range oases {
		allocated := solution.Allocations[oasis.ID]
		demandMet := 0.0
		if oasis.DailyWaterDemand/86400.0 > 0 {
			demandMet = allocated / (oasis.DailyWaterDemand / 86400.0)
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

	return solution, nil
}

func (wa *WaterAllocator) solveLinearProgramming(problem AllocationProblem) *AllocationSolution {
	n := len(problem.Oases)

	solution := &AllocationSolution{
		Allocations: make(map[int]float64),
		DemandMet:   make(map[int]float64),
	}

	if n == 0 || problem.TotalAvailableFlow <= 0 {
		for _, o := range problem.Oases {
			solution.Allocations[o.ID] = 0
			solution.DemandMet[o.ID] = 0
		}
		solution.ObjectiveValue = 0
		return solution
	}

	c := make([]float64, n)
	for i, o := range problem.Oases {
		weight := 1.0 / float64(o.Priority)
		c[i] = weight * o.Demand
	}

	A := make([][]float64, 0)
	b := make([]float64, 0)

	totalRow := make([]float64, n)
	for i := range totalRow {
		totalRow[i] = 1.0
	}
	A = append(A, totalRow)
	b = append(b, problem.TotalAvailableFlow)

	for i, o := range problem.Oases {
		row := make([]float64, n)
		row[i] = 1.0
		A = append(A, row)
		b = append(b, math.Min(o.MaxAllocation, o.Demand))
	}

	for i, o := range problem.Oases {
		row := make([]float64, n)
		row[i] = -1.0
		A = append(A, row)
		b = append(b, -o.MinAllocation)
	}

	lp := NewSimplexSolver(c, A, b)
	optVal, x := lp.Solve()

	totalAllocated := 0.0
	for i, o := range problem.Oases {
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
	c     []float64
	A     [][]float64
	b     []float64
	numVars int
	numConstraints int
	tableau [][]float64
	basicVars []int
	nonBasicVars []int
}

func NewSimplexSolver(c []float64, A [][]float64, b []float64) *SimplexSolver {
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
		c: c,
		A: A,
		b: b,
		numVars: numVars,
		numConstraints: numConstraints,
		tableau: tableau,
		basicVars: basicVars,
		nonBasicVars: nonBasicVars,
	}
}

func (s *SimplexSolver) Solve() (float64, []float64) {
	maxIterations := 1000
	epsilon := 1e-9

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

	x := make([]float64, s.numVars)
	for i := 0; i < s.numConstraints; i++ {
		if s.basicVars[i] < s.numVars {
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

func (wa *WaterAllocator) ProportionalAllocation(problem AllocationProblem) *AllocationSolution {
	solution := &AllocationSolution{
		Allocations: make(map[int]float64),
		DemandMet:   make(map[int]float64),
	}

	totalDemand := 0.0
	for _, o := range problem.Oases {
		totalDemand += o.Demand
	}

	if totalDemand <= 0 {
		for _, o := range problem.Oases {
			solution.Allocations[o.ID] = 0
			solution.DemandMet[o.ID] = 0
		}
		return solution
	}

	ratio := problem.TotalAvailableFlow / totalDemand
	totalAllocated := 0.0

	for _, o := range problem.Oases {
		allocated := o.Demand * ratio
		if allocated > o.MaxAllocation {
			allocated = o.MaxAllocation
		}
		solution.Allocations[o.ID] = allocated
		totalAllocated += allocated
		if o.Demand > 0 {
			solution.DemandMet[o.ID] = allocated / o.Demand
		}
	}

	solution.TotalAllocated = totalAllocated
	solution.ObjectiveValue = totalAllocated / totalDemand

	return solution
}
