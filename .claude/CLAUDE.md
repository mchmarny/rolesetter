# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Rolesetter is a Kubernetes controller that automatically assigns `node-role.kubernetes.io/<role>` labels to nodes based on a configurable source label. Solves the restriction where `kubeadm`'s NodeRestriction admission controller prevents kubelet from self-applying `node-role.kubernetes.io/*` labels.

**How it works:** Watches nodes via informer → reads source label value (e.g., `nodeGroup=gpu-worker`) → patches node with `node-role.kubernetes.io/gpu-worker` label. Uses leader election when `NAMESPACE` is set.

**Tech Stack:** Go 1.26, Kubernetes 1.33+, Ko for images, golangci-lint v2.4.0

## Commands

```bash
# Development workflow
make pre          # Full check: tidy + lint + test + vet + helm-lint (run before PR)
make test         # Unit tests with coverage
make lint         # golangci-lint + yamllint
make vet          # go vet with shadow checking
make tidy         # Format + update deps
make build        # Build binary to bin/node-role-controller
make helm-lint    # Lint the Helm chart

# Run single test
go test -v ./pkg/role/... -run TestSpecificFunction

# Local Kubernetes (KinD)
make up           # Create KinD cluster (2 workers, labeled role=worker)
make down         # Delete KinD cluster
make integration  # Run integration tests in KinD

# Dependencies
make upgrade      # Upgrade all Go dependencies

# Release
make tag          # Create signed git tag (uses APP_VERSION from Makefile)
```

## Non-Negotiable Rules

1. **Read before writing** — Never modify code you haven't read
2. **Tests must pass** — `make test` with coverage; never skip tests
3. **Run `make pre` often** — Run at every stopping point. Fix ALL lint/test failures before proceeding
4. **Use project patterns** — Learn existing code before inventing new approaches
5. **3-strike rule** — After 3 failed fix attempts, stop and reassess

## Git Configuration

- Commit to `main` branch (not `master`)
- Do use `-S` to cryptographically sign the commit
- Do NOT add `Co-Authored-By` lines (organization policy)
- Do not sign-off commits (no `-s` flag)

## Key Packages

| Package | Purpose |
|---------|---------|
| `pkg/node` | Controller entry point, informer, leader election, signal handling |
| `pkg/role` | Node role patching with backoff, permanent error detection, JSON patches |
| `pkg/log` | Zap logger factory (production, debug, test modes) |
| `pkg/metric` | Prometheus counter metrics with safe re-registration |
| `pkg/server` | HTTP server for metrics (`/metrics`), health (`/healthz`), readiness (`/readyz`) |

**Architecture:**

```
main.go → pkg/node.InformNodeRoles()
  → reads env vars (ROLE_LABEL, NAMESPACE, etc.)
  → creates Informer with functional options
  → if NAMESPACE set: leader election via Lease → runInformer
  → if NAMESPACE empty: runInformer directly
  → runInformer: creates single CacheResourceHandler → watches nodes → EnsureRole(ctx, obj)
  → metrics server runs always (regardless of leadership)
```

## Required Patterns

**Functional options (configuration):**
```go
inf, err := NewInformer(
    WithLogger(logger),
    WithLabel(roleLabel),
    WithPort(port),
    WithReplace(replace),
    WithNamespace(namespace),
)
```

**Constructor with validation (handler creation):**
```go
handler, err := role.NewCacheResourceHandler(patcher, logger, label, replace)
```

**Context propagation (always pass ctx to I/O):**
```go
func (h *CacheResourceHandler) EnsureRole(ctx context.Context, obj interface{}) {
    patchCtx, cancel := context.WithTimeout(ctx, patchTimeout)
    defer cancel()
    // ...
}
```

**Permanent error detection (avoid retrying non-transient errors):**
```go
if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) || apierrors.IsInvalid(err) {
    return backoff.Permanent(fmt.Errorf("non-retryable: %w", err))
}
```

**JSON patch via encoding/json (not string concatenation):**
```go
type patchPayload struct {
    Metadata patchMetadata `json:"metadata"`
}
type patchMetadata struct {
    Labels map[string]*string `json:"labels"` // nil pointer = delete
}
```

**Structured logging (zap):**
```go
logger.Debug("processing node", zap.String("name", n.Name))
logger.Error("patch failed", zap.Error(err), zap.String("node", n.Name))
```

**Table-driven tests:**
```go
tests := []struct {
    name    string
    input   string
    wantErr bool
}{
    {"valid", "test", false},
    {"empty", "", true},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { /* ... */ })
}
```

## Deployment

**Helm chart** in `chart/` — OCI-native, published to `oci://ghcr.io/mchmarny/node-role-controller` on tag.

**Kustomize-based** with overlays:
- `deployment/base/` — Namespace, ServiceAccount, RBAC, ConfigMap, Deployment
- `deployment/overlays/prod/` — Production patches
- `deployment/overlays/dev/` — Development patches (with image override)
- `deployment/manifest.yaml` — Pre-built manifest (regenerate via `kubectl kustomize deployment/base`)

**Container image:** `ghcr.io/mchmarny/node-role-controller` built with Ko on `cgr.dev/chainguard/static:latest` (multi-arch: amd64, arm64)

**Environment variables:**
- `ROLE_LABEL` (required) — Source label to watch (e.g., `nodeGroup`)
- `ROLE_LABEL_REPLACE` — Replace existing role labels (`true`/`false`)
- `NAMESPACE` — Enables leader election via Lease in this namespace (injected via downward API)
- `SERVER_PORT` — Metrics server port (default: `8080`)
- `LOG_LEVEL` — Logging level (`debug`/`info`/`warn`/`error`, default: `info`)

## CI/CD

**On Push/PR** (`on-push.yaml`): Test + lint + vulnerability scan (with concurrency group, 15min timeout)
**On Tag** (`on-tag.yaml`): Build image → integration test in KinD → Trivy scan + SARIF upload → SLSA provenance → verification

All actions SHA-pinned. SLSA reusable workflow uses tagged version (GitHub requirement).

## Anti-Patterns (Do Not Do)

| Anti-Pattern | Correct Approach |
|--------------|------------------|
| Modify code without reading it first | Always `Read` files before `Edit` |
| Skip or disable tests to make CI pass | Fix the actual issue |
| Invent new patterns | Study existing code in same package first |
| Use `context.Background()` in I/O without timeout | Use `context.WithTimeout()` |
| Build JSON via string concatenation | Use `encoding/json.Marshal` with typed structs |
| Retry permanent errors (403, 404) | Wrap with `backoff.Permanent` |
| Create handler per event | Create once, reuse across callbacks |
| Add features not requested | Implement exactly what was asked |
| Create new files when editing suffices | Prefer `Edit` over `Write` |
| Continue after 3 failed fix attempts | Stop, reassess approach, explain blockers |

## Key Files

| File | Purpose |
|------|---------|
| `Makefile` | Build, test, lint, dev cluster commands |
| `.golangci.yaml` | Linter configuration (20+ linters) |
| `.yamllint` | YAML linter configuration |
| `ko.yaml` | Ko image builder config |
| `kind.yaml` | KinD test cluster (1 control-plane, 2 workers) |
| `deployment/` | Kustomize manifests (base + overlays) |
| `deployment/manifest.yaml` | Pre-built manifest for non-kustomize users |
| `tests/integration` | Integration test script for KinD |
| `chart/` | Helm chart (OCI-published to ghcr.io) |
| `CONTRIBUTING.md` | Contribution guidelines, DCO |
