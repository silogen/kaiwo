# Installation Guide

This guide provides detailed steps for installing the Kaiwo operator and its dependencies on a Kubernetes cluster.

## Prerequisites

*   A running Kubernetes cluster (v1.22+ recommended).
*   `kubectl` installed and configured with cluster-admin privileges.
*   `helm` (if using Helm installation method).
*   `git` (if installing dependencies via repository clone).

## Dependency Overview

Kaiwo requires several core Kubernetes components to function correctly:

1.  **Cert-Manager**: Manages TLS certificates for webhooks.
2.  **GPU Operator**:
    *   **NVIDIA**: [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/overview.html) + [GPU Feature Discovery](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/gpu-feature-discovery.html).
    *   **AMD**: [AMD GPU Operator](https://github.com/ROCm/amdgpu-operator). (Includes Node Labeller).
    *   Ensures GPU drivers are installed and nodes are correctly labeled with GPU information.
3.  **Kueue**: Provides job queueing, fair sharing, and quota management. ([Docs](https://kueue.sigs.k8s.io/)).
4.  **KubeRay Operator**: Required *only* if users will run Ray-based workloads (`spec.ray: true`). Manages Ray clusters. ([Docs](https://docs.ray.io/en/latest/cluster/kubernetes/index.html)).
5.  **AppWrapper**: Used by Kueue to manage atomic scheduling of complex workloads, particularly Ray clusters/services. ([GitHub](https://github.com/project-codeflare/appwrapper)).
6.  **Prometheus (Recommended)**: For monitoring the Kaiwo operator and cluster metrics.

## Installation Methods

There are two recommended approaches for installing Kaiwo. Choose the method that best fits your infrastructure and preferences.

### Method 1: Helm Chart (Recommended)

The **recommended** approach is to use the official Helm chart from GitHub releases.

#### Step 1: Install Dependencies

!!!warning "GPU Operator Required"
    You must install the **AMD GPU Operator** or **NVIDIA GPU Operator** separately according to their documentation *before* proceeding. Ensure node labeling features are enabled.

Clone the repository and install dependencies using the deployment script:

```bash
git clone https://github.com/silogen/kaiwo.git
cd kaiwo
dependencies/deploy.sh kind-test up  # Use appropriate environment
```

Available environments:
- `kind-test`: For Kind/testing clusters
- `tw-009-038`: GPU environment example  
- `banff-sc-cx42-43`: GPU environment example

#### Step 2: Install Kaiwo Operator via Helm

```bash
# Add the Kaiwo Helm repository (if published) or use local chart
helm repo add kaiwo https://silogen.github.io/kaiwo/
helm repo update

# Install with default values
helm install kaiwo-operator kaiwo/kaiwo-operator \
  --namespace kaiwo-system \
  --create-namespace

# Or install a specific version
export KAIWO_VERSION=vX.Y.Z
helm install kaiwo-operator kaiwo/kaiwo-operator \
  --version ${KAIWO_VERSION} \
  --namespace kaiwo-system \
  --create-namespace
```

### Method 2: Kustomize Install Manifest

For environments where Helm is not available, you can use the install manifest.

#### Step 1: Install Dependencies

Same as Method 1 - clone the repository and run the dependency script:

```bash
git clone https://github.com/silogen/kaiwo.git
cd kaiwo
dependencies/deploy.sh kind-test up  # Use appropriate environment
```

#### Step 2: Install Kaiwo Operator via Manifest

Install the latest version:

```bash
kubectl apply -f https://github.com/silogen/kaiwo/releases/latest/download/install.yaml --server-side
```

Or install a specific version:

```bash
export KAIWO_VERSION=vX.Y.Z
kubectl apply -f https://github.com/silogen/kaiwo/releases/download/${KAIWO_VERSION}/install.yaml --server-side
```

This installs:

*   Kaiwo CRDs (`KaiwoJob`, `KaiwoService`, `KaiwoQueueConfig`)
*   The Kaiwo Controller Manager `Deployment` in the `kaiwo-system` namespace
*   RBAC rules (`ClusterRole`, `Role`, `ClusterRoleBinding`, `RoleBinding`)
*   Webhook configurations and services

## Verification

After installation, verify that all components are running correctly:

### 1. Check Dependencies

Verify that all dependency components are running:

```bash
# Check Cert-Manager
kubectl get pods -n cert-manager

# Check Kueue
kubectl get pods -n kueue-system

# Check KubeRay (if Ray workloads are used)
kubectl get pods -n default -l app.kubernetes.io/name=kuberay

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
kubectl get crds | grep kaiwo.silogen.ai
# Expected output:
# kaiwojobs.kaiwo.silogen.ai
# kaiwoqueueconfigs.kaiwo.silogen.ai  
# kaiwoservices.kaiwo.silogen.ai
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

## Uninstallation

### Remove Kaiwo Operator

For Helm installations:

```bash
helm uninstall kaiwo-operator -n kaiwo-system
```

For Kustomize installations:

```bash
kubectl delete -f https://github.com/silogen/kaiwo/releases/latest/download/install.yaml
```

### Remove Dependencies

To remove dependencies:

```bash
cd kaiwo  # Your cloned repository
dependencies/deploy.sh kind-test down  # Use same environment as installation
```

## Provide CLI to Users

Instruct your users (AI Scientists/Engineers) on how to download and install the `kaiwo` CLI tool. Point them to the [User Quickstart guide](../scientist/quickstart.md) or the CLI Installation instructions.

## Next Steps

*   **Configure Kaiwo**: Customize the default `KaiwoQueueConfig` to define appropriate Kueue `ResourceFlavors` and `ClusterQueues` reflecting your cluster's hardware and policies. See the [Configuration Guide](./configuration.md).
*   **Set up Monitoring**: Integrate Kaiwo operator metrics with your monitoring system (e.g., Prometheus). See the [Monitoring Guide](./monitoring.md).
*   **Authentication**: Ensure users have the necessary `kubeconfig` files and any required authentication plugins installed. See [Authentication & Authorization](./auth.md).
