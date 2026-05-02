package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
)

const (
	rollbackCostName       = "cost-rollback"
	rollbackUnsafeCostName = "cost-rollback-unsafe"
)

type EradicationStrategy struct{}

func (es *EradicationStrategy) CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric {
	costs := make([]Metric, 0, len(w.Digitals())*2)

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		cost, ok := d.RollbackCost()

		if !ok {
			continue
		}

		bias := float64(cost) * ctx.EpsilonH * ctx.ROIMap[d.ID()]

		costs = append(costs,
			Metric{
				Name:  rollbackCostName,
				Args:  []string{d.PDDLName()},
				Value: cost - bias,
			},
			Metric{
				Name:  rollbackUnsafeCostName,
				Args:  []string{d.PDDLName()},
				Value: cost + ctx.DeltaRem - bias,
			},
		)
	}

	return costs
}
