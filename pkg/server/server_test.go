package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mchmarny/rolesetter/pkg/logger"
)

func TestBuildHandler_HealthAndReady(t *testing.T) {
	log := logger.GetTestLogger()
	srv := &server{logger: log, port: 8080, ready: func() bool { return true }}
	ts := httptest.NewServer(srv.buildHandler(nil))
	defer ts.Close()

	for _, ep := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(ts.URL + ep)
		if err != nil {
			t.Fatalf("GET %s: %v", ep, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", ep, resp.StatusCode)
		}
	}
}

func TestBuildHandler_ReadyFailure(t *testing.T) {
	log := logger.GetTestLogger()
	srv := &server{logger: log, port: 8080, ready: func() bool { return false }}
	ts := httptest.NewServer(srv.buildHandler(nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestBuildHandler_UnknownReturns404(t *testing.T) {
	log := logger.GetTestLogger()
	srv := &server{logger: log, port: 8080, ready: func() bool { return true }}
	ts := httptest.NewServer(srv.buildHandler(nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got %d", resp.StatusCode)
	}
}

func TestBuildHandler_RegistersMetrics(t *testing.T) {
	log := logger.GetTestLogger()
	srv := &server{logger: log, port: 8080, ready: func() bool { return true }}
	called := atomic.Bool{}
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(srv.buildHandler(map[string]http.Handler{"/metrics": metricsHandler}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if !called.Load() {
		t.Error("metrics handler not called")
	}
}

func TestServe_GracefulShutdown(t *testing.T) {
	log := logger.GetTestLogger()
	port := freePort(t)
	srv := NewServer(WithLogger(log), WithPort(port))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx, nil)
	}()

	// Wait for listener to come up.
	if !waitFor(t, "http://localhost:"+strconv.Itoa(port)+"/healthz", 2*time.Second) {
		t.Fatal("server never started")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

func TestServe_BindFailureReturnsError(t *testing.T) {
	log := logger.GetTestLogger()
	port := freePort(t)
	// Hold the port to force ListenAndServe to fail.
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	srv := NewServer(WithLogger(log), WithPort(port))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Serve(ctx, nil); err == nil {
		t.Error("expected error when port is already bound")
	} else if errors.Is(err, http.ErrServerClosed) {
		t.Error("did not expect ErrServerClosed for bind failure")
	}
}

func TestOptionSetters(t *testing.T) {
	s := &server{}
	WithLogger(logger.GetTestLogger())(s)
	if s.logger == nil {
		t.Error("WithLogger did not set logger")
	}
	WithPort(1234)(s)
	if s.port != 1234 {
		t.Error("WithPort did not set port")
	}
	WithReady(func() bool { return false })(s)
	if s.ready == nil || s.ready() {
		t.Error("WithReady did not set ready func")
	}
}

func TestNewServer_DefaultsAndOptions(t *testing.T) {
	s := NewServer()
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	impl, ok := NewServer(WithLogger(logger.GetTestLogger()), WithPort(4321)).(*server)
	if !ok {
		t.Fatal("NewServer did not return *server")
	}
	if impl.port != 4321 {
		t.Errorf("expected port 4321, got %d", impl.port)
	}
	if impl.ready == nil || !impl.ready() {
		t.Error("default ready should report true")
	}
}

// freePort finds an unused TCP port for tests that need a real listener.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitFor(t *testing.T, url string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
