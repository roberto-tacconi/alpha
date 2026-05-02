package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const errNoSuchKey = "ERR no such key"

type Repository struct {
	client *redis.Client
	log    *slog.Logger
}

func NewRepository(client *redis.Client, log *slog.Logger) *Repository {
	return &Repository{
		client: client,
		log:    log.With("adapter", "redis"),
	}
}

func (r *Repository) PublishTombstone(ctx context.Context, stream string, batchID int) error {
	r.log.Debug("publishing tombstone", "stream", stream, "batch_id", batchID)

	payload, err := json.Marshal(map[string]any{
		"event_type": "end_of_batch",
		"batch_id":   batchID,
		"timestamp":  time.Now().Format(time.RFC3339),
		"src_ip":     "0.0.0.0",
		"dest_ip":    "0.0.0.0",
		"proto":      "NONE",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal tombstone payload: %w", err)
	}

	err = r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"eve": string(payload)},
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to xadd tombstone to stream %q: %w", stream, err)
	}

	r.log.Info("tombstone.published", "stream", stream, "batch_id", batchID)
	return nil
}

func (r *Repository) HasConsumerGroup(ctx context.Context, stream, group string) (bool, error) {
	groups, err := r.client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		if strings.HasPrefix(err.Error(), errNoSuchKey) {
			return false, nil
		}
		return false, fmt.Errorf("xinfo groups failed for stream %q: %w", stream, err)
	}

	for _, g := range groups {
		if g.Name == group {
			return true, nil
		}
	}

	return false, nil
}
