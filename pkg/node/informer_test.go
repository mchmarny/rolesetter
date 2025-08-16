package node

import (
	"context"
	"testing"

	"github.com/mchmarny/rolesetter/pkg/log"
	"github.com/mchmarny/rolesetter/pkg/server"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInformer_Validate(t *testing.T) {
	logger := log.GetTestLogger()
	clientset := fake.NewSimpleClientset()
	srv := server.NewServer(
		server.WithLogger(logger),
		server.WithPort(8080),
	)

	inf := &Informer{
		logger:    logger,
		label:     "test-label",
		port:      8080,
		clientset: clientset,
		server:    srv,
	}
	if err := inf.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInformer_Inform_ContextCancel(t *testing.T) {
	logger := log.GetTestLogger()
	clientset := fake.NewSimpleClientset()
	srv := server.NewServer(
		server.WithLogger(logger),
		server.WithPort(8080),
	)

	inf := &Informer{
		logger:    logger,
		label:     "test-label",
		port:      8080,
		clientset: clientset,
		server:    srv,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := inf.Inform(ctx)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}
