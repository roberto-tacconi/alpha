package pddl

import "context"

type PlannerEngine interface {
	Solve(ctx context.Context, domainPath, problemPath string, args []string) ([]string, error)
}
