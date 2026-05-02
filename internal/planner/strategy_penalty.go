package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
)

const (
	penaltyActiveThreatName    = "penalty-active-threat"
	penaltyExistingThreatName  = "penalty-existing-threat"
	penaltyRecoveryAbandonName = "penalty-unrecovered-host"
)

type UniformPenaltyStrategy struct{}

func (s *UniformPenaltyStrategy) CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric {
	costs := make([]Metric, 0)

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		name := d.PDDLName()

		costs = append(costs,
			Metric{
				Name:  penaltyActiveThreatName,
				Args:  []string{name},
				Value: ctx.UniformPenalty,
			},
			Metric{
				Name:  penaltyExistingThreatName,
				Args:  []string{name},
				Value: ctx.UniformPenalty,
			},
			Metric{
				Name:  penaltyRecoveryAbandonName,
				Args:  []string{name},
				Value: ctx.UniformPenalty,
			},
		)
	}

	return costs
}
