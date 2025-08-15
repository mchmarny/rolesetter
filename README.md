# Kubernetes Node Role Labeler

Kubernetes controller that automatically assign node role based on a value of specific node label (e.g., `nodeGroup=gpu-worker` == `node-role.kubernetes.io/gpu-worker`).

## Why 

By default, `kubeadm` enables the NodeRestriction admission controller that restricts what labels can be self-applied by `kubelet` on node registration. The `node-role.kubernetes.io/*` label is a restricted label and thus can't be set in the cloud init script or during other node inception process.

## Features

- Watches for node add/update events in the cluster.
- Checks if the node has the specified label value (configurable via launch parameter).
- If the node is missing the corresponding Kubernetes role label, it patches the node to add it.
- Uses exponential backoff for patch operations to handle transient API errors.
- Emits Prometheus metrics for successful and failed node patch operations.
- Gracefully handles shutdown signals (SIGINT/SIGTERM).

## Requirements

- Runs inside a Kubernetes cluster
- Requires RBAC permissions to patch node resources

## Usage

```sh
kubectl apply -k deployment/overlays/prod
```

This will ensure all nodes with `nodeGroup=customer-gpu` are labeled with `node-role.kubernetes.io/customer-gpu`.

## Metrics

The `node-role-controller` emits following metrics: 

- `node_patch_success_total`: Number of successful node patch operations.
- `node_patch_failure_total`: Number of failed node patch operations.

## Validation (optional)

The image produced by this repo comes with SLSA attestation which verifies that node role setter image was built in this repo. You can either verify that manually via [Sigstore](https://docs.sigstore.dev/about/overview/)  CLI or in the cluster suing [Sigstore](https://docs.sigstore.dev/about/overview/) policy controller.

### Manual 

> Update the image digest to the version you end up using.

```shell
export IMAGE=ghcr.io/mchmarny/node-role-controller@sha256:4f3cad359219be1e758cf7eafb90284f7bb280999127b1aa079618541e154766

cosign verify-attestation \
    --output json \
    --type slsaprovenance \
    --certificate-identity-regexp 'https://github.com/.*/.*/.github/workflows/.*' \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    $IMAGE 
```

### In Cluster

To to ensure the image used in the node role setter was built in this repo, you can enroll that one namespace (default: `node-labeler`):

```shell
kubectl label namespace node-labeler policy.sigstore.dev/include=true
```

And then add ClusterImagePolicy:

```shell
kubectl apply -f policy/clusterimagepolicy.yaml
```

And then test admission policy: 

```shell
 kubectl -n node-labeler run test --image=$IMAGE
```

If you don't already have [Sigstore](https://docs.sigstore.dev/about/overview/) policy controller, you add it into your cluster:

```shell
kubectl create namespace cosign-system
helm repo add sigstore https://sigstore.github.io/helm-charts
helm repo update
helm install policy-controller -n cosign-system sigstore/policy-controller
```

## Disclaimer

This is my personal project and it does not represent my employer. While I do my best to ensure that everything works, I take no responsibility for issues caused by this code.