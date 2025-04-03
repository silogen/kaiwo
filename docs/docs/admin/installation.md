# Installation Guide

This guide provides detailed steps for installing the Kaiwo operator and its dependencies on a Kubernetes cluster.

## Prerequisites

*   A running Kubernetes cluster (v1.22+ recommended).
*   `kubectl` installed and configured with cluster-admin privileges.
*   `git` (if cloning repositories).
*   Go (if using Cluster Forge).

## Dependency Overview

Kaiwo requires several core Kubernetes components to function correctly.

1.  **Cert-Manager**: Manages TLS certificates for webhooks.
2.  **GPU Operator**:
    *   **NVIDIA**: [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/overview.html) + [GPU Feature Discovery](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/gpu-feature-discovery.html).
    *   **AMD**: [AMD GPU Operator](https://github.com/ROCm/amdgpu-operator). (Includes Node Labeller).
    *   Ensures GPU drivers are installed and nodes are correctly labeled with GPU information.
3.  **Kueue**: Provides job queueing, fair sharing, and quota management. ([Docs](https://kueue.sigs.k8s.io/)).
4.  **KubeRay Operator**: Required *only* if users will run Ray-based workloads (`spec.ray: true`). Manages Ray clusters. ([Docs](https://docs.ray.io/en/latest/cluster/kubernetes/index.html)).
5.  **AppWrapper**: Used by Kueue to manage atomic scheduling of complex workloads, particularly Ray clusters/services. ([GitHub](https://github.com/project-codeflare/appwrapper)).
6.  **Prometheus (Recommended)**: For monitoring the Kaiwo operator and cluster metrics.

## Step 1: Install Kaiwo and its dependencies

There are several different ways that you can install the Kaiwo dependencies and operator. The following serve as references that you can adapt to your particular environment and workflow.

### Dependencies via convenience script

This script uses Kustomize to install Cert-Manager, Kueue, KubeRay, and AppWrapper.

```bash
# Clone the Kaiwo repository if you haven't already
# git clone https://github.com/silogen/kaiwo.git
# cd kaiwo

# Run the script
bash dependencies/install-dependencies.sh --local
```

!!!warning "GPU Operator Not Included"
    You must install the **AMD GPU Operator** separately according to its documentation *before* running the convenience script or installing Kaiwo. Ensure node labeling features are enabled.

### Kaiwo operator via install manifest

Once dependencies are ready, install the Kaiwo operator itself.

1.  **Choose Release**: Find the latest stable release tag on the [Kaiwo GitHub Releases page](https://github.com/silogen/kaiwo/releases).
2.  **Apply Manifest**: Use `kubectl apply` with the `--server-side` flag (recommended for managing large manifests and CRDs). Replace `vX.Y.Z` with your chosen release tag.

    ```bash
    export KAIWO_VERSION=vX.Y.Z
    kubectl apply -f https://github.com/silogen/kaiwo/releases/download/${KAIWO_VERSION}/install.yaml --server-side
    ```

    This installs:

    *   Kaiwo CRDs (`KaiwoJob`, `KaiwoService`, `KaiwoQueueConfig`)
    *   The Kaiwo Controller Manager `Deployment` in the `kaiwo-system` namespace.
    *   RBAC rules (`ClusterRole`, `Role`, `ClusterRoleBinding`, `RoleBinding`).
    *   Webhook configurations (if enabled in the release).
    *   Service for webhooks/metrics.

### Everything via Cluster Forge

[Cluster Forge](https://github.com/silogen/cluster-forge) is a tool for managing Kubernetes stacks. You can use it to install Kaiwo and its dependencies.

1.  Clone the Cluster Forge repository: `git clone https://github.com/silogen/cluster-forge.git`
2.  Navigate into the directory: `cd cluster-forge`
3.  Ensure Go is installed (`go version`).
4.  Run the forge command, selecting `kaiwo-all` and optionally the relevant GPU operator (`amd-gpu-operator`):
    ```bash
    go run . forge -s kaiwo
    # Follow prompts to select 'kaiwo-all' and your GPU operator stack.
    ```
5.  Deploy the selected stack:
    ```bash
    bash stacks/kaiwo/deploy.sh
    ```
6.  Verify pods in relevant namespaces (`kaiwo-system`, `cert-manager`, `kueue-system`, etc.).

## Step 2: Verify Installation

1.  **Check Operator Pod**: Ensure the Kaiwo controller manager pod is running.
    ```bash
    kubectl get pods -n kaiwo-system -l control-plane=kaiwo-controller-manager
    # Example Output:
    # NAME                                          READY   STATUS    RESTARTS   AGE
    # kaiwo-controller-manager-6c...-...            1/1     Running   0          2m
    ```

2.  **Check CRDs**: Verify that the Kaiwo Custom Resource Definitions are installed.
    ```bash
    kubectl get crds | grep kaiwo.silogen.ai
    # Example Output:
    # kaiwojoblists.kaiwo.silogen.ai          ...
    # kaiwojobs.kaiwo.silogen.ai              ...
    # kaiwoqueueconfigs.kaiwo.silogen.ai      ...
    # kaiwoservicelists.kaiwo.silogen.ai      ...
    # kaiwoservices.kaiwo.silogen.ai          ...
    ```

3.  **Check Default QueueConfig**: The operator should automatically create a default `KaiwoQueueConfig`.
    ```bash
    kubectl get kaiwoqueueconfig kaiwo
    # Example Output:
    # NAME    AGE
    # kaiwo   3m
    ```
    If this is missing, check the operator logs: `kubectl logs -n kaiwo-system -l control-plane=kaiwo-controller-manager`

## Step 3: Provide Kaiwo CLI to Users

Instruct your users (AI Scientists/Engineers) on how to download and install the `kaiwo` CLI tool. Point them to the [User Quickstart guide](../scientist/quickstart.md) or the [CLI Installation instructions](./../getting-started/installation.md#kaiwo-cli-tool).

## Next Steps

*   **Configure Kaiwo**: Customize the default `KaiwoQueueConfig` (`kubectl edit kaiwoqueueconfig kaiwo`) to define appropriate Kueue `ResourceFlavors` and `ClusterQueues` reflecting your cluster's hardware and policies. See the [Configuration Guide](./configuration.md).
*   **Set up Monitoring**: Integrate Kaiwo operator metrics with your monitoring system (e.g., Prometheus). See the [Monitoring Guide](./monitoring.md).
*   **Authentication**: Ensure users have the necessary `kubeconfig` files and any required authentication plugins installed. See [Authentication & Authorization](./auth.md).
