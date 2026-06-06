package node

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/mchmarny/rolesetter/pkg/logger"
	"github.com/mchmarny/rolesetter/pkg/server"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeServer struct {
	served chan struct{}
	err    error
}

func (f *fakeServer) Serve(ctx context.Context, _ map[string]http.Handler) error {
	if f.served != nil {
		close(f.served)
	}
	if f.err != nil {
		return f.err
	}
	<-ctx.Done()
	return nil
}

func newTestInformer(t *testing.T, srv server.Server) *Informer {
	t.Helper()
	cs := fake.NewClientset()
	inf, err := NewInformer(
		WithLogger(logger.GetTestLogger()),
		WithLabel("test-label"),
		WithPort(8080),
		WithClientset(cs),
		WithServer(srv),
	)
	if err != nil {
		t.Fatalf("NewInformer: %v", err)
	}
	return inf
}

func TestInformer_Validate(t *testing.T) {
	log := logger.GetTestLogger()
	cs := fake.NewClientset()
	srv := server.NewServer(server.WithLogger(log), server.WithPort(8080))

	inf := &Informer{
		logger:    log,
		label:     "test-label",
		port:      8080,
		clientset: cs,
		server:    srv,
	}
	if err := inf.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	i := &Informer{}
	if err := i.validate(); err == nil {
		t.Error("expected error for missing fields")
	}
	i.logger = logger.GetTestLogger()
	if err := i.validate(); err == nil {
		t.Error("expected error for missing label")
	}
	i.label = "foo"
	if err := i.validate(); err == nil {
		t.Error("expected error for missing port")
	}
	i.port = 1234
	if err := i.validate(); err == nil {
		t.Error("expected error for missing clientset")
	}
}

func TestNewInformer_Validation(t *testing.T) {
	log := logger.GetTestLogger()
	cs := fake.NewClientset()

	inf, err := NewInformer(
		WithLogger(log),
		WithLabel("test-label"),
		WithPort(8080),
		WithClientset(cs),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inf == nil {
		t.Fatal("expected non-nil informer")
	}

	if _, err := NewInformer(
		WithLogger(log),
		WithPort(8080),
		WithClientset(cs),
	); err == nil {
		t.Error("expected error for missing label")
	}
}

func TestInformer_Inform_GracefulShutdown(t *testing.T) {
	srv := &fakeServer{served: make(chan struct{})}
	inf := newTestInformer(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- inf.Inform(ctx)
	}()

	select {
	case <-srv.served:
	case <-time.After(2 * time.Second):
		t.Fatal("server.Serve never started")
	}

	// Wait for cache to sync before canceling so we exercise the
	// graceful-shutdown path rather than the cache-sync-failure path.
	if !waitForReady(inf, 2*time.Second) {
		t.Fatal("informer cache never synced")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Inform returned error on graceful shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Inform did not return after context cancel")
	}
}

func waitForReady(inf *Informer, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if inf.ready() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestInformer_Inform_ServerError(t *testing.T) {
	srv := &fakeServer{err: errors.New("listen fail")}
	inf := newTestInformer(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- inf.Inform(ctx)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from Inform when server fails")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Inform did not return after server error")
	}
}

func TestInformer_Ready(t *testing.T) {
	srv := &fakeServer{served: make(chan struct{})}
	inf := newTestInformer(t, srv)

	if inf.ready() {
		t.Error("expected not ready before cache sync")
	}
	inf.cacheSynced.Store(true)
	if !inf.ready() {
		t.Error("expected ready after cache sync (no namespace)")
	}

	inf.namespace = "test-ns"
	if inf.ready() {
		t.Error("expected not ready when namespace set but not leading")
	}
	inf.leading.Store(true)
	if !inf.ready() {
		t.Error("expected ready when leading + cache synced")
	}
}

// Option setter unit tests
func TestOptionSetters(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*Informer)
		check func(*Informer) bool
	}{
		{"WithReplace", func(i *Informer) { WithReplace(true)(i) }, func(i *Informer) bool { return i.replace }},
		{"WithLabel", func(i *Informer) { WithLabel("x")(i) }, func(i *Informer) bool { return i.label == "x" }},
		{"WithPort", func(i *Informer) { WithPort(99)(i) }, func(i *Informer) bool { return i.port == 99 }},
		{"WithNamespace", func(i *Informer) { WithNamespace("ns")(i) }, func(i *Informer) bool { return i.namespace == "ns" }},
		{"WithLogger", func(i *Informer) { WithLogger(logger.GetTestLogger())(i) }, func(i *Informer) bool { return i.logger != nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i := &Informer{}
			c.apply(i)
			if !c.check(i) {
				t.Errorf("%s did not apply", c.name)
			}
		})
	}
}
