```plaintext
 _  __     _
| |/ /__ _(_)_      _____
| ' // _' | \ \ /\ / / _ \
| . \ (_| | |\ V  V / (_) |
|_|\_\__,_|_| \_/\_/ \___/
Kubernetes-native AI Workload Orchestrator
```

# Kaiwo - Kubernetes-native AI Workload Orchestrator to accelerate GPU workloads

🚀️🚀️ Kaiwo supports ***AMD*** GPUs! 🚀️🚀️

## Description

**Kaiwo** (pronunciation *"ky-voh"*) is a Kubernetes-native tool designed to optimize GPU resource utilization for AI workloads. Built on top of **Ray** and **Kueue** , Kaiwo minimizes GPU idleness and increases resource efficiency through intelligent job queueing, fair sharing of resources, guaranteed quotas and opportunistic gang scheduling.

Kaiwo supports a wide range of AI workloads, including distributed multi-node pretraining, fine-tuning, online inference, and batch inference, with seamless integration into Kubernetes environments.

**Full documentation can be found** [here](https://silogen.github.io/kaiwo/)

Kaiwo consists of two main components:

- **Kaiwo CLI**: A command-line interface for submitting and managing workloads to the Kaiwo Operator.
- **Kaiwo Operator**: A Kubernetes operator that manages the scheduling and execution of workloads on GPU nodes.
  The Kaiwo Operator is responsible for managing the lifecycle of workloads, including scheduling, resource allocation, and monitoring. It leverages the power of Ray and Kueue to provide efficient job queueing and scheduling.

## Main Features

* **GPU Utilization Optimization** :
  * Kaiwo Operator dynamically queues workloads to reduce GPU idle time and maximize resource utilization.
* **CLI Tool** :
  * Simplified workload submission using the kaiwo CLI tool
* **Distributed Workload Scheduling** :
  * Effortlessly schedule distributed workloads across multiple Kubernetes nodes with Kaiwo Operator.
* **Broad Workload Support** with pre-built templates:
  * Supports running **Kubernetes Jobs**, **RayJobs** and **RayServices**.
* **Integration with Ray and Kueue** :
  * Leverages the power of Ray for distributed computing and Kueue for efficient job queueing.

## Developer Quick Start

The easiest way to build and run the operator for development is via the `kaiwo.sh` helper script. You can also run the operator process locally once dependencies are installed.

- Prerequisites: Install Kaiwo dependencies (cert-manager, Kueue, GPU operator(s), etc.) per the Installation Guide step “Install Kaiwo and its Dependencies”. See docs: docs/docs/admin/installation.md#step-1-install-kaiwo-and-its-dependencies

### Fast Path: Use `kaiwo.sh`

`kaiwo.sh` wraps build, image publishing, and deployment via Helm or Kustomize.

- Kind + Helm (local dev loop):

  ```bash
  ./kaiwo.sh --install-crds --build --push=kind --deploy-via=helm up
  # Tear down
  ./kaiwo.sh --deploy-via=helm down
  ```

- ttl.sh + Helm (share image to a remote cluster):

  ```bash
  # 1h TTL by default; override with --ttl
  ./kaiwo.sh --install-crds --build --push=ttl.sh --deploy-via=helm up --ttl=2h
  ```

- Kind + Kustomize (no Helm):

  ```bash
  ./kaiwo.sh --install-crds --push=kind --deploy-via=kustomization up
  ```

Useful environment variables:

- `HELM_NAMESPACE` (default `kaiwo-system`), `HELM_RELEASE_NAME` (default `kaiwo`)
- `KIND_CLUSTER` (default `kaiwo-test`)
- `CONTAINER_TOOL` (`docker` or `podman`)
- `IMAGE_NAME` (override image name/reference), `IMAGE_REGISTRY`, `TAG`

For Helm configuration and values, see: docs/docs/admin/installation.md

### Run Operator Locally (no container)

Once dependencies and CRDs are installed in your cluster, you can run the controller directly from your machine using your kubeconfig context:

```bash
# Install/refresh CRDs into the target cluster
make install

# Run the controller against the cluster in your kubeconfig
make run
```

Tips:
- Ensure your kube context points to the cluster with Kaiwo dependencies installed. See docs: docs/docs/admin/installation.md#step-1-install-kaiwo-and-its-dependencies
- For E2E log collection while running locally, you can set: `export KAIWO_LOG_FILE=$PWD/kaiwo.logs`
- To deploy the containerized operator without the script, you can also use: `make deploy IMG=ghcr.io/silogen/kaiwo-operator:<tag>`

## Contributing to Kaiwo

We welcome contributions to Kaiwo! Please refer to the [Contributing Guidelines](contributing-guidelines.md) for more information on how to contribute to the project.
