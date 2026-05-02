package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
	"alpha/internal/pkg/pddlx/ast"
	"fmt"
	"math"
)

type RiskEvaluator interface {
	Evaluate(w *world.WorldState) RiskAssessment
}

type Metric struct {
	Name  string
	Args  []string
	Value float64
}

type Fact struct {
	Name    string
	Args    []string
	Negated bool
}

type ProblemParts struct {
	InitCosts []Metric
	InitState []Fact
	Goals     []Fact
	Metric    *ast.Metric
}

type RiskContext struct {
	MaxDependencies   int
	MaxHostingOptions int

	EpsilonH float64
	EpsilonS float64
	EpsilonF float64

	DeltaCon float64
	DeltaRem float64

	Omega          float64
	DeltaP         float64
	UniformPenalty float64

	ROIMap map[int64]float64
	CSWMap map[int64]float64
	SOIMap map[int64]float64
}

type RiskAssessment struct {
	parts   ProblemParts
	context RiskContext
}

func (ra *RiskAssessment) Parts() ProblemParts {
	return ra.parts
}

func (ra *RiskAssessment) Context() RiskContext {
	return ra.context
}

type CostTuner struct {
	CostQuantum float64

	EpsilonMarginH float64
	EpsilonMarginS float64
	EpsilonMarginF float64

	DeltaP   float64
	DeltaCon float64
	DeltaRem float64

	SecondaryHostFactor  float64
	NeutralContextFactor float64
	NonCriticalDiscount  float64
}

func DefaultTuner() CostTuner {
	return CostTuner{
		CostQuantum:          1.0,
		EpsilonMarginH:       0.9,
		EpsilonMarginS:       0.9,
		EpsilonMarginF:       0.9,
		NonCriticalDiscount:  0.1,
		NeutralContextFactor: 2.0,
		SecondaryHostFactor:  2.0,
		DeltaP:               1.0,
		DeltaCon:             10.0,
		DeltaRem:             10.0,
	}
}

type riskEvaluator struct {
	tuner          CostTuner
	costStrategies []CostStrategy
	goalStrategies []GoalStrategy
}

func NewRiskEvaluator(tuner CostTuner) (RiskEvaluator, error) {
	if tuner.CostQuantum <= 0 {
		return nil, fmt.Errorf("invalid CostQuantum: %f (must be > 0)", tuner.CostQuantum)
	}

	return &riskEvaluator{
		tuner: tuner,
		costStrategies: []CostStrategy{
			&ContainmentStrategy{},
			&EradicationStrategy{},
			&RecoveryStrategy{},
			&SwitchStrategy{},
			&FailoverStrategy{},
			&UniformPenaltyStrategy{},
		},
		goalStrategies: []GoalStrategy{
			&IncidentRemediationStrategy{},
			&CriticalAvailabilityStrategy{},
		},
	}, nil
}

func (m *riskEvaluator) Evaluate(w *world.WorldState) RiskAssessment {
	ctx := m.newRiskContext(w)
	costs := make([]Metric, 0)
	goals := make([]Fact, 0)

	for _, s := range m.costStrategies {
		cs := s.CalculateCosts(w, ctx)
		costs = append(costs, cs...)
	}

	for _, s := range m.goalStrategies {
		gs := s.CalculateGoals(w)
		goals = append(goals, gs...)
	}

	return RiskAssessment{
		parts: ProblemParts{
			InitCosts: costs,
			Goals:     goals,
			Metric: &ast.Metric{
				Optimization: ast.Minimize,
				Expression:   ast.NewFunctionCall("total-cost"),
			},
		},
		context: *ctx,
	}
}

func (m *riskEvaluator) newRiskContext(w *world.WorldState) *RiskContext {
	ctx := &RiskContext{
		ROIMap: make(map[int64]float64),
		CSWMap: make(map[int64]float64),
		SOIMap: make(map[int64]float64),
	}

	m.buildLocalCaches(w, ctx)

	m.buildTopologicalBounds(w, ctx)

	m.buildSafetyDeltas(w, ctx)

	ctx.Omega = m.buildOmega(w, ctx)
	ctx.DeltaP = m.tuner.DeltaP
	ctx.UniformPenalty = ctx.Omega + ctx.DeltaP

	return ctx
}

func (m *riskEvaluator) buildLocalCaches(w *world.WorldState, ctx *RiskContext) {
	var maxCSW, maxSOI, maxROI float64

	for _, root := range w.RootCapabilities() {
		csw := root.CapabilitySafetyWeight()
		if !root.ContextCritical() {
			csw *= m.tuner.NonCriticalDiscount
		}
		ctx.CSWMap[root.ID()] = csw
		if csw > maxCSW {
			maxCSW = csw
		}
	}

	for _, s := range w.Services() {
		soi := 0.0
		for _, cap := range s.ProvidedCapabilities() {
			if root, ok := cap.(*world.RootCapability); ok {
				soi += ctx.CSWMap[root.ID()]
			}
		}
		ctx.SOIMap[s.ID()] = soi
		if soi > maxSOI {
			maxSOI = soi
		}
	}

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		roi := 0.0
		for _, hs := range d.SupportedServices() {
			if hs.Service != nil {
				roi += ctx.SOIMap[hs.Service.ID()]
			}
		}

		ctx.ROIMap[d.ID()] = roi
		if roi > maxROI {
			maxROI = roi
		}
	}

	ctx.EpsilonS = m.tuner.EpsilonMarginS
	if maxCSW > 0 {
		ctx.EpsilonS /= maxCSW
	}

	ctx.EpsilonF = m.tuner.EpsilonMarginF
	if maxSOI > 0 {
		ctx.EpsilonF /= maxSOI
	}

	ctx.EpsilonH = m.tuner.EpsilonMarginH
	if maxROI > 0 {
		ctx.EpsilonH /= maxROI
	}
}

func (m *riskEvaluator) buildTopologicalBounds(w *world.WorldState, ctx *RiskContext) {
	for _, s := range w.Services() {
		deps := len(s.TransitiveRequirements())
		if deps > ctx.MaxDependencies {
			ctx.MaxDependencies = deps
		}
	}

	hostCounts := make(map[int64]int)

	countHosts := func(services []world.HostableService) {
		for _, hs := range services {
			if hs.Service != nil {
				hostCounts[hs.Service.ID()]++
			}
		}
	}

	for _, d := range w.Digitals() {
		countHosts(d.SupportedServices())
	}
	for _, a := range w.Analogs() {
		countHosts(a.SupportedServices())
	}

	for _, count := range hostCounts {
		if count > ctx.MaxHostingOptions {
			ctx.MaxHostingOptions = count
		}
	}
}

func (m *riskEvaluator) buildSafetyDeltas(w *world.WorldState, ctx *RiskContext) {
	sumContainment := 0.0

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		cContain := math.Min(d.IsolateCost(), d.ShutdownCost())
		sumContainment += cContain
	}

	ctx.DeltaCon = sumContainment + m.tuner.DeltaCon
	ctx.DeltaRem = sumContainment + m.tuner.DeltaRem
}

func (m *riskEvaluator) buildOmega(w *world.WorldState, ctx *RiskContext) float64 {
	omega := 0.0

	for _, c := range w.RootCapabilities() {
		maxSwitch := 0.0
		for _, prov := range c.Providers() {
			cost := prov.SwitchCost()
			if !prov.Primary() {
				cost *= m.tuner.NeutralContextFactor
			}

			if c := math.Ceil(cost); c > maxSwitch {
				maxSwitch = math.Ceil(c)
			}
		}
		omega += maxSwitch
	}

	maxFailovers := make(map[int64]float64)

	evalFailover := func(services []world.HostableService) {
		for _, hs := range services {
			if hs.Service == nil {
				continue
			}
			cost := hs.MigrationTimeSec
			if !hs.Primary {
				cost *= m.tuner.SecondaryHostFactor
			}

			if c := math.Ceil(cost); c > maxFailovers[hs.Service.ID()] {
				maxFailovers[hs.Service.ID()] = c
			}
		}
	}

	for _, d := range w.Digitals() {
		evalFailover(d.SupportedServices())
	}
	for _, a := range w.Analogs() {
		evalFailover(a.SupportedServices())
	}

	for _, maxFail := range maxFailovers {
		omega += maxFail
	}

	for _, d := range w.Digitals() {
		if d.Type() == world.TypeNetworkDevice {
			continue
		}

		hostCost := math.Ceil(d.IsolateCost()) + math.Ceil(d.ShutdownCost()) +
			math.Ceil(d.PowerOnCost()+ctx.DeltaCon) +
			math.Ceil(d.ReconnectCost()+ctx.DeltaCon)

		if rbCost, ok := d.RollbackCost(); ok {
			hostCost += math.Ceil(rbCost + ctx.DeltaRem)
		}

		omega += hostCost
	}

	return omega
}
