package memgraph

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Repository struct {
	driver neo4j.DriverWithContext
	log    *slog.Logger
}

func NewRepository(driver neo4j.DriverWithContext, log *slog.Logger) *Repository {
	return &Repository{
		driver: driver,
		log:    log.With("adapter", "memgraph"),
	}
}

func (r *Repository) VerifyConnectivity(ctx context.Context) error {
	r.log.Debug("verifying memgraph connectivity...")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		if err := r.driver.VerifyConnectivity(ctx); err == nil {
			r.log.Debug("memgraph is ready and accepting connections")
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for memgraph: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (r *Repository) SeedGraph(ctx context.Context, cypherFilePath string) (err error) {
	r.log.Debug("seeding memgraph", "file", cypherFilePath)

	rawBytes, err := os.ReadFile(cypherFilePath)
	if err != nil {
		return fmt.Errorf("failed to read cypher file %q: %w", cypherFilePath, err)
	}

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})

	defer func() {
		if closeErr := session.Close(ctx); closeErr != nil {
			r.log.Error("memgraph.session.close_failed", "err", closeErr)
			if err == nil {
				err = fmt.Errorf("failed to close memgraph session: %w", closeErr)
			}
		}
	}()

	statements := strings.Split(string(rawBytes), ";")
	executedCount := 0

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)

		if stmt == "" {
			continue
		}

		if _, runErr := session.Run(ctx, stmt, nil); runErr != nil {
			return fmt.Errorf("failed to execute cypher statement %q: %w", stmt, runErr)
		}
		executedCount++
	}

	r.log.Info("memgraph.seed.completed", "statements_executed", executedCount, "file", cypherFilePath)
	return nil
}

func (r *Repository) CountAnomalousNodes(ctx context.Context) (int, error) {
	const query = `
        MATCH (rd:Resource:Digital:Device)
		MATCH (rd)-[:CURRENT_STATE]->(ds:DigitalState)

		WITH rd,
			coalesce(rd.is_quarantined, false) AS quarantined,
			coalesce(ds.is_powered,     true)  AS powered,
			coalesce(ds.is_compromised, false) AS compromised

		WITH rd, compromised,
			quarantined OR NOT powered AS contained

		OPTIONAL MATCH (rd)-[:HAS_INTERFACE]->(:NetworkInterface)
					-[:CURRENT_STATE]->(nis_out:NetworkInterfaceState)
					-[:ANOMALOUS_COMMUNICATION]->(:NetworkInterface)

		WITH rd, compromised, contained,
			count(nis_out) > 0 AS has_outgoing_anomaly

		WITH rd, contained,
			compromised OR has_outgoing_anomaly AS is_problematic

		WHERE is_problematic AND NOT contained

		RETURN count(DISTINCT rd) AS anomalous_count
    `

	wm := time.Now().UnixMilli()

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() {
		if err := session.Close(ctx); err != nil {
			r.log.Error("memgraph.session.close_failed", "err", err)
		}
	}()

	result, err := session.Run(ctx, query, map[string]any{"wm": wm})
	if err != nil {
		return 0, fmt.Errorf("querying anomalous nodes: %w", err)
	}

	record, err := result.Single(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading anomalous count record: %w", err)
	}

	raw, ok := record.Get("anomalous_count")
	if !ok {
		return 0, fmt.Errorf("anomalous_count field missing from result")
	}

	count, ok := raw.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected type for anomalous_count: %T", raw)
	}

	r.log.Debug("memgraph.anomalous_count", "count", count, "wm", wm)
	return int(count), nil
}
