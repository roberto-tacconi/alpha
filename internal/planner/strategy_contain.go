package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
	"math"
)

const (
	shutdownCostName = "cost-shutdown"
	isolateCostName  = "cost-isolate"
)

type ContainmentStrategy struct{}

func (cs *ContainmentStrategy) CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric {
	costs := make([]Metric, 0, len(w.Digitals())*2)

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		h := math.Min(d.IsolateCost(), d.ShutdownCost())
		bias := h * ctx.EpsilonH * ctx.ROIMap[d.ID()]

		costs = append(costs,
			Metric{
				Name:  isolateCostName,
				Args:  []string{d.PDDLName()},
				Value: d.IsolateCost() - bias,
			},
			Metric{
				Name:  shutdownCostName,
				Args:  []string{d.PDDLName()},
				Value: d.ShutdownCost() - bias,
			},
		)
	}

	return costs
}
