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

- `node_patch_success_total`: Number of successful node patch operations.
- `node_patch_failure_total`: Number of failed node patch operations.

## Disclaimer

This is my personal project and it does not represent my employer. While I do my best to ensure that everything works, I take no responsibility for issues caused by this code.