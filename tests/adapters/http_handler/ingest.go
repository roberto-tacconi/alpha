package httphandler

import (
	"io"
	"net/http"

	"github.com/redis/go-redis/v9"
)

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Warn("ingest.read.failed", "err", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	err = s.rc.XAdd(r.Context(), &redis.XAddArgs{
		Stream: s.suricataStream,
		Values: map[string]any{"eve": string(payload)},
	}).Err()

	if err != nil {
		s.log.Error("ingest.redis_xadd.failed", "err", err)
		http.Error(w, "internal stream write failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
