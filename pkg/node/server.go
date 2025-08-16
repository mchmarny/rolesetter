package node

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// startServer initializes and starts the HTTP server for metrics and health checks.
func (i *Informer) serve(handlers map[string]http.Handler) {
	handler := i.buildHandler(handlers)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", i.port),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	i.logger.Info("starting metrics server", zap.Int("port", i.port))

	if err := srv.ListenAndServe(); err != nil {
		i.logger.Fatal("failed to start metrics server", zap.Error(err))
	}
}

// buildHandler constructs the HTTP handler mux for the server.
func (i *Informer) buildHandler(handlers map[string]http.Handler) http.Handler {
	mux := http.NewServeMux()
	okFunc := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	mux.HandleFunc("/healthz", okFunc)
	mux.HandleFunc("/readyz", okFunc)
	mux.HandleFunc("/", okFunc)
	for path, handler := range handlers {
		mux.Handle(path, handler)
		i.logger.Info("registered handler", zap.String("path", path))
	}
	return mux
}
