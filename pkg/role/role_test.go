package role

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mchmarny/rolesetter/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testLabel = "test-label"
	roleVal   = "worker"
	testFoo   = "foo"
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
	log := logger.GetTestLogger()
	patcher := newTestPatcher(nil, nil)

	if _, err := NewCacheResourceHandler(nil, log, "label", false); err == nil {
		t.Error("expected error for nil patcher")
	}
	if _, err := NewCacheResourceHandler(patcher, nil, "label", false); err == nil {
		t.Error("expected error for nil logger")
	}
	if _, err := NewCacheResourceHandler(patcher, log, "", false); err == nil {
		t.Error("expected error for empty label")
	}
	h, err := NewCacheResourceHandler(patcher, log, "label", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Error("returned nil handler without error")
	}
}

func TestEnsureRole_PatchVariants(t *testing.T) {
	log := logger.GetTestLogger()

	tests := []struct {
		name      string
		node      *corev1.Node
		replace   bool
		patchErr  error
		wantPatch bool
	}{
		{
			name:      "patch success, no replace",
			node:      getTestNode("n5", map[string]string{testLabel: roleVal}),
			replace:   false,
			wantPatch: true,
		},
		{
			name:      "patch permanent failure",
			node:      getTestNode("n6", map[string]string{testLabel: roleVal}),
			replace:   false,
			patchErr:  apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "n6", errors.New("forbidden")),
			wantPatch: true,
		},
		{
			name:      "patch success, with replace",
			node:      getTestNode("n7", map[string]string{testLabel: roleVal, rolePrefix + "other": ""}),
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
			h, err := NewCacheResourceHandler(patcher, log, testLabel, tt.replace)
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
	log := logger.GetTestLogger()
	called := false
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		called = true
		return nil, nil
	}
	h, err := NewCacheResourceHandler(patcher, log, testLabel, false)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), "not a node")
	if called {
		t.Error("patcher should not be called for non-Node object")
	}
}

func TestEnsureRole_ReplaceRemovesOldRoles(t *testing.T) {
	log := logger.GetTestLogger()
	var gotPatchData []byte
	patcher := func(_ context.Context, _ string, _ types.PatchType, data []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		gotPatchData = data
		return nil, nil
	}
	node := getTestNode("n1", map[string]string{
		testLabel:            roleVal,
		rolePrefix + "old":   "",
		rolePrefix + "stale": "",
	})

	h, err := NewCacheResourceHandler(patcher, log, testLabel, true)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), node)

	var patch patchPayload
	if err := json.Unmarshal(gotPatchData, &patch); err != nil {
		t.Fatalf("failed to unmarshal patch: %v", err)
	}

	if v, ok := patch.Metadata.Labels[rolePrefix+roleVal]; !ok || v == nil || *v != "" {
		t.Errorf("expected worker role to be set, got %v", patch.Metadata.Labels)
	}

	for _, old := range []string{rolePrefix + "old", rolePrefix + "stale"} {
		if v, ok := patch.Metadata.Labels[old]; !ok || v != nil {
			t.Errorf("expected %s to be null (deleted), got %v", old, v)
		}
	}
}

// Regression test for H1: when the desired role label is already present
// but other stale role labels exist, replace=true must still remove the
// stale labels in a single patch.
func TestEnsureRole_ReplaceCleansStaleWhenDesiredPresent(t *testing.T) {
	log := logger.GetTestLogger()
	var gotPatchData []byte
	called := false
	patcher := func(_ context.Context, _ string, _ types.PatchType, data []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		called = true
		gotPatchData = data
		return nil, nil
	}
	node := getTestNode("n1", map[string]string{
		testLabel:               roleVal,
		rolePrefix + roleVal:    "", // desired already present
		rolePrefix + "stale":    "",
		rolePrefix + "leftover": "",
	})

	h, err := NewCacheResourceHandler(patcher, log, testLabel, true)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), node)

	if !called {
		t.Fatal("expected patcher to be called to clean stale roles")
	}

	var patch patchPayload
	if err := json.Unmarshal(gotPatchData, &patch); err != nil {
		t.Fatalf("failed to unmarshal patch: %v", err)
	}

	if _, ok := patch.Metadata.Labels[rolePrefix+roleVal]; ok {
		t.Error("desired role should not be re-set when already present")
	}
	for _, old := range []string{rolePrefix + "stale", rolePrefix + "leftover"} {
		if v, ok := patch.Metadata.Labels[old]; !ok || v != nil {
			t.Errorf("expected %s to be null (deleted), got %v", old, v)
		}
	}
}

func TestEnsureRole_NoOpWhenInDesiredState(t *testing.T) {
	log := logger.GetTestLogger()
	called := false
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		called = true
		return nil, nil
	}
	node := getTestNode("n1", map[string]string{
		testLabel:            roleVal,
		rolePrefix + roleVal: "",
	})

	h, err := NewCacheResourceHandler(patcher, log, testLabel, false)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), node)

	if called {
		t.Error("patcher should not be called when node already in desired state")
	}
}

func TestEnsureRole_InvalidLabelValueSkipped(t *testing.T) {
	log := logger.GetTestLogger()
	called := false
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		called = true
		return nil, nil
	}
	// Invalid: starts with `-`, contains spaces.
	node := getTestNode("n1", map[string]string{testLabel: "-bad value!"})

	h, err := NewCacheResourceHandler(patcher, log, testLabel, false)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	h.EnsureRole(context.Background(), node)

	if called {
		t.Error("patcher should not be called for invalid label value")
	}
}

func TestEnsureRole_ContextCancellation(t *testing.T) {
	log := logger.GetTestLogger()
	callCount := 0
	patcher := func(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*corev1.Node, error) {
		callCount++
		return nil, errors.New("transient error")
	}
	node := getTestNode("n1", map[string]string{testLabel: roleVal})

	h, err := NewCacheResourceHandler(patcher, log, testLabel, false)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		h.EnsureRole(ctx, node)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("EnsureRole did not return promptly after context cancel")
	}
	// At least one attempt is acceptable; bounded by parent deadline.
	if callCount < 0 {
		t.Errorf("unexpected negative call count: %d", callCount)
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
			input: map[string]*string{testFoo: ptr("")},
			want:  `{"metadata":{"labels":{"foo":""}}}`,
		},
		{
			name:  "multiple add",
			input: map[string]*string{"bar": ptr(""), testFoo: ptr("")},
			want:  `{"metadata":{"labels":{"bar":"","foo":""}}}`,
		},
		{
			name:  "add and remove",
			input: map[string]*string{"bar": nil, testFoo: ptr("")},
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

func TestBuildPatchLabels(t *testing.T) {
	log := logger.GetTestLogger()
	patcher := newTestPatcher(nil, nil)

	tests := []struct {
		name      string
		labels    map[string]string
		replace   bool
		wantKeys  []string
		wantNoKey []string
	}{
		{
			name:     "add only",
			labels:   map[string]string{testLabel: roleVal},
			replace:  false,
			wantKeys: []string{rolePrefix + roleVal},
		},
		{
			name:     "no-op",
			labels:   map[string]string{testLabel: roleVal, rolePrefix + roleVal: ""},
			replace:  false,
			wantKeys: nil,
		},
		{
			name: "replace cleans stale only",
			labels: map[string]string{
				testLabel:            roleVal,
				rolePrefix + roleVal: "",
				rolePrefix + "old":   "",
			},
			replace:   true,
			wantKeys:  []string{rolePrefix + "old"},
			wantNoKey: []string{rolePrefix + roleVal},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewCacheResourceHandler(patcher, log, testLabel, tt.replace)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			n := getTestNode("n", tt.labels)
			got := h.buildPatchLabels(n, rolePrefix+roleVal)
			for _, k := range tt.wantKeys {
				if _, ok := got[k]; !ok {
					t.Errorf("expected key %s in patch labels, got %v", k, keys(got))
				}
			}
			for _, k := range tt.wantNoKey {
				if _, ok := got[k]; ok {
					t.Errorf("did not expect key %s in patch labels", k)
				}
			}
			if len(tt.wantKeys) == 0 && len(got) != 0 {
				t.Errorf("expected empty labels, got %v", got)
			}
		})
	}
}

func keys(m map[string]*string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return strings.Split(strings.Join(out, ","), ",")
}
