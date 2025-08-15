package node

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const testWorkerLabel = "worker"

func TestEnsureRole_NonNodeObject(t *testing.T) {
	logger := zaptest.NewLogger(t)
	h := &cacheResourceHandler{
		patcher:   nil,
		logger:    logger,
		roleLabel: "test-label",
	}
	h.ensureRole("not-a-node") // Should log a warning, no panic
}

func TestEnsureRole_NodeMissingLabel(t *testing.T) {
	logger := zaptest.NewLogger(t)
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{}}}
	h := &cacheResourceHandler{
		patcher:   nil,
		logger:    logger,
		roleLabel: "test-label",
	}
	h.ensureRole(n) // Should log debug and return
}

func TestEnsureRole_NodeAlreadyHasRoleLabel(t *testing.T) {
	logger := zaptest.NewLogger(t)
	val := testWorkerLabel
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{"test-label": val, rolePrefix + val: ""}}}
	h := &cacheResourceHandler{
		patcher:   nil,
		logger:    logger,
		roleLabel: "test-label",
	}
	h.ensureRole(n) // Should log debug and return
}

func TestEnsureRole_PatchSuccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	val := testWorkerLabel
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"test-label": val}}}
	called := false
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		called = true
		return n, nil
	}
	h := &cacheResourceHandler{
		patcher:   patcher,
		logger:    logger,
		roleLabel: "test-label",
	}
	h.ensureRole(n)
	if !called {
		t.Error("patcher was not called for patch success case")
	}
}

func TestEnsureRole_PatchFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	val := testWorkerLabel
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n4", Labels: map[string]string{"test-label": val}}}
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		return nil, errors.New("patch failed")
	}
	h := &cacheResourceHandler{
		patcher:   patcher,
		logger:    logger,
		roleLabel: "test-label",
	}
	h.ensureRole(n) // Should log error
}
