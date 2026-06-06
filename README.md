# Kubernetes Node Role Labeler

Kubernetes controller that automatically assigns node roles based on a configurable label value (e.g., `nodeGroup=gpu-worker` becomes `node-role.kubernetes.io/gpu-worker`).

## Why

By default, `kubeadm` enables the NodeRestriction admission controller that restricts what labels `kubelet` can self-apply on node registration. The `node-role.kubernetes.io/*` label is restricted and can't be set in cloud init scripts or during other node bootstrap processes.

## How it works

1. Nodes are labeled with a source label (e.g., `nodeGroup=gpu-worker`).
2. The controller watches `Node` add/update events via a Kubernetes informer, scoped to nodes that carry the source label.
3. When a node has the source label, the controller patches it with `node-role.kubernetes.io/<value>` via a strategic merge patch with bounded exponential backoff.
4. In `replace` mode, any stale `node-role.kubernetes.io/*` labels (other than the desired one) are removed in the same patch.
5. Leader election via `coordination.k8s.io/Lease` ensures only one replica reconciles at a time when running with `replicas >= 2`.

Readiness (`/readyz`) returns 503 until the informer cache has synced — and, when leader election is enabled, until the replica is the active leader.

**Example:** A node with `nodeGroup=gpu-worker` gets `node-role.kubernetes.io/gpu-worker`.

## Install

The Helm chart and the prebuilt manifest install into different namespaces by default:

| Install method | Default namespace |
|---|---|
| Helm (recommended) | whatever you pass via `-n` (the README uses `node-role-controller`) |
| `deployment/manifest.yaml` and Kustomize overlays | `node-labeler` (encoded in the manifests) |

### Helm (recommended)

`helm upgrade --install` works for both fresh installs and upgrades:

```shell
helm upgrade --install node-role-controller \
  oci://ghcr.io/mchmarny/node-role-controller \
  -n node-role-controller --create-namespace
```

To schedule on tainted nodes, add tolerations:

```shell
helm upgrade --install node-role-controller \
  oci://ghcr.io/mchmarny/node-role-controller \
  -n node-role-controller --create-namespace \
  --set-json 'tolerations=[{"key":"dedicated","value":"system-workload","operator":"Equal","effect":"NoExecute"},{"key":"dedicated","value":"system-workload","operator":"Equal","effect":"NoSchedule"}]'
```

To enable Prometheus Operator scraping and a PodDisruptionBudget:

```shell
helm upgrade --install node-role-controller \
  oci://ghcr.io/mchmarny/node-role-controller \
  -n node-role-controller --create-namespace \
  --set replicas=2 \
  --set serviceMonitor.enabled=true \
  --set podDisruptionBudget.enabled=true
```

### Manifest

```shell
kubectl apply -f https://raw.githubusercontent.com/mchmarny/rolesetter/refs/heads/main/deployment/manifest.yaml
```

### Kustomize

```shell
kubectl apply -k deployment/overlays/prod
```

## Configuration

The controller is configured via environment variables sourced from a ConfigMap. With Helm, set values directly:

```shell
helm upgrade --install node-role-controller \
  oci://ghcr.io/mchmarny/node-role-controller \
  -n node-role-controller --create-namespace \
  --set config.roleLabel=nodeGroup \
  --set config.roleReplace=true \
  --set config.logLevel=debug
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `config.roleLabel` | `nodeGroup` | Source label whose value becomes the node role |
| `config.roleReplace` | `false` | Replace existing `node-role.kubernetes.io/*` labels other than the desired one |
| `config.logLevel` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `replicas` | `1` | Number of controller replicas (leader election enabled) |
| `image.tag` | Chart `appVersion` | Override the image tag |
| `resources.requests.cpu` | `50m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.cpu` | `250m` | CPU limit |
| `resources.limits.memory` | `256Mi` | Memory limit |
| `tolerations` | `[]` | Pod tolerations |
| `nodeSelector` | `{}` | Pod node selector |
| `serviceMonitor.enabled` | `false` | Create a headless Service and Prometheus Operator ServiceMonitor |
| `serviceMonitor.interval` | `30s` | Scrape interval |
| `serviceMonitor.scrapeTimeout` | `10s` | Scrape timeout |
| `serviceMonitor.labels` | `{}` | Extra labels on the ServiceMonitor |
| `podDisruptionBudget.enabled` | `false` | Create a PDB (only effective when `replicas > 1`) |
| `podDisruptionBudget.minAvailable` | `1` | `minAvailable` for the PDB |

Environment variables (when running the binary directly or via the manifests):

| Variable | Default | Description |
|---|---|---|
| `ROLE_LABEL` | _(required)_ | Source label to watch (e.g., `nodeGroup`) |
| `ROLE_LABEL_REPLACE` | `false` | `true`/`false`/`1`/`0`/`yes`/`no` — replace stale role labels |
| `NAMESPACE` | _(unset)_ | Enables leader election via a Lease in this namespace |
| `SERVER_PORT` | `8080` | Metrics/health HTTP server port |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |

> After changing configuration, restart to apply: `kubectl -n node-role-controller rollout restart deployment node-role-controller`

## Uninstall

```shell
helm uninstall node-role-controller -n node-role-controller
```

## Observability

| Endpoint | Purpose |
|---|---|
| `/metrics` | Prometheus metrics |
| `/healthz` | Liveness probe (always 200 once HTTP is listening) |
| `/readyz` | Readiness probe — 503 until cache sync + (if applicable) lease ownership |

| Metric | Description |
|--------|-------------|
| `node_role_patch_success_total{role=...}` | Successful patch operations |
| `node_role_patch_failure_total{role=...}` | Failed patch operations (including permanently invalid label values) |

Default port: `8080`. Override with `SERVER_PORT`.

## Version

The binary prints build metadata injected at release time:

```shell
node-role-controller -version
# node-role-controller v0.7.0 (commit abcdef0, built 2026-06-06T12:34:56Z)
```

## Image Verification

Every release includes [SLSA](https://slsa.dev) provenance attestation:

```shell
export IMAGE=ghcr.io/mchmarny/node-role-controller:v0.6.0

cosign verify-attestation \
    --output json \
    --type slsaprovenance \
    --certificate-identity-regexp 'https://github.com/.*/.*/.github/workflows/.*' \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    $IMAGE
```

To enforce verification in-cluster with the [Sigstore policy controller](https://docs.sigstore.dev/about/overview/):

```shell
kubectl label namespace node-labeler policy.sigstore.dev/include=true
kubectl apply -f policy/clusterimagepolicy.yaml
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Run `make pre` before submitting PRs.

## Disclaimer

This is my personal project and it does not represent my employer. While I do my best to ensure that everything works, I take no responsibility for issues caused by this code.
