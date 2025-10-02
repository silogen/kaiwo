# Installation Guide

This guide provides clear, step‑by‑step instructions to install the Kaiwo operator and its dependencies on a Kubernetes cluster.

## Prerequisites

- A running Kubernetes cluster (v1.22+ recommended)
- `kubectl` configured with cluster-admin privileges
- `helm` (for Helm-based install)
- `git` (if using the helper scripts)

Optional (for GPU workloads): GPU-capable nodes and the appropriate GPU operator (AMD or NVIDIA).

## Dependency Overview

Kaiwo requires several core Kubernetes components to function correctly:

1.  **Cert-Manager**: Manages TLS certificates for webhooks.
2.  **GPU Operator**:
    *   **AMD**: [AMD GPU Operator](https://github.com/ROCm/amdgpu-operator). (Includes Node Labeler).
    *   **NVIDIA**: [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/overview.html) + [GPU Feature Discovery](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/gpu-feature-discovery.html).
    *   Ensures GPU drivers are installed and nodes are correctly labeled with GPU information.
3.  **Kueue**: Provides job queueing, fair sharing, and quota management. ([Docs](https://kueue.sigs.k8s.io/)).
4.  **KubeRay Operator**: Required only if users will run Ray-based workloads. Manages Ray clusters. ([Docs](https://docs.ray.io/en/latest/cluster/kubernetes/index.html)).
5.  **AppWrapper**: Used by Kueue to manage atomic scheduling of complex workloads, particularly Ray clusters/services. ([GitHub](https://github.com/project-codeflare/appwrapper)).
6.  **Prometheus (Recommended)**: For monitoring the Kaiwo operator and cluster metrics.

## Installation Methods

There are two main phases: install dependencies (Step 1) and install Kaiwo (Step 2). Choose the option(s) that fit your environment.

### Step 1: Install Dependencies

You can either install dependencies yourself or use the helper script (handy for dev/test).

!!! note
    The helper script requires [Helmfile](https://github.com/helmfile/helmfile) and [yq](https://github.com/mikefarah/yq).

Clone the repository and install dependencies using the script:

```bash
git clone https://github.com/silogen/kaiwo.git
cd kaiwo
dependencies/deploy.sh kind-test up  # Use appropriate environment
```

Available environments:
- `kind-test`: For Kind/testing clusters
- `tw-009-038`: GPU environment example
- `banff-sc-cx42-43`: GPU environment example

!!! info
    The GPU environments above are examples with hard-coded values for specific environments. To use the helper script with your own GPU cluster:

    1. Create a new environment file: `dependencies/environments/<my-env>.yaml`
    2. Create a new overlay: `dependencies/kustomization-server-side/overlays/environments/<my-env>/kustomization.yaml`

    Then install: `dependencies/deploy.sh <my-env> up`


### Step 2: Install the Kaiwo Operator

You can install Kaiwo via Helm (recommended) or by applying a prebuilt manifest with Kustomize.

#### Option A — Helm

Install from the OCI registry:

```bash
# Install latest version to kaiwo-system namespace
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator \
  --namespace kaiwo-system --create-namespace

# Install a specific version
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator \
  --version <version> \
  --namespace kaiwo-system --create-namespace
```

#### Option B — Kustomize Manifests

Install the latest version:

```bash
kubectl apply -f https://github.com/silogen/kaiwo/releases/latest/download/install.yaml --server-side
```

Or install a specific version:

```bash
export KAIWO_VERSION=vX.Y.Z
kubectl apply -f https://github.com/silogen/kaiwo/releases/download/${KAIWO_VERSION}/install.yaml --server-side
```

Install from a local build (useful for development):

```bash
make build-installer # produces dist/install.yaml
kubectl apply -f dist/install.yaml --server-side
```

This installs:

- Kaiwo CRDs (cluster-scoped)
  - `kaiwojobs.kaiwo.silogen.ai`
  - `kaiwoservices.kaiwo.silogen.ai`
  - `kaiwoqueueconfigs.kaiwo.silogen.ai`
  - `kaiwoconfigs.config.kaiwo.silogen.ai`
  - `resourceflavors.kaiwo.silogen.ai`
  - `topologies.kaiwo.silogen.ai`
- The Kaiwo controller `Deployment` in the `kaiwo-system` namespace
- RBAC rules (`ClusterRole`, `Role`, `ClusterRoleBinding`, `RoleBinding`)
- Webhook configurations and services

## Verification

After installation, verify that all components are running correctly:

### 1. Check Dependencies

Verify that all dependency components are running (only the ones you installed/apply):

```bash
# Check Cert-Manager
kubectl get pods -n cert-manager

# Check Kueue
kubectl get pods -n kueue-system

# Check KubeRay (if Ray workloads are used)
kubectl get pods -A | grep kuberay-operator || true

# Check AppWrapper
kubectl get pods -n appwrapper-system
```

### 2. Check Kaiwo Operator

Ensure the Kaiwo controller manager pod is running:

```bash
kubectl get pods -n kaiwo-system
# Expected output:
# NAME                                        READY   STATUS    RESTARTS   AGE
# kaiwo-controller-manager-xxxxxxxxxx-xxxxx   2/2     Running   0          2m
```

### 3. Verify CRDs

Check that the Kaiwo Custom Resource Definitions are installed:

```bash
kubectl get crds | grep -E 'kaiwo\.silogen\.ai|config\.kaiwo\.silogen\.ai'
# Expected output (at minimum):
# kaiwojobs.kaiwo.silogen.ai
# kaiwoservices.kaiwo.silogen.ai
# kaiwoqueueconfigs.kaiwo.silogen.ai
# kaiwoconfigs.config.kaiwo.silogen.ai
# resourceflavors.kaiwo.silogen.ai
# topologies.kaiwo.silogen.ai
```

### 4. Check Default Configuration

The operator should automatically create a default `KaiwoQueueConfig`:

```bash
kubectl get kaiwoqueueconfig kaiwo
# Expected output:
# NAME    AGE
# kaiwo   3m
```

If this is missing, check the operator logs:

```bash
kubectl logs -n kaiwo-system -l app.kubernetes.io/name=kaiwo
```

If pods are pending or webhooks fail, see [Troubleshooting](./troubleshooting.md).

## Uninstallation

### Remove Kaiwo Operator

For Helm installations:

```bash
helm uninstall kaiwo -n kaiwo-system
```

!!! danger "CRD Removal"
    Helm uninstall keeps CRDs by default. Deleting CRDs will remove all Kaiwo resources. Only delete CRDs if you intend to wipe all Kaiwo state.

For Kustomize installations:

```bash
kubectl delete -f https://github.com/silogen/kaiwo/releases/latest/download/install.yaml
```

!!! danger "CRD Removal"
    Kustomize **will** delete CRDs, which removes all Kaiwo resources. Only delete CRDs if you intend to wipe all Kaiwo state.

### Remove Dependencies

To remove dependencies:

```bash
cd kaiwo  # Your cloned repository
dependencies/deploy.sh kind-test down  # Use same environment as installation
```

## Provide CLI to Users

Instruct your users (AI Scientists/Engineers) on how to download and install the `kaiwo` CLI tool. Point them to the [User Quickstart guide](../scientist/quickstart.md) or the CLI Installation instructions.

## Next Steps

- **Configure Kaiwo**: Customize `KaiwoQueueConfig` and (optionally) `KaiwoConfig` to reflect your cluster’s hardware and policies. See the [Configuration Guide](./configuration.md).
- **Set up Monitoring**: Integrate Kaiwo operator metrics with your monitoring system (e.g., Prometheus). See the [Monitoring Guide](./monitoring.md).
- **Authentication**: Ensure users have the necessary `kubeconfig` files and any required authentication plugins installed. See [Authentication & Authorization](./auth.md).
- **Troubleshooting**: If something isn’t working, review common issues and fixes in [Troubleshooting](./troubleshooting.md).
