package httphandler

import (
	"alpha/internal/app/tests/load_test"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type Server struct {
	port           int
	suricataStream string
	rc             *redis.Client
	receiver       load_test.EventReceiver
	log            *slog.Logger
	srv            *http.Server
}

func NewServer(
	port int,
	suricataStream string,
	rc *redis.Client,
	receiver load_test.EventReceiver,
	log *slog.Logger,
) *Server {
	return &Server{
		port:           port,
		suricataStream: suricataStream,
		rc:             rc,
		receiver:       receiver,
		log:            log.With("adapter", "http_server"),
	}
}

func (s *Server) Start(ctx context.Context) {
	mux := http.NewServeMux()

	mux.HandleFunc("/win", s.handleWin)
	mux.HandleFunc("/ingest", s.handleIngest)

	s.srv = &http.Server{
		Addr:        fmt.Sprintf(":%d", s.port),
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
	}

	go func() {
		s.log.Info("http.server.listening", "port", s.port)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("http.server.fatal", "err", err)
			os.Exit(1)
		}
	}()

	go func() {
		<-ctx.Done()
		s.log.Info("http.server.shutting_down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutCtx); err != nil {
			s.log.Error("http.server.shutdown_error", "err", err)
		}
	}()
}
