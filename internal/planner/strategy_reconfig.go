package pddl

import "alpha/internal/app/alpha/planner/dto/world"

const (
	switchCostName   = "cost-switch"
	failoverCostName = "cost-failover"
)

type SwitchStrategy struct{}

func (ss *SwitchStrategy) CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric {
	fastestSwitches := map[int64]float64{}

	for _, c := range w.RootCapabilities() {
		for _, s := range c.Providers() {
			switchTime := s.SwitchCost()
			if fastest, ok := fastestSwitches[c.ID()]; !ok || switchTime < fastest {
				fastestSwitches[c.ID()] = switchTime
			}
		}
	}

	costs := make([]Metric, 0, len(w.RootCapabilities())*len(w.Services()))

	for _, c := range w.RootCapabilities() {
		for _, s := range c.Providers() {
			service := s.Service()

			rhoDep := 1.0 - (float64(len(service.TransitiveRequirements())) / float64(ctx.MaxDependencies+1))
			rhoRed := float64(len(service.Hosts())) / float64(ctx.MaxHostingOptions)

			bias := fastestSwitches[c.ID()] * ctx.EpsilonS * ctx.CSWMap[c.ID()] * rhoDep * rhoRed

			costs = append(costs, Metric{
				Name:  switchCostName,
				Args:  []string{c.PDDLName(), service.PDDLName()},
				Value: s.SwitchCost() - bias,
			})
		}
	}

	return costs
}

type FailoverStrategy struct{}

func (fs *FailoverStrategy) CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric {
	fastestFailovers := map[int64]float64{}

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		for _, s := range d.SupportedServices() {
			if fastest, ok := fastestFailovers[s.Service.ID()]; !ok || s.MigrationTimeSec < fastest {
				fastestFailovers[s.Service.ID()] = s.MigrationTimeSec
			}
		}
	}

	for _, a := range w.Analogs() {
		for _, s := range a.SupportedServices() {
			if fastest, ok := fastestFailovers[s.Service.ID()]; !ok || s.MigrationTimeSec < fastest {
				fastestFailovers[s.Service.ID()] = s.MigrationTimeSec
			}
		}
	}

	costs := make([]Metric, 0, len(w.Digitals())*len(w.Services()))

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}
		for _, s := range d.SupportedServices() {
			bias := fastestFailovers[s.Service.ID()] * ctx.EpsilonF * ctx.SOIMap[s.Service.ID()]

			costs = append(costs, Metric{
				Name:  failoverCostName,
				Args:  []string{s.Service.PDDLName(), d.PDDLName()},
				Value: s.MigrationTimeSec - bias,
			})
		}
	}

	for _, d := range w.Analogs() {
		for _, s := range d.SupportedServices() {
			bias := fastestFailovers[s.Service.ID()] * ctx.EpsilonF * ctx.SOIMap[s.Service.ID()]

			costs = append(costs, Metric{
				Name:  failoverCostName,
				Args:  []string{s.Service.PDDLName(), d.PDDLName()},
				Value: s.MigrationTimeSec - bias,
			})
		}
	}

	return costs
}
