package role

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mchmarny/rolesetter/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func getTestNode(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func newTestPatcher(retNode *corev1.Node, retErr error) NodePatcher {
	return func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		return retNode, retErr
	}
}

func TestNewCacheResourceHandler_Validation(t *testing.T) {
	logger := logger.GetTestLogger()
	patcher := newTestPatcher(nil, nil)

	if _, err := NewCacheResourceHandler(nil, logger, "label", false); err == nil {
		t.Error("expected error for nil patcher")
	}
	if _, err := NewCacheResourceHandler(patcher, nil, "label", false); err == nil {
		t.Error("expected error for nil logger")
	}
	if _, err := NewCacheResourceHandler(patcher, logger, "", false); err == nil {
		t.Error("expected error for empty label")
	}
	h, err := NewCacheResourceHandler(patcher, logger, "label", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Error("returned nil handler without error")
	}
}

func TestEnsureRole_PatchVariants(t *testing.T) {
	logger := logger.GetTestLogger()
	val := "worker"

	tests := []struct {
		name      string
		node      *corev1.Node
		replace   bool
		patchErr  error
		wantPatch bool
	}{
		{
			name:      "patch success, no replace",
			node:      getTestNode("n5", map[string]string{"test-label": val}),
			replace:   false,
			wantPatch: true,
		},
		{
			name:      "patch permanent failure",
			node:      getTestNode("n6", map[string]string{"test-label": val}),
			replace:   false,
			patchErr:  apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "n6", errors.New("forbidden")),
			wantPatch: true,
		},
		{
			name:      "patch success, with replace",
			node:      getTestNode("n7", map[string]string{"test-label": val, rolePrefix + "other": ""}),
			replace:   true,
			wantPatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			var gotName string
			var gotLabels []byte
			patcher := func(_ context.Context, name string, _ types.PatchType, data []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
				called = true
				gotName = name
				gotLabels = data
				return tt.node, tt.patchErr
			}
			h, err := NewCacheResourceHandler(patcher, logger, "test-label", tt.replace)
			if err != nil {
				t.Fatalf("failed to create handler: %v", err)
			}
			h.EnsureRole(context.Background(), tt.node)
			if tt.wantPatch && !called {
				t.Error("patcher was not called when expected")
			}
			if called && gotName != tt.node.Name {
				t.Errorf("patcher called with wrong node name: got %s, want %s", gotName, tt.node.Name)
			}
			if called && len(gotLabels) == 0 {
				t.Error("patcher called with empty patch data")
			}
		})
	}
}

func TestEnsureRole_NonNodeObject(t *testing.T) {
	logger := logger.GetTestLogger()
	called := false
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		called = true
		return nil, nil
	}
	h, err := NewCacheResourceHandler(patcher, logger, "test-label", false)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), "not a node")
	if called {
		t.Error("patcher should not be called for non-Node object")
	}
}

func TestEnsureRole_ReplaceRemovesOldRoles(t *testing.T) {
	logger := logger.GetTestLogger()
	var gotPatchData []byte
	patcher := func(_ context.Context, _ string, _ types.PatchType, data []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		gotPatchData = data
		return nil, nil
	}
	node := getTestNode("n1", map[string]string{
		"test-label":         "worker",
		rolePrefix + "old":   "",
		rolePrefix + "stale": "",
	})

	h, err := NewCacheResourceHandler(patcher, logger, "test-label", true)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), node)

	var patch patchPayload
	if err := json.Unmarshal(gotPatchData, &patch); err != nil {
		t.Fatalf("failed to unmarshal patch: %v", err)
	}

	// New role should be set
	if v, ok := patch.Metadata.Labels[rolePrefix+"worker"]; !ok || v == nil || *v != "" {
		t.Errorf("expected worker role to be set, got %v", patch.Metadata.Labels)
	}

	// Old roles should be null (deleted)
	for _, old := range []string{rolePrefix + "old", rolePrefix + "stale"} {
		if v, ok := patch.Metadata.Labels[old]; !ok || v != nil {
			t.Errorf("expected %s to be null (deleted), got %v", old, v)
		}
	}
}

func TestEnsureRole_ContextCancellation(t *testing.T) {
	logger := logger.GetTestLogger()
	callCount := 0
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		callCount++
		return nil, errors.New("transient error")
	}
	node := getTestNode("n1", map[string]string{"test-label": "worker"})

	h, err := NewCacheResourceHandler(patcher, logger, "test-label", false)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	h.EnsureRole(ctx, node)
	// Should have retried at least once but stopped due to context timeout
	if callCount < 1 {
		t.Error("expected at least one patch attempt")
	}
}

func TestMakePatchMetadata(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]*string
		want  string
	}{
		{
			name:  "single add",
			input: map[string]*string{"foo": ptr("")},
			want:  `{"metadata":{"labels":{"foo":""}}}`,
		},
		{
			name:  "multiple add",
			input: map[string]*string{"bar": ptr(""), "foo": ptr("")},
			want:  `{"metadata":{"labels":{"bar":"","foo":""}}}`,
		},
		{
			name:  "add and remove",
			input: map[string]*string{"bar": nil, "foo": ptr("")},
			want:  `{"metadata":{"labels":{"bar":null,"foo":""}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makePatchMetadata(tt.input)
			if err != nil {
				t.Fatalf("makePatchMetadata() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("makePatchMetadata() = %s, want %s", got, tt.want)
			}
		})
	}
}
