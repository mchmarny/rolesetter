package node

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backoff "github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// cacheResourceHandler handles Node events and ensures the correct role label is applied.
// It implements the cache.ResourceEventHandler interface.
type cacheResourceHandler struct {
	cs        *kubernetes.Clientset
	logger    *zap.Logger
	roleLabel string
}

// ensureRole checks if the Node has the correct role label and patches it if necessary.
func (h *cacheResourceHandler) ensureRole(obj interface{}) {
	n, ok := obj.(*corev1.Node)
	if !ok {
		h.logger.Warn("object is not a Node")
		return
	}

	h.logger.Debug("processing role for node", zap.String("name", n.Name), zap.String("label", h.roleLabel))

	// Check if the node has the expected role label
	val, ok := n.Labels[h.roleLabel]
	if !ok {
		h.logger.Debug("node does not have the expected label",
			zap.String("name", n.Name),
			zap.String("want", h.roleLabel))
		return
	}

	h.logger.Debug("node has the expected label", zap.String("name", n.Name), zap.String("value", val))

	// Check if the node already has the role label
	roleKey := rolePrefix + val
	if _, ok := n.Labels[roleKey]; ok {
		h.logger.Debug("node already has the role label", zap.String("node", n.Name), zap.String("roleKey", roleKey))
		return
	}

	// Patch the node to add the role label
	patch := []byte(`{"metadata":{"labels":{"` + roleKey + `":""}}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	op := func() error {
		_, err := h.cs.CoreV1().Nodes().Patch(ctx, n.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return err
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 15 * time.Second // Limit total retry duration
	if err := backoff.Retry(op, expBackoff); err != nil {
		patchFailure.Inc()
		h.logger.Error("patch node failed after backoff", zap.String("node", n.Name), zap.Error(err))
		return
	}

	h.logger.Info("node role label patched successfully", zap.String("node", n.Name), zap.String("roleKey", roleKey))
	patchSuccess.Inc()
}
