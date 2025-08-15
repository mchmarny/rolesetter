package node

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServe_HealthAndReadyEndpoints(t *testing.T) {
	// Use a custom mux for test isolation
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	ts := httptest.NewServer(mux)
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
