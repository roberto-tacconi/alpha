package pddl

import "alpha/internal/app/alpha/planner/dto/world"

type CostStrategy interface {
	CalculateCosts(w *world.WorldState, ctx *RiskContext) []Metric
}

type GoalStrategy interface {
	CalculateGoals(w *world.WorldState) []Fact
}
