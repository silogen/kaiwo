# Quickstart for Administrators

This guide provides the essential steps to get the Kaiwo operator running on your Kubernetes cluster. For detailed instructions, refer to the [Installation](./installation.md) page.

## Prerequisites

*   A running Kubernetes cluster.
*   `kubectl` installed and configured to access your cluster.
*   Helm (optional, if using Helm charts for dependencies).
*   Appropriate cluster-admin privileges.

## 1. Install Dependencies

Kaiwo relies on several other Kubernetes components.

**Option A: Using Convenience Script (Recommended)**

This script installs Cert-Manager, Kueue, KubeRay, and AppWrapper using Kustomize.

```bash
# Clone the Kaiwo repository if you haven't already
# git clone https://github.com/silogen/kaiwo.git
# cd kaiwo

bash dependencies/setup_dependencies.sh
```

!!!warning
    This method does **not** install a GPU Operator (e.g., NVIDIA or AMD). You must install the appropriate GPU operator for your hardware separately *before* installing Kaiwo or its other dependencies, if you want to run GPU workloads.

**Option B: Manual Installation**

Install the following components manually, following their respective documentation:

1.  **Cert-Manager**: Required for webhook certificate management.
2.  **GPU Operator**: [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/overview.html) or [AMD GPU Operator](https://github.com/ROCm/gpu-operator). Ensure node labeling (e.g., via NVIDIA GPU Feature Discovery or AMD Node Labeller) is active.
3.  **Kueue**: For queueing and scheduling. ([Installation Guide](https://kueue.sigs.k8s.io/docs/installation/)).
4.  **KubeRay Operator**: Required if users will run Ray-based workloads (`spec.ray: true`). ([Installation Guide](https://docs.ray.io/en/latest/cluster/kubernetes/index.html)).
5.  **AppWrapper**: Used by Kueue for managing Ray workloads. ([Installation Guide](https://github.com/project-codeflare/appwrapper#installation)).

## 2. Install Kaiwo Operator

Find the latest release version on the [Kaiwo Releases Page](https://github.com/silogen/kaiwo/releases). Replace `vX.Y.Z` in the command below with the desired version tag.

```bash
# Replace vX.Y.Z with the actual release tag, e.g., v0.5.0
export KAIWO_VERSION=vX.Y.Z
kubectl apply -f https://github.com/silogen/kaiwo/releases/download/${KAIWO_VERSION}/install.yaml --server-side
```

This command installs the Kaiwo CRDs and the operator deployment in the `kaiwo-system` namespace.

## 3. Verify Installation

Check if the Kaiwo operator pod is running:

```bash
kubectl get pods -n kaiwo-system
```

You should see a pod named similar to `kaiwo-controller-manager-...` in the `Running` state.

Also, check that the Kaiwo CRDs are installed:

```bash
kubectl get crds | grep kaiwo.silogen.ai
```

You should see `kaiwojobs.kaiwo.silogen.ai`, `kaiwoservices.kaiwo.silogen.ai`, and `kaiwoqueueconfigs.kaiwo.silogen.ai`.

## 4. Initial Configuration (Default)

Upon startup, the Kaiwo operator automatically creates a default, cluster-scoped `KaiwoQueueConfig` resource named `kaiwo`. This configuration attempts to auto-discover node pools based on common GPU labels and creates corresponding Kueue `ResourceFlavors` and a default `ClusterQueue` named `kaiwo`.

Inspect the default configuration:

```bash
kubectl get kaiwoqueueconfig kaiwo -o yaml
```

You will likely need to customize this default configuration to match your specific cluster hardware, node labels, and desired queuing policies. See the [Configuration Guide](./configuration.md) for details.

## Next Steps

*   **Customize Configuration**: Modify the `kaiwo` `KaiwoQueueConfig` resource to define appropriate `ResourceFlavors` and `ClusterQueues` for your environment.
*   **Install CLI**: Instruct users on how to install and use the [Kaiwo CLI](../scientist/quickstart.md).
*   **Review Monitoring**: Set up monitoring for the Kaiwo operator and cluster resources. (See [Monitoring](./monitoring.md)).
