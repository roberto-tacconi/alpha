package k6

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"alpha/internal/app/tests/load_test"
)

type Runner struct {
	log *slog.Logger
}

func NewRunner(log *slog.Logger) *Runner {
	return &Runner{
		log: log.With("adapter", "k6_runner"),
	}
}

func (r *Runner) Run(ctx context.Context, params *load_test.TestParameters) error {
	r.log.Info("k6.run.start",
		"cell_index", params.Cell.Index,
		"rate", params.Cell.Rate,
		"compromised", params.Cell.CompromisedCount,
		"duration", params.Duration,
	)

	args := []string{
		"run",

		"--out", fmt.Sprintf("xk6-influxdb=%s", params.InfluxURL),
		"--env", "K6_INFLUXDB_PUSH_INTERVAL=10s",
		"--env", fmt.Sprintf("K6_INFLUXDB_ORGANIZATION=%s", params.InfluxOrg),
		"--env", fmt.Sprintf("K6_INFLUXDB_BUCKET=%s", params.InfluxBucket),
		"--env", fmt.Sprintf("K6_INFLUXDB_TOKEN=%s", params.InfluxToken),

		"--env", fmt.Sprintf("INGEST_URL=%s", params.IngestURL),
		"--env", fmt.Sprintf("INJECTION_RATE=%d", params.Cell.Rate),
		"--env", fmt.Sprintf("SCENARIO_DURATION=%s", params.Duration),
		"--env", fmt.Sprintf("SEED=%d", params.Seed+int64(params.Cell.Index)),

		"--env", fmt.Sprintf("SYSTEM_IPS=%s", strings.Join(params.SystemIPs, ",")),
		"--env", fmt.Sprintf("COMPROMISED_IPS=%s", strings.Join(params.CompromisedIPs, ",")),

		"--tag", fmt.Sprintf("cell_index=%d", params.Cell.Index),
		"--tag", fmt.Sprintf("rate=%d", params.Cell.Rate),
		"--tag", fmt.Sprintf("compromised_count=%d", params.Cell.CompromisedCount),

		params.ScriptPath,
	}

	cmd := exec.CommandContext(ctx, "k6", args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			r.log.Warn("k6.run.aborted_by_context", "cell_index", params.Cell.Index)
			return ctx.Err()
		}

		return fmt.Errorf("k6 process failed with error: %w", err)
	}

	r.log.Info("k6.run.completed", "cell_index", params.Cell.Index)
	return nil
}
