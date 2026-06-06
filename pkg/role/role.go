// Package role provides the node role reconciliation logic.
//
// EnsureRole reads a source label on a Node and applies a corresponding
// node-role.kubernetes.io/<value> label via strategic merge patch with
// bounded exponential backoff. When replace is enabled, stale role labels
// (other than the desired one) are removed in the same patch.
package role

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/mchmarny/rolesetter/pkg/metric"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	rolePrefix     = "node-role.kubernetes.io/"
	patchTimeout   = 15 * time.Second
	attemptTimeout = 5 * time.Second
)

// NodePatcher defines the function signature for patching a Node.
type NodePatcher func(
	ctx context.Context,
	name string,
	pt types.PatchType,
	data []byte,
	opts metav1.PatchOptions,
	subresources ...string) (result *corev1.Node, err error)

// CacheResourceHandler handles Node events and ensures the correct role label is applied.
type CacheResourceHandler struct {
	patcher   NodePatcher
	logger    *zap.Logger
	roleLabel string
	replace   bool
}

// NewCacheResourceHandler creates a validated CacheResourceHandler.
func NewCacheResourceHandler(patcher NodePatcher, logger *zap.Logger, roleLabel string, replace bool) (*CacheResourceHandler, error) {
	if patcher == nil {
		return nil, fmt.Errorf("patcher must not be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	if roleLabel == "" {
		return nil, fmt.Errorf("role label must not be empty")
	}
	return &CacheResourceHandler{
		patcher:   patcher,
		logger:    logger,
		roleLabel: roleLabel,
		replace:   replace,
	}, nil
}

var (
	successCounter = metric.NewCounter("node_role_patch_success_total", "Total number of successful node role patches", "role")
	failureCounter = metric.NewCounter("node_role_patch_failure_total", "Total number of failed node role patches", "role")
)

// EnsureRole reconciles the node-role.kubernetes.io/<value> label on the
// supplied Node toward the value of the configured source label. It is safe
// to call on any informer event payload; non-Node objects are ignored.
//
// In replace mode, any other node-role.kubernetes.io/* labels are removed in
// the same patch. The function is a no-op when the desired state already
// holds (no patch is issued).
func (h *CacheResourceHandler) EnsureRole(ctx context.Context, obj interface{}) {
	n, ok := obj.(*corev1.Node)
	if !ok {
		h.logger.Warn("object is not a Node")
		return
	}

	h.logger.Debug("processing role for node",
		zap.String("name", n.Name),
		zap.String("label", h.roleLabel),
	)

	val, ok := n.Labels[h.roleLabel]
	if !ok {
		h.logger.Debug("node does not have the expected label",
			zap.String("name", n.Name),
			zap.String("want", h.roleLabel),
		)
		return
	}

	roleKey := rolePrefix + val
	if errs := validation.IsQualifiedName(roleKey); len(errs) > 0 {
		h.logger.Warn("invalid role label value; skipping",
			zap.String("node", n.Name),
			zap.String("value", val),
			zap.Strings("errors", errs),
		)
		failureCounter.Increment(val)
		return
	}

	labels := h.buildPatchLabels(n, roleKey)
	if len(labels) == 0 {
		h.logger.Debug("node already in desired role state",
			zap.String("node", n.Name),
			zap.String("roleKey", roleKey),
		)
		return
	}

	patchData, err := makePatchMetadata(labels)
	if err != nil {
		h.logger.Error("failed to create patch metadata",
			zap.String("node", n.Name),
			zap.Error(err),
		)
		failureCounter.Increment(val)
		return
	}

	patchCtx, cancel := context.WithTimeout(ctx, patchTimeout)
	defer cancel()

	if err := h.patchWithBackoff(patchCtx, n.Name, patchData); err != nil {
		failureCounter.Increment(val)
		h.logger.Error("patch node failed after backoff",
			zap.String("node", n.Name),
			zap.String("roleKey", roleKey),
			zap.Bool("replace", h.replace),
			zap.Error(err),
		)
		return
	}

	successCounter.Increment(val)
	h.logger.Info("node role label patched successfully",
		zap.String("node", n.Name),
		zap.String("roleKey", roleKey),
		zap.Bool("replace", h.replace),
	)
}

// buildPatchLabels returns the label-set the strategic merge patch should
// apply to converge the node toward the desired role. An empty map signals
// "nothing to do" — the desired label is present and no stale role labels
// need removing.
func (h *CacheResourceHandler) buildPatchLabels(n *corev1.Node, roleKey string) map[string]*string {
	labels := map[string]*string{}
	if _, ok := n.Labels[roleKey]; !ok {
		labels[roleKey] = ptr("")
	}
	if h.replace {
		for k := range n.Labels {
			if strings.HasPrefix(k, rolePrefix) && k != roleKey {
				labels[k] = nil
			}
		}
	}
	return labels
}

// patchWithBackoff issues the strategic-merge patch under a bounded
// exponential backoff. Each attempt receives its own short timeout so a
// single slow API call cannot starve subsequent retries.
func (h *CacheResourceHandler) patchWithBackoff(ctx context.Context, name string, patchData []byte) error {
	op := func() error {
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		defer cancel()
		if _, patchErr := h.patcher(
			attemptCtx, name,
			types.StrategicMergePatchType,
			patchData,
			metav1.PatchOptions{},
		); patchErr != nil {
			if apierrors.IsForbidden(patchErr) || apierrors.IsNotFound(patchErr) || apierrors.IsInvalid(patchErr) {
				return backoff.Permanent(fmt.Errorf("non-retryable error patching node %s: %w", name, patchErr))
			}
			return fmt.Errorf("failed to patch node %s: %w", name, patchErr)
		}
		return nil
	}

	// Defaults already include randomization (jitter 0.5); rely on the
	// parent context for the overall deadline so all callers share one
	// budget definition.
	expBackoff := backoff.NewExponentialBackOff()
	return backoff.Retry(op, backoff.WithContext(expBackoff, ctx))
}

func ptr(s string) *string {
	return &s
}

// patchPayload represents the JSON structure for a Kubernetes strategic merge patch.
type patchPayload struct {
	Metadata patchMetadata `json:"metadata"`
}

type patchMetadata struct {
	Labels map[string]*string `json:"labels"`
}

// makePatchMetadata creates a JSON patch for the given role labels.
// A non-nil string pointer sets the label; a nil pointer deletes it.
func makePatchMetadata(labels map[string]*string) ([]byte, error) {
	return json.Marshal(patchPayload{
		Metadata: patchMetadata{Labels: labels},
	})
}
