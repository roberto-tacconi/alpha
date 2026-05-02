package influx

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"alpha/internal/app/tests/load_test"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

type Repository struct {
	write api.WriteAPIBlocking
	log   *slog.Logger
}

func NewRepository(writeAPI api.WriteAPIBlocking, log *slog.Logger) *Repository {
	return &Repository{
		write: writeAPI,
		log:   log.With("adapter", "influxdb"),
	}
}

func (r *Repository) RecordCell(ctx context.Context, res load_test.CellResult) error {
	r.log.Debug("recording cell metrics", "cell_index", res.Cell.Index, "status", res.Status)

	p := influxdb2.NewPointWithMeasurement("orchestrator.cell_result").
		AddTag("cell_index", strconv.Itoa(res.Cell.Index)).
		AddTag("rate", strconv.Itoa(res.Cell.Rate)).
		AddTag("compromised_count", strconv.Itoa(res.Cell.CompromisedCount)).
		AddTag("status", string(res.Status)).
		AddField("cell_index", float64(res.Cell.Index)).
		AddField("injection_rate", float64(res.Cell.Rate)).
		AddField("compromised_count", float64(res.Cell.CompromisedCount)).
		AddField("burst_duration_ms", res.Cell.BurstDuration.Milliseconds()).
		AddField("started_at", res.StartedAt).
		AddField("simulation_started_at", res.SimulationStartedAt).
		AddField("reset_latency_ms", res.ResetElapsed.Milliseconds()).
		AddField("k6_latency_ms", res.K6Elapsed.Milliseconds()).
		AddField("remediation_latency_ms", res.RemediationLatency.Milliseconds()).
		AddField("total_elapsed_ms", res.TotalElapsed.Milliseconds()).
		SetTime(res.StartedAt)

	if err := r.write.WritePoint(ctx, p); err != nil {
		return fmt.Errorf("failed to write cell_result point: %w", err)
	}

	return nil
}
