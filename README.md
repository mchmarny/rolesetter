# Kubernetes Node Role Labeler

Kubernetes controller that automatically assigns node roles based on a configurable label value (e.g., `nodeGroup=gpu-worker` becomes `node-role.kubernetes.io/gpu-worker`).

## Why

By default, `kubeadm` enables the NodeRestriction admission controller that restricts what labels `kubelet` can self-apply on node registration. The `node-role.kubernetes.io/*` label is restricted and can't be set in cloud init scripts or during other node bootstrap processes.

## Install

### Helm (recommended)

```shell
helm install node-role-controller oci://ghcr.io/mchmarny/node-role-controller \
  --namespace node-labeler --create-namespace
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
helm install node-role-controller oci://ghcr.io/mchmarny/node-role-controller \
  --namespace node-labeler --create-namespace \
  --set config.roleLabel=nodeGroup \
  --set config.roleReplace=true \
  --set config.logLevel=debug
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `config.roleLabel` | `nodeGroup` | Source label whose value becomes the node role |
| `config.roleReplace` | `false` | Replace existing `node-role.kubernetes.io/*` labels |
| `config.logLevel` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `replicas` | `1` | Number of controller replicas (leader election enabled) |
| `image.tag` | Chart `appVersion` | Override the image tag |
| `resources.requests.cpu` | `50m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.cpu` | `250m` | CPU limit |
| `resources.limits.memory` | `256Mi` | Memory limit |
| `tolerations` | `[]` | Pod tolerations |
| `nodeSelector` | `{}` | Pod node selector |

> After changing configuration, restart to apply: `kubectl -n node-labeler rollout restart deployment node-role-controller`

## Upgrade

```shell
helm upgrade node-role-controller oci://ghcr.io/mchmarny/node-role-controller \
  --namespace node-labeler
```

## Uninstall

```shell
helm uninstall node-role-controller -n node-labeler
```

## How It Works

1. Nodes are labeled with a source label (e.g., `nodeGroup=gpu-worker`)
2. The controller watches node add/update events via a Kubernetes informer
3. When a node has the source label, the controller patches it with `node-role.kubernetes.io/<value>`
4. Leader election via Lease ensures only one replica is active

**Example:** A node with `nodeGroup=gpu-worker` gets `node-role.kubernetes.io/gpu-worker`.

## Metrics

| Metric | Description |
|--------|-------------|
| `node_role_patch_success_total` | Successful patch operations (labeled by role) |
| `node_role_patch_failure_total` | Failed patch operations (labeled by role) |

Available at `/metrics` on port `8080`. Health at `/healthz`, readiness at `/readyz`.

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
