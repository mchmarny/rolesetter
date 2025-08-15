package node

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// startServer initializes and starts the HTTP server for metrics and health checks.
func (i *Informer) serve(handlers map[string]http.Handler) {
	okFunc := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// health endpoint for liveness probe
	http.HandleFunc("/healthz", okFunc)
	http.HandleFunc("/readyz", okFunc)
	http.HandleFunc("/", okFunc)

	// metrics
	for path, handler := range handlers {
		http.Handle(path, handler)
		i.logger.Info("registered handler", zap.String("path", path))
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", i.port),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	i.logger.Info("starting metrics server", zap.Int("port", i.port))

	if err := srv.ListenAndServe(); err != nil {
		i.logger.Fatal("failed to start metrics server", zap.Error(err))
	}
}
