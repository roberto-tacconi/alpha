package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
	"fmt"
	"log/slog"
	"time"
)

type ExecutionPolicy interface {
	GenerateArgs(snapshot *world.WorldState, assessment RiskAssessment) ([]string, error)
}

type lamaExecutionPolicy struct {
	minTimeForSolution time.Duration
	maxTimeForSolution time.Duration
	log                *slog.Logger
}

func NewLAMAExecutionPolicy(
	minTimeForSolution time.Duration,
	maxTimeForSolution time.Duration,
	log *slog.Logger,
) ExecutionPolicy {
	return &lamaExecutionPolicy{
		minTimeForSolution: minTimeForSolution,
		maxTimeForSolution: maxTimeForSolution,
		log:                log,
	}
}

func (p *lamaExecutionPolicy) GenerateArgs(snapshot *world.WorldState, assessment RiskAssessment) ([]string, error) {
	ctx := assessment.Context()
	roiTotal := int(snapshot.GlobalTotalROI())

	var sumCompROI, nCompromised int
	for _, d := range snapshot.Digitals() {
		if d.Type() == world.TypeNetworkDevice || !d.Compromised() {
			continue
		}
		nCompromised++
		sumCompROI += int(ctx.ROIMap[d.ID()])
	}

	budget := p.budget(sumCompROI, roiTotal)

	p.log.Info("planner.budget.computed",
		slog.Int("n_compromised", nCompromised),
		slog.Int("sum_comp_roi", sumCompROI),
		slog.Int("roi_total", roiTotal),
		slog.Float64("residual_fraction", residualFraction(sumCompROI, roiTotal)),
		slog.Float64("budget_s", budget.Seconds()),
	)

	return []string{
		"--overall-time-limit", fmt.Sprintf("%ds", int(budget.Seconds())),
	}, nil
}

func (p *lamaExecutionPolicy) budget(sumCompROI, roiTotal int) time.Duration {
	if sumCompROI <= 0 || roiTotal <= 0 {
		return p.maxTimeForSolution
	}

	raw := time.Duration(float64(p.maxTimeForSolution) * residualFraction(sumCompROI, roiTotal))

	switch {
	case raw < p.minTimeForSolution:
		return p.minTimeForSolution
	case raw > p.maxTimeForSolution:
		return p.maxTimeForSolution
	default:
		return raw
	}
}

func residualFraction(sumCompROI, roiTotal int) float64 {
	if roiTotal <= 0 {
		return 1.0
	}
	return 1.0 - float64(sumCompROI)/float64(roiTotal)
}
