package node

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backoff "github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
)

const (
	rolePrefix = "node-role.kubernetes.io/"
)

// nodePatcher defines the function signature for patching a Node.
type nodePatcher func(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.Node, err error)

// cacheResourceHandler handles Node events and ensures the correct role label is applied.
// It implements the cache.ResourceEventHandler interface.
type cacheResourceHandler struct {
	patcher   nodePatcher
	logger    *zap.Logger
	roleLabel string
	replace   bool
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

	// setup the labels to patch
	labels := map[string]string{
		roleKey: "",
	}

	if h.replace {
		for k := range n.Labels {
			if strings.HasPrefix(k, rolePrefix) {
				h.logger.Debug("node already has a role label, deleting", zap.String("node", n.Name), zap.String("roleKey", k))
				// delete the existing role label
				labels[k] = "nill"
				break
			}
		}
	}

	// patch the node with the new roles
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	op := func() error {
		_, err := h.patcher(ctx, n.Name, types.StrategicMergePatchType, makePatchMetadata(labels), metav1.PatchOptions{})
		return err
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 15 * time.Second // Limit total retry duration
	if err := backoff.Retry(op, expBackoff); err != nil {
		incFailureMetric()
		h.logger.Error("patch node failed after backoff", zap.String("node", n.Name), zap.Error(err))
		return
	}

	h.logger.Info("node role label patched successfully", zap.String("node", n.Name), zap.String("roleKey", roleKey))
	incSuccessMetric()
}

// makePatchMetadata creates a patch metadata for the given roles.
func makePatchMetadata(roles map[string]string) []byte {
	patch := `{"metadata":{"labels":{`
	for k, v := range roles {
		if patch[len(patch)-1] != '{' {
			patch += ","
		}
		patch += `"` + k + `":"` + v + `"`
	}
	patch += "}}}"
	return []byte(patch)
}
