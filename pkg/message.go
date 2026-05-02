package message

import (
	"alpha/internal/pkg/action"
	"alpha/internal/pkg/mutation"
	"encoding/json"
	"fmt"
	"time"
)

type EventKind string
type AlertKind string
type PlanKind string

const (
	MutationEvent         EventKind = "event.kind.mutation"
	SuricataEvent         EventKind = "event.kind.suricata"
	ExecutorEvent         EventKind = "event.kind.executor"
	AlertKindGraphChanged AlertKind = "alert.kind.graph.changed"
	PlanKindRemediation   PlanKind  = "plan.kind.remediation"

	EventKindTombstone EventKind = "event.kind.tombstone"
	AlertKindTombstone AlertKind = "alert.kind.tombstone"
	PlanKindTombstone  PlanKind  = "plan.kind.tombstone"
)

type Event struct {
	CreatedAt time.Time       `redisx:"created_at"`
	Kind      EventKind       `redisx:"kind"`
	Mutation  json.RawMessage `redisx:"mutation"`
}

func NewEvent(kind EventKind, mut mutation.Mutation) (*Event, error) {
	payload, err := json.Marshal(mut)
	if err != nil {
		return nil, fmt.Errorf("message.event.marshal_failed: kind=%s, op=%s, err=%w", kind, mut.Op(), err)
	}

	return &Event{
		CreatedAt: time.Now().UTC(),
		Kind:      kind,
		Mutation:  payload,
	}, nil
}

type Alert struct {
	CreatedAt time.Time `redisx:"created_at"`
	Kind      AlertKind `redisx:"kind"`
	Watermark int64     `redisx:"watermark"`
}

func NewAlert(kind AlertKind, watermark int64) (*Alert, error) {
	return &Alert{
		CreatedAt: time.Now().UTC(),
		Kind:      kind,
		Watermark: watermark,
	}, nil
}

type Plan struct {
	CreatedAt int64             `redisx:"created_at"`
	Kind      PlanKind          `redisx:"kind"`
	Watermark int64             `redisx:"watermark"`
	Uuid      string            `redisx:"uuid"`
	Steps     []json.RawMessage `redisx:"steps"`
}

func NewPlan(kind PlanKind, watermark int64, uuid string, steps []action.Action) (*Plan, error) {
	rawSteps := make([]json.RawMessage, 0, len(steps))

	for i, step := range steps {
		b, err := step.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("message.plan.marshal_step_failed: index=%d, action=%s, err=%w", i, step.Name(), err)
		}
		rawSteps = append(rawSteps, b)
	}

	return &Plan{
		CreatedAt: time.Now().UTC().UnixNano(),
		Kind:      kind,
		Watermark: watermark,
		Uuid:      uuid,
		Steps:     rawSteps,
	}, nil
}

type DLQMessage struct {
	CreatedAt  time.Time `redisx:"created_at"`
	OriginalID string    `redisx:"original_id"`
	Error      string    `redisx:"error"`
	FieldsJSON string    `redisx:"fields_json"`
}

type SuricataRawEvent struct {
	Payload json.RawMessage `redisx:"eve"`
}

type Tombstone struct {
	BatchID   int       `json:"batch_id" redisx:"batch_id"`
	CreatedAt time.Time `json:"created_at" redisx:"created_at"`
}

func NewTombstone(batchID int) *Tombstone {
	return &Tombstone{
		BatchID:   batchID,
		CreatedAt: time.Now().UTC(),
	}
}

func NewTombstoneEvent(batchID int) (*Event, error) {
	t := NewTombstone(batchID)
	payload, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("message.tombstone.marshal_failed: batch=%d, err=%w", batchID, err)
	}

	return &Event{
		CreatedAt: t.CreatedAt,
		Kind:      EventKindTombstone,
		Mutation:  payload,
	}, nil
}

func NewTombstoneAlert(batchID int) *Alert {
	return &Alert{
		CreatedAt: time.Now().UTC(),
		Kind:      AlertKindTombstone,
		Watermark: int64(batchID),
	}
}

func NewTombstonePlan(batchID int) *Plan {
	return &Plan{
		CreatedAt: time.Now().UTC().UnixNano(),
		Kind:      PlanKindTombstone,
		Watermark: int64(batchID),
		Uuid:      fmt.Sprintf("tombstone-batch-%d", batchID),
		Steps:     []json.RawMessage{},
	}
}
