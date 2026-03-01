# Kubernetes Node Role Labeler

Kubernetes controller that automatically assigns node roles based on a configurable label value (e.g., `nodeGroup=gpu-worker` becomes `node-role.kubernetes.io/gpu-worker`).

## Why

By default, `kubeadm` enables the NodeRestriction admission controller that restricts what labels `kubelet` can self-apply on node registration. The `node-role.kubernetes.io/*` label is restricted and can't be set in cloud init scripts or during other node bootstrap processes.

## Features

- Watches node add/update events via Kubernetes informer
- Patches nodes with role labels derived from a configurable source label
- Leader election via Lease for safe multi-replica deployments
- Exponential backoff with permanent error detection for patch retries
- Rate-limited Kubernetes API client to prevent API server overload
- Prometheus metrics for successful and failed patch operations
- Health (`/healthz`) and readiness (`/readyz`) endpoints
- Graceful shutdown on SIGINT/SIGTERM with context propagation

## Requirements

- Kubernetes 1.33+
- RBAC permissions: nodes (list, watch, patch) and leases (get, create, update)

## Usage

Update [patch-configmap.yaml](deployment/overlays/prod/patch-configmap.yaml) to configure the source label and behavior:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: node-role-controller-config
  namespace: node-labeler
data:
  roleLabel: "nodeGroup"  # value of this label becomes the node role
  roleReplace: "false"    # whether to replace existing node roles
  logLevel: "info"        # debug, info, warn, error
```

Then apply to the cluster:

```sh
kubectl apply -k deployment/overlays/prod
```

This ensures all nodes with `nodeGroup=customer-gpu` get labeled with `node-role.kubernetes.io/customer-gpu`.

> If you change ConfigMap values after deployment, restart to apply: `kubectl -n node-labeler rollout restart deployment node-role-controller`

Alternatively, apply the prebuilt manifest:

```shell
kubectl apply -f https://raw.githubusercontent.com/mchmarny/rolesetter/refs/heads/main/deployment/manifest.yaml
```

This creates:

* `Namespace` - Isolates the controller resources
* `ServiceAccount` - Authenticates the controller
* `ClusterRole` - Grants node list/watch/patch and lease permissions
* `ClusterRoleBinding` - Links the role to the ServiceAccount
* `ConfigMap` - Defines label, replace, and logging configuration
* `Deployment` - Runs the controller with leader election, health probes, and security hardening

## Helm

Install from the OCI registry:

```shell
helm install node-role-controller oci://ghcr.io/mchmarny/node-role-controller \
  --namespace node-labeler --create-namespace
```

Configure via values:

```shell
helm install node-role-controller oci://ghcr.io/mchmarny/node-role-controller \
  --namespace node-labeler --create-namespace \
  --set config.roleLabel=nodeGroup \
  --set config.roleReplace=true \
  --set replicas=2
```

Uninstall:

```shell
helm uninstall node-role-controller -n node-labeler
```

## Metrics

The controller exposes:

- `node_role_patch_success_total` - Successful node patch operations (labeled by role)
- `node_role_patch_failure_total` - Failed node patch operations (labeled by role)

Available at the `/metrics` endpoint on port `8080`.

## Validation

The image comes with SLSA attestation verifying it was built in this repo. You can verify using [Sigstore](https://docs.sigstore.dev/about/overview/) CLI or the in-cluster policy controller.

### Manual

> Update the image digest to the version you are using.

```shell
export IMAGE=ghcr.io/mchmarny/node-role-controller:v0.5.1

cosign verify-attestation \
    --output json \
    --type slsaprovenance \
    --certificate-identity-regexp 'https://github.com/.*/.*/.github/workflows/.*' \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    $IMAGE
```

### In Cluster

To enforce provenance verification on the `node-labeler` namespace:

```shell
kubectl label namespace node-labeler policy.sigstore.dev/include=true
kubectl apply -f policy/clusterimagepolicy.yaml
```

Test admission:

```shell
kubectl -n node-labeler run test --image=$IMAGE
```

If you don't already have the [Sigstore](https://docs.sigstore.dev/about/overview/) policy controller:

```shell
kubectl create namespace cosign-system
helm repo add sigstore https://sigstore.github.io/helm-charts
helm repo update
helm install policy-controller -n cosign-system sigstore/policy-controller
```

## Demo

> Requires [Kind](https://kind.sigs.k8s.io/)

Create a Kind cluster with multiple nodes:

```shell
make up
```

Check the nodes (workers have no role):

```shell
kubectl get nodes
```

```
NAME                                 STATUS   ROLES           AGE    VERSION
node-role-controller-control-plane   Ready    control-plane   2m9s   v1.33.1
node-role-controller-worker          Ready    <none>          114s   v1.33.1
node-role-controller-worker2         Ready    <none>          114s   v1.33.1
```

Label the workers and deploy:

```shell
kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name | \
  xargs -I {} kubectl label {} nodeGroup=worker --overwrite
kubectl apply -k deployment/overlays/dev
```

After a few seconds, roles appear:

```shell
kubectl get nodes
```

```
NAME                                 STATUS   ROLES                  AGE     VERSION
node-role-controller-control-plane   Ready    control-plane          3m12s   v1.33.1
node-role-controller-worker          Ready    worker                 2m57s   v1.33.1
node-role-controller-worker2         Ready    worker                 2m57s   v1.33.1
```

Change a node's label to see the role update:

```shell
kubectl label node node-role-controller-worker nodeGroup=gpu --overwrite
```

Clean up:

```shell
make down
```

## Disclaimer

This is my personal project and it does not represent my employer. While I do my best to ensure that everything works, I take no responsibility for issues caused by this code.
