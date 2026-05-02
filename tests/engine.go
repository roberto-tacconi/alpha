package load_test

import (
	"alpha/internal/pkg/config"
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

type winSignal struct{ ReceivedAt time.Time }

type SimulationEngine struct {
	cfg         *config.TestConfig
	metrics     MetricsRepository
	environment Environment
	executor    TestExecutor
	redis       RedisRepository
	log         *slog.Logger

	mu         sync.RWMutex
	activeCell MatrixCell
	winChan    chan winSignal
}

func NewSimulationEngine(
	cfg *config.TestConfig,
	metrics MetricsRepository,
	environment Environment,
	executor TestExecutor,
	redis RedisRepository,
	log *slog.Logger,
) *SimulationEngine {
	return &SimulationEngine{
		cfg:         cfg,
		metrics:     metrics,
		environment: environment,
		executor:    executor,
		redis:       redis,
		log:         log.With("component", "simulation_engine"),
		winChan:     make(chan winSignal, 1),
	}
}

func (e *SimulationEngine) AcceptWin(batchID int) error {
	e.mu.RLock()
	active := e.activeCell
	e.mu.RUnlock()

	if batchID != active.Index {
		return fmt.Errorf("old win signal received: current %d, expected %d", batchID, active.Index)
	}

	select {
	case e.winChan <- winSignal{ReceivedAt: time.Now()}:
		e.log.Info("win.accepted", "batch_id", batchID)
		return nil
	default:
		return fmt.Errorf("win already notified for %d", batchID)
	}
}

func (e *SimulationEngine) Run(ctx context.Context) error {
	cells := e.buildMatrix()
	results := make([]CellResult, 0, len(cells))

	e.log.Info("simulation.start",
		"total_cells", len(cells),
		"rates", e.cfg.Rates,
		"compromised_counts", e.cfg.CompromisedCounts,
	)

	for _, cell := range cells {
		result, err := e.execCell(ctx, cell)
		if err != nil {
			e.log.Error("cell.failed", "cell_index", cell.Index, "err", err)
		}
		results = append(results, result)

		if ctx.Err() != nil {
			e.log.Warn("simulation.interrupted",
				"completed", len(results),
				"total", len(cells),
			)
			break
		}
	}

	e.logSummary(results)
	return nil
}

func (e *SimulationEngine) execCell(ctx context.Context, cell MatrixCell) (CellResult, error) {
	e.log.Info("cell.start",
		"cell_index", cell.Index,
		"rate", cell.Rate,
		"compromised_count", cell.CompromisedCount,
	)

	result := CellResult{
		Cell:      cell,
		Status:    CellStatusFailedReset,
		StartedAt: time.Now(),
	}

	if err := e.environment.Reset(ctx); err != nil {
		result.ResetElapsed = time.Since(result.StartedAt)
		_ = e.metrics.RecordCell(ctx, result)
		return result, fmt.Errorf("reset: %w", err)
	}

	if err := e.environment.Initialize(ctx); err != nil {
		result.ResetElapsed = time.Since(result.StartedAt)
		_ = e.metrics.RecordCell(ctx, result)
		return result, fmt.Errorf("initialize: %w", err)
	}

	result.ResetElapsed = time.Since(result.StartedAt)
	result.Status = CellStatusFailedK6

	e.armCell(cell)

	shuffled := make([]string, len(e.cfg.VulnerableIPs()))
	copy(shuffled, e.cfg.VulnerableIPs())

	rng := rand.New(rand.NewSource(e.cfg.Seed()))

	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	count := min(cell.CompromisedCount, len(shuffled))
	compromised := shuffled[:count]

	params := &TestParameters{
		Cell:           cell,
		Seed:           e.cfg.Seed(),
		Duration:       e.cfg.ScenarioDuration().String(),
		ScriptPath:     e.cfg.K6Script(),
		IngestURL:      fmt.Sprintf("%s/ingest", e.cfg.OrchestratorURL()),
		InfluxURL:      e.cfg.InfluxURL(),
		InfluxOrg:      e.cfg.InfluxOrg(),
		InfluxBucket:   e.cfg.InfluxBucket(),
		InfluxToken:    e.cfg.InfluxToken(),
		SystemIPs:      e.cfg.SystemIPs(),
		CompromisedIPs: compromised,
	}

	result.SimulationStartedAt = time.Now()

	if err := e.executor.Run(ctx, params); err != nil {
		result.K6Elapsed = time.Since(result.SimulationStartedAt)
		result.TotalElapsed = time.Since(result.StartedAt)
		_ = e.metrics.RecordCell(ctx, result)
		return result, fmt.Errorf("k6: %w", err)
	}
	result.K6Elapsed = time.Since(result.SimulationStartedAt)
	e.log.Debug("cell.k6_done", "cell_index", cell.Index)

	if err := e.redis.PublishTombstone(ctx, e.cfg.StreamSuricataAlerts(), cell.Index); err != nil {
		return result, fmt.Errorf("tombstone: %w", err)
	}

	select {
	case w := <-e.winChan:
		safe, err := e.environment.Safe(ctx)
		if err != nil {
			e.log.Warn("cell.issafe.error", "cell_index", cell.Index, "err", err)
			result.Status = CellStatusFailedVerification
			result.RemediationLatency = time.Since(result.SimulationStartedAt)
		} else if safe {
			result.Status = CellStatusSuccess
			result.RemediationLatency = w.ReceivedAt.Sub(result.SimulationStartedAt)
		} else {
			e.log.Warn("cell.issafe.anomalies_remain", "cell_index", cell.Index)
			result.Status = CellStatusFailedVerification
			result.RemediationLatency = time.Since(result.SimulationStartedAt)
		}
	case <-time.After(e.cfg.WaitTimeout()):
		result.Status = CellStatusTimeout
		result.RemediationLatency = time.Since(result.SimulationStartedAt)
	case <-ctx.Done():
		return result, ctx.Err()
	}

	result.TotalElapsed = time.Since(result.StartedAt)

	e.log.Info("cell.done",
		"cell_index", cell.Index,
		"status", result.Status,
		"total_elapsed_ms", result.TotalElapsed.Milliseconds(),
	)

	if err := e.metrics.RecordCell(ctx, result); err != nil {
		e.log.Warn("metrics.record_cell.failed", "cell_index", cell.Index, "err", err)
	}
	return result, nil
}

func (e *SimulationEngine) armCell(cell MatrixCell) {
	for {
		select {
		case <-e.winChan:
		default:
			e.mu.Lock()
			e.activeCell = cell
			e.mu.Unlock()
			return
		}
	}
}

func (e *SimulationEngine) buildMatrix() []MatrixCell {
	cells := make([]MatrixCell, 0, len(e.cfg.Rates())*len(e.cfg.CompromisedCounts()))
	idx := 1
	for _, c := range e.cfg.CompromisedCounts() {
		for _, r := range e.cfg.Rates() {
			cells = append(cells, MatrixCell{
				Index:            idx,
				Rate:             r,
				CompromisedCount: c,
				BurstDuration:    e.cfg.ScenarioDuration(),
			})
			idx++
		}
	}
	return cells
}

func (e *SimulationEngine) logSummary(results []CellResult) {
	counts := map[CellStatus]int{}
	for _, r := range results {
		counts[r.Status]++
	}
	e.log.Info("simulation.done",
		"total", len(results),
		"success", counts[CellStatusSuccess],
		"timeout", counts[CellStatusTimeout],
		"failed_reset", counts[CellStatusFailedReset],
		"failed_k6", counts[CellStatusFailedK6],
		"failed_verification", counts[CellStatusFailedVerification],
	)
}
