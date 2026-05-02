package load_test

import (
	"context"
	"time"
)

type CellStatus string

const (
	CellStatusSuccess            CellStatus = "success"
	CellStatusTimeout            CellStatus = "timeout"
	CellStatusFailedK6           CellStatus = "failed_k6"
	CellStatusFailedReset        CellStatus = "failed_reset"
	CellStatusFailedVerification CellStatus = "failed_verification"
)

type MatrixCell struct {
	Index            int
	Rate             int
	CompromisedCount int
	BurstDuration    time.Duration
}

type CellResult struct {
	Cell                MatrixCell
	Status              CellStatus
	StartedAt           time.Time
	SimulationStartedAt time.Time
	ResetElapsed        time.Duration
	K6Elapsed           time.Duration
	TotalElapsed        time.Duration
	RemediationLatency  time.Duration
}

type TestParameters struct {
	Cell           MatrixCell
	Seed           int64
	Duration       string
	ScriptPath     string
	IngestURL      string
	InfluxURL      string
	InfluxOrg      string
	InfluxBucket   string
	InfluxToken    string
	SystemIPs      []string
	CompromisedIPs []string
}

type MetricsRepository interface {
	RecordCell(ctx context.Context, r CellResult) error
}

type EventReceiver interface {
	AcceptWin(batchID int) error
}

type RedisRepository interface {
	PublishTombstone(ctx context.Context, stream string, batchID int) error
	HasConsumerGroup(ctx context.Context, stream, group string) (bool, error)
}

type MemgraphRepository interface {
	VerifyConnectivity(ctx context.Context) error
	SeedGraph(ctx context.Context, cypherFilePath string) error
	CountAnomalousNodes(ctx context.Context) (int, error)
}

type DockerActuation interface {
	ComposeUp(ctx context.Context, wait bool, services ...string) error
	ComposeStop(ctx context.Context, timeoutSeconds int, services ...string) error
	ComposeDown(ctx context.Context, services ...string) error

	RemoveVolumes(ctx context.Context, volumes ...string) error
}

type TestExecutor interface {
	Run(ctx context.Context, params *TestParameters) error
}
