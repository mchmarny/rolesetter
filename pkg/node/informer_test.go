package node

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestNewInformer_Validation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Missing label
	_, err := NewInformer(logger, "", 8080)
	if err == nil {
		t.Error("expected error for missing label")
	}

	// Invalid port
	_, err = NewInformer(logger, "test-label", 0)
	if err == nil {
		t.Error("expected error for invalid port")
	}

	// Valid config
	inf, err := NewInformer(logger, "test-label", 8080)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if inf == nil {
		t.Error("expected informer instance")
	}
}

func TestInformer_Validate(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	inf := &Informer{logger: logger, label: "test-label", port: 8080}
	if err := inf.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInformer_Inform_ContextCancel(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	inf := &Informer{logger: logger, label: "test-label", port: 8080}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	// Inform should return quickly due to context cancellation
	err := inf.Inform(ctx)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}
