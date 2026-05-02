package pddl

import (
	"alpha/internal/app/alpha/planner/dto/world"
	"alpha/internal/pkg/pddlx"
	"alpha/internal/pkg/pddlx/generator"
	"alpha/internal/pkg/pddlx/parser"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type PlanningOrchestrator interface {
	GeneratePlan(ctx context.Context, problemName string, snapshot *world.WorldState) (pddlx.Plan, error)
}

type planningOrchestrator struct {
	domainPath  string
	problemsDir string
	plansDir    string

	domain pddlx.Domain

	riskEvaluator   RiskEvaluator
	problemEncoder  ProblemEncoder
	engine          PlannerEngine
	executionPolicy ExecutionPolicy

	log *slog.Logger
}

func NewPlanningOrchestrator(
	domainPath string,
	problemsDir string,
	plansDir string,

	riskEvaluator RiskEvaluator,
	problemEncoder ProblemEncoder,
	engine PlannerEngine,
	executionPolicy ExecutionPolicy,

	log *slog.Logger,
) PlanningOrchestrator {
	orchestrator := &planningOrchestrator{
		domainPath:      domainPath,
		problemsDir:     problemsDir,
		plansDir:        plansDir,
		riskEvaluator:   riskEvaluator,
		problemEncoder:  problemEncoder,
		engine:          engine,
		executionPolicy: executionPolicy,
		log:             log,
	}

	domain, err := parser.ParseDomain(domainPath)
	if err != nil {
		orchestrator.log.Error("pddl.parse.failed", "err", err)
		return nil
	}
	orchestrator.domain = domain

	return orchestrator
}

func (po *planningOrchestrator) GeneratePlan(ctx context.Context, problemName string, snapshot *world.WorldState) (pddlx.Plan, error) {
	assessment := po.riskEvaluator.Evaluate(snapshot)

	args, err := po.executionPolicy.GenerateArgs(snapshot, assessment)

	if err != nil {
		return nil, fmt.Errorf("failed defining execution policy: %w", err)
	}

	encoded, err := po.problemEncoder.Encode(snapshot, assessment.Parts())

	if err != nil {
		return nil, fmt.Errorf("problem encoding failed: %w", err)
	}

	problem, err := po.toPDDLProblem(problemName, encoded)

	if err != nil {
		return nil, fmt.Errorf("problem translation failed: %w", err)
	}

	rawProblem, err := generator.GenerateProblem(problem)

	if err != nil {
		return nil, fmt.Errorf("problem generation failed: %w", err)
	}

	problemPath := filepath.Join(po.problemsDir, fmt.Sprintf("%s.pddl", problem.Name()))
	if err := os.WriteFile(problemPath, []byte(rawProblem), 0644); err != nil {
		return nil, fmt.Errorf("failed to save problem file: %w", err)
	}

	rawPlans, err := po.engine.Solve(ctx, po.domainPath, problemPath, args)
	if err != nil {
		return nil, fmt.Errorf("planning execution failed for '%s': %w", problemName, err)
	}

	for i, rawPlan := range rawPlans {
		dest := filepath.Join(po.plansDir, fmt.Sprintf("%s.plan.%d", problem.Name(), i))
		if err := os.WriteFile(dest, []byte(rawPlan), 0644); err != nil {
			po.log.Warn("planner.plan.persist_failed", "path", dest, "err", err)
		}
	}

	best, err := parser.ParsePlan(rawPlans[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse best plan: %w", err)
	}

	return best, nil
}

func (po *planningOrchestrator) toPDDLProblem(problemName string, problem *EncodedProblem) (pddlx.Problem, error) {
	builder := pddlx.NewProblemBuilder(problemName, po.domain)

	for _, obj := range problem.Objects {
		if err := builder.AddObject(obj.Name, obj.Type); err != nil {
			return nil, fmt.Errorf("failed to add object '%s': %w", obj.Name, err)
		}
	}

	for _, p := range problem.Preds {
		if err := builder.AssignPredicate(p); err != nil {
			return nil, fmt.Errorf("failed to assign initial predicate %s: %w", p.Name, err)
		}
	}

	for _, assign := range problem.Assigns {
		if err := builder.AssignFunction(assign); err != nil {
			return nil, fmt.Errorf("failed to assign initial function value: %w", err)
		}
	}

	for _, gp := range problem.Goals {
		if err := builder.AddGoalPredicate(gp); err != nil {
			return nil, fmt.Errorf("failed to add goal predicate: %w", err)
		}
	}

	if metric := problem.Metric; metric != nil {
		if err := builder.SetMetric(metric.Optimization, metric.Expression); err != nil {
			return nil, fmt.Errorf("failed to set problem metric: %w", err)
		}
	}

	prob, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build PDDL problem: %w", err)
	}

	return prob, nil
}
