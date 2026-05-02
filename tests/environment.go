package load_test

import (
	"alpha/internal/pkg/config"
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Environment interface {
	Reset(ctx context.Context) error
	Initialize(ctx context.Context) error
	Safe(ctx context.Context) (bool, error)
}

type environment struct {
	cfg      *config.TestConfig
	docker   DockerActuation
	memgraph MemgraphRepository
	redis    RedisRepository
	log      *slog.Logger
}

func NewEnvironment(
	cfg *config.TestConfig,
	docker DockerActuation,
	memgraph MemgraphRepository,
	redis RedisRepository,
	log *slog.Logger,
) Environment {
	return &environment{
		cfg:      cfg,
		docker:   docker,
		memgraph: memgraph,
		redis:    redis,
		log:      log.With("component", "test_environment"),
	}
}

func (e *environment) Reset(ctx context.Context) error {
	e.log.Info("environment.reset.start", "action", "stop_and_wipe")

	allServices := append(append([]string{}, e.cfg.AlphaServices()...), e.cfg.InfraServices()...)

	if err := e.docker.ComposeStop(ctx, 10, allServices...); err != nil {
		e.log.Warn("compose.stop.non_zero", "err", err)
	}

	if err := e.docker.ComposeDown(ctx, allServices...); err != nil {
		return fmt.Errorf("failed to rm containers: %w", err)
	}

	volumes := []string{
		e.cfg.ComposeProject() + "_redisdata",
		e.cfg.ComposeProject() + "_memgraphdata",
	}

	if err := e.docker.RemoveVolumes(ctx, volumes...); err != nil {
		return fmt.Errorf("failed to rm volumes: %w", err)
	}

	e.log.Info("environment.reset.done")
	return nil
}

func (e *environment) Initialize(ctx context.Context) error {
	e.log.Info("environment.initialize.start")

	infraServices := []string{"memgraph", "redis"}
	e.log.Debug("starting infra services", "services", infraServices)
	if err := e.docker.ComposeUp(ctx, true, infraServices...); err != nil {
		return fmt.Errorf("failed to start infra: %w", err)
	}

	if err := e.memgraph.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("memgraph connectivity failed: %w", err)
	}

	cypherFile := e.cfg.CypherFile()
	e.log.Debug("seeding memgraph", "file", cypherFile)
	if err := e.memgraph.SeedGraph(ctx, cypherFile); err != nil {
		return fmt.Errorf("failed to seed memgraph: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
	}

	alphaServices := e.cfg.AlphaServices()
	e.log.Debug("starting alpha services", "services", alphaServices)
	if err := e.docker.ComposeUp(ctx, false, alphaServices...); err != nil {
		return fmt.Errorf("failed to start alpha services: %w", err)
	}

	if err := e.waitReady(ctx); err != nil {
		return fmt.Errorf("services readiness failed: %w", err)
	}

	e.log.Info("environment.initialize.done")
	return nil
}

func (e *environment) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(e.cfg.ReadyTimeout())
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	requiredGroups := map[string]string{
		e.cfg.StreamSuricataAlerts(): e.cfg.GroupSuricataAlerts(),
		e.cfg.StreamEvents():         e.cfg.GroupEvents(),
		e.cfg.StreamAlerts():         e.cfg.GroupAlerts(),
		e.cfg.StreamPlans():          e.cfg.GroupPlans(),
	}

	e.log.Debug("waiting for redis consumer groups", "groups_count", len(requiredGroups))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for consumer groups after %s", e.cfg.ReadyTimeout())
			}

			allReady := true
			for stream, group := range requiredGroups {
				exists, err := e.redis.HasConsumerGroup(ctx, stream, group)
				if err != nil {
					e.log.Debug("redis.xinfo.error", "stream", stream, "err", err)
					allReady = false
					break
				}
				if !exists {
					e.log.Debug("redis.group.missing", "stream", stream, "group", group)
					allReady = false
					break
				}
			}

			if allReady {
				e.log.Debug("all consumer groups ready")
				return nil
			}
		}
	}
}

func (e *environment) Safe(ctx context.Context) (bool, error) {
	count, err := e.memgraph.CountAnomalousNodes(ctx)
	if err != nil {
		return false, fmt.Errorf("issafe: %w", err)
	}

	e.log.Info("environment.safe",
		"anomalous_nodes", count,
		"safe", count == 0,
	)
	return count == 0, nil
}
