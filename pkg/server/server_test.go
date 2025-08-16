package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mchmarny/rolesetter/pkg/log"
)

func TestBuildHandler_HealthAndReadyEndpoints(t *testing.T) {
	logger := log.GetTestLogger()
	srv := &server{logger: logger, port: 8080}
	handler := srv.buildHandler(nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	endpoints := []string{"/healthz", "/readyz", "/"}
	for _, ep := range endpoints {
		resp, err := http.Get(ts.URL + ep)
		if err != nil {
			t.Fatalf("failed to GET %s: %v", ep, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for %s, got %d", ep, resp.StatusCode)
		}
	}
}

func TestBuildHandler_RegistersMetricsHandler(t *testing.T) {
	logger := log.GetTestLogger()
	srv := &server{logger: logger, port: 8080}
	metricsCalled := false
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		metricsCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.buildHandler(map[string]http.Handler{"/metrics": metricsHandler})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("failed to GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /metrics, got %d", resp.StatusCode)
	}
	if !metricsCalled {
		t.Error("metrics handler was not called")
	}
}
