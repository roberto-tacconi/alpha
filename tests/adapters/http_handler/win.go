package httphandler

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleWin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)

	var body struct {
		BatchID int `json:"batch_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.log.Warn("win.decode.failed", "err", err)
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if body.BatchID <= 0 {
		http.Error(w, "missing or invalid batch_id", http.StatusBadRequest)
		return
	}

	if err := s.receiver.AcceptWin(body.BatchID); err != nil {
		s.log.Warn("win.rejected_by_engine", "batch_id", body.BatchID, "err", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusOK)
}
