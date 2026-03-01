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
)

const (
	rolePrefix   = "node-role.kubernetes.io/"
	patchTimeout = 15 * time.Second
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

// EnsureRole checks if the Node has the correct role label and patches it if necessary.
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

	// Check if the node has the expected role label
	val, ok := n.Labels[h.roleLabel]
	if !ok {
		h.logger.Debug("node does not have the expected label",
			zap.String("name", n.Name),
			zap.String("want", h.roleLabel),
		)
		return
	}

	h.logger.Debug("node has the expected label",
		zap.String("name", n.Name),
		zap.String("value", val),
	)

	// Check if the node already has the role label
	roleKey := rolePrefix + val
	if _, ok := n.Labels[roleKey]; ok {
		h.logger.Debug("node already has the role label",
			zap.String("node", n.Name),
			zap.String("roleKey", roleKey),
		)
		return
	}

	// Setup the labels to patch: non-nil pointer sets the label, nil deletes it
	labels := map[string]*string{
		roleKey: ptr(""),
	}

	if h.replace {
		for k := range n.Labels {
			if strings.HasPrefix(k, rolePrefix) {
				h.logger.Debug("node already has a role label, deleting",
					zap.String("node", n.Name),
					zap.String("roleKey", k),
				)
				labels[k] = nil
			}
		}
	}

	patchCtx, cancel := context.WithTimeout(ctx, patchTimeout)
	defer cancel()

	patchData, err := makePatchMetadata(labels)
	if err != nil {
		h.logger.Error("failed to create patch metadata",
			zap.String("node", n.Name),
			zap.Error(err),
		)
		return
	}

	op := func() error {
		if _, patchErr := h.patcher(
			patchCtx, n.Name,
			types.StrategicMergePatchType,
			patchData,
			metav1.PatchOptions{},
		); patchErr != nil {
			if apierrors.IsForbidden(patchErr) || apierrors.IsNotFound(patchErr) || apierrors.IsInvalid(patchErr) {
				return backoff.Permanent(fmt.Errorf("non-retryable error patching node %s: %w", n.Name, patchErr))
			}
			return fmt.Errorf("failed to patch node %s: %w", n.Name, patchErr)
		}
		return nil
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = patchTimeout
	if err := backoff.Retry(op, backoff.WithContext(expBackoff, patchCtx)); err != nil {
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
