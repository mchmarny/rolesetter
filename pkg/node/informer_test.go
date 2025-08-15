package node

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInformer_Validate(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	clientset := fake.NewSimpleClientset()

	inf := &Informer{
		logger:    logger,
		label:     "test-label",
		port:      8080,
		clientset: clientset,
	}
	if err := inf.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInformer_Inform_ContextCancel(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	clientset := fake.NewSimpleClientset()

	inf := &Informer{
		logger:    logger,
		label:     "test-label",
		port:      8080,
		clientset: clientset,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := inf.Inform(ctx)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}
