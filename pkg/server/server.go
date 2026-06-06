// Package server hosts the controller's HTTP surface: Prometheus metrics,
// a liveness probe (/healthz), and a readiness probe (/readyz). The
// readiness probe is gated by a caller-supplied Ready function so the pod
// only reports ready once the informer cache has synced and (when leader
// election is enabled) the replica is actively leading.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mchmarny/rolesetter/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultPort  = 8080
	readWriteTO  = 10 * time.Second
	idleTO       = 60 * time.Second
	shutdownTO   = 5 * time.Second
	readHeaderTO = 5 * time.Second
)

// Server defines the controller's HTTP server.
type Server interface {
	// Serve starts the HTTP server and blocks until ctx is canceled or
	// ListenAndServe returns an error. The supplied handlers are mounted
	// in addition to the built-in /healthz and /readyz endpoints.
	Serve(ctx context.Context, handlers map[string]http.Handler) error
}

// ReadyFunc returns true when the controller is ready to serve traffic.
// It must be cheap to call (probe frequency).
type ReadyFunc func() bool

// Option configures Server.
type Option func(*server)

// WithLogger sets the logger.
func WithLogger(logger *zap.Logger) Option {
	return func(s *server) {
		s.logger = logger
	}
}

// WithPort sets the listen port.
func WithPort(port int) Option {
	return func(s *server) {
		s.port = port
	}
}

// WithReady installs a readiness predicate consulted by /readyz. When unset,
// readiness reports OK as soon as the HTTP listener is up.
func WithReady(ready ReadyFunc) Option {
	return func(s *server) {
		s.ready = ready
	}
}

// NewServer returns a Server configured by the supplied options.
func NewServer(opts ...Option) Server {
	s := &server{
		logger: logger.GetLogger(),
		port:   defaultPort,
		ready:  func() bool { return true },
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.ready == nil {
		s.ready = func() bool { return true }
	}
	return s
}

type server struct {
	logger *zap.Logger
	port   int
	ready  ReadyFunc
}

// Serve starts the metrics/health HTTP server. It blocks until ctx is
// canceled (graceful shutdown) or ListenAndServe fails. Callers should
// treat a non-nil return as fatal and cancel the parent context.
func (s *server) Serve(ctx context.Context, handlers map[string]http.Handler) error {
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s.buildHandler(handlers),
		ReadTimeout:       readWriteTO,
		ReadHeaderTimeout: readHeaderTO,
		WriteTimeout:      readWriteTO,
		IdleTimeout:       idleTO,
	}

	s.logger.Info("starting metrics server", zap.Int("port", s.port))

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTO)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("metrics server shutdown error", zap.Error(err))
		}
		<-errCh
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("metrics server: %w", err)
		}
		return nil
	}
}

// buildHandler returns the mux with built-in probes and caller-supplied
// handlers mounted. Unknown paths return 404 so probe misconfiguration is
// surfaced explicitly.
func (s *server) buildHandler(handlers map[string]http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !s.ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	for path, handler := range handlers {
		mux.Handle(path, handler)
		s.logger.Info("registered handler", zap.String("path", path))
	}

	return mux
}
