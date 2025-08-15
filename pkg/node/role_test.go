package node

import (
	"testing"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnsureRole_NonNodeObject(_ *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := &cacheResourceHandler{logger: logger, roleLabel: "test-role"}
	h.ensureRole("not-a-node") // Should log a warning, no panic
}

func TestEnsureRole_NodeMissingLabel(_ *testing.T) {
	logger, _ := zap.NewDevelopment()
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{}}}
	h := &cacheResourceHandler{logger: logger, roleLabel: "test-role"}
	h.ensureRole(n) // Should log debug and return
}

func TestEnsureRole_NodeAlreadyHasRoleLabel(_ *testing.T) {
	logger, _ := zap.NewDevelopment()
	roleKey := rolePrefix + "test-role"
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{"test-role": "test-role", roleKey: ""}}}
	h := &cacheResourceHandler{logger: logger, roleLabel: "test-role"}
	h.ensureRole(n) // Should log debug and return
}
