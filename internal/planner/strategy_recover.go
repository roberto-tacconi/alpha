package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
	"math"
)

const (
	reconnectCostName               = "cost-reconnect"
	reconnectRecoveryCostName       = "cost-reconnect-recovery"
	reconnectRecoveryUnsafeCostName = "cost-reconnect-recovery-unsafe"

	powerOnCostName               = "cost-power-on"
	powerOnRecoveryCostName       = "cost-power-on-recovery"
	powerOnRecoveryUnsafeCostName = "cost-power-on-recovery-unsafe"
)

type RecoveryStrategy struct{}

func (rs *RecoveryStrategy) CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric {
	costs := make([]Metric, 0, len(w.Digitals())*6)

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}
		
		h := math.Min(d.PowerOnCost(), d.ReconnectCost())
		bias := h * ctx.EpsilonH * ctx.ROIMap[d.ID()]

		costs = append(costs,
			Metric{
				Name:  reconnectCostName,
				Args:  []string{d.PDDLName()},
				Value: d.ReconnectCost(),
			},
			Metric{
				Name:  reconnectRecoveryCostName,
				Args:  []string{d.PDDLName()},
				Value: d.ReconnectCost() - bias,
			},
			Metric{
				Name:  reconnectRecoveryUnsafeCostName,
				Args:  []string{d.PDDLName()},
				Value: d.ReconnectCost() + ctx.DeltaCon - bias,
			},
			Metric{
				Name:  powerOnCostName,
				Args:  []string{d.PDDLName()},
				Value: d.PowerOnCost(),
			},
			Metric{
				Name:  powerOnRecoveryCostName,
				Args:  []string{d.PDDLName()},
				Value: d.PowerOnCost() - bias,
			},
			Metric{
				Name:  powerOnRecoveryUnsafeCostName,
				Args:  []string{d.PDDLName()},
				Value: d.PowerOnCost() + ctx.DeltaCon - bias,
			},
		)
	}

	return costs
}
