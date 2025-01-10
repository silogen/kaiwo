# Aiwo - AI Workload Orchestrator

Kubernetes-native AI Workload Orchestrator to accelerate GPU workloads

🚀️🚀️ Aiwo supports ***AMD*** GPUs! 🚀️🚀️

⚠️ **Caveat: Heavy Development in Progress** ⚠️

This repository is under active and ****heavy development****, and the codebase is subject to frequent changes, including potentially some breaking updates. While we strive to maintain stability, please be aware that the **`main`** branch may not always be in a functional or stable state.

To ensure a smooth experience, we strongly recommend that users:

**1.** ****Stick to Stable Releases****

- Use the tagged [releases on GitHub](https://github.com/silogen/ai-workload-orchestrator/releases) for the most stable and tested versions.
- Avoid building directly from the **`main`** branch unless you are comfortable with potential instability or are contributing to the project.

**2.** ****Monitor Changes****

- Keep an eye on the [Changelog](https://github.com/silogen/ai-workload-orchestrator/CHANGELOG.md) for updates and breaking changes.

**3.** ****Provide Feedback****

- If you encounter any issues or have suggestions, feel free to open an issue in the [Issues section](https://github.com/silogen/ai-workload-orchestrator/issues).

## Description

**AI Workload Orchestrator (Aiwo)** is a Kubernetes-native tool designed to optimize GPU resource utilization for AI workloads. The project is built primarily for AMD GPUs. Built on top of **Ray** and **Kueue** , Aiwo minimizes GPU idleness and increases resource efficiency through intelligent job queueing, fair sharing of resources, guaranteed quotas and opportunistic batch job scheduling.

Aiwo supports a wide range of AI workloads, including distributed multi-node pretraining, fine-tuning, online inference, and batch inference, with seamless integration into Kubernetes environments.

## Main Features

* **GPU Utilization Optimization** :
  * Dynamically queues workloads to reduce GPU idle time and maximize resource utilization.
* **CLI Tool** :
  * Simplified workload submission using the aiwo CLI tool
* **Distributed Workload Scheduling** :
  * Effortlessly schedule distributed workloads across multiple Kubernetes nodes.
* **Broad Workload Support** with pre-built templates:
  * Supports running **Kubernetes Jobs**, **RayJobs** and **RayServices**.
* **Integration with Ray and Kueue** :
  * Leverages the power of Ray for distributed computing and Kueue for efficient job queueing.

## Installation

### Installation of Ray and Kueue Operators on Kubernetes

Aiwo requires 4-5 components installed on Kubernetes:

1. Cert-Manager
2. AMD Operator (with AMD-Device-Config) or Nvidia Operator
3. Kueue Operator
4. KubeRay Operator
5. Prometheus (not strictly necessary but recommended)

We recommend using [Cluster-Forge](https://github.com/silogen/cluster-forge) to install all necessary components for Kubernetes. There is a README in the Cluster-Forge repo, but the steps are simple:

1. Clone the repository: `git clone https://github.com/silogen/cluster-forge.git`
2. Make sure you have Go installed
3. Run `./scripts/clean.sh` to make sure you're starting off clean slate
4. Run `go run . smelt` and select your components (above)
5. Make sure docker is using multiarch-builder `docker buildx create --name multiarch-builder --use`
6. Run `go run . cast`
7. Run `go run . forge`

### Installation of Aiwo CLI tool

The installation of Aiwo CLI tool is easy as it's a single binary. The only requirement is a kubeconfig file to access a Kubernetes cluster. If you are unsure where to get a kubeconfig, speak to the engineers who set up your Kubernetes cluster. Just like kubectl, Aiwo will first look for a `KUBECONFIG=path` environment variable. If `KUBECONFIG` is not set, Aiwo will then look for kubeconfig file in the default location `~/.kube/config`.

1. Download the AIWO CLI binary from the [Releases Page](https://github.com/silogen/ai-workload-orchestrator/releases).
2. Make the binary executable and add it to your PATH:

```bash
chmod +x aiwo
mv aiwo /usr/local/bin/
```

## Usage

At the moment, Aiwo can submit three types of workloads to Kubernetes

- Standard Jobs
- RayJobs
- RayServices

`workloads` directory includes examples with code for different types workloads

Before submitting workloads

TODO, describe init

Kueue resource flavour(s)
Kueue Cluster Queue
Kueue Local Queue

- code must be in /workload dir if already mounted in the image
- Describe commands and flags (Number of GPUs and path are required. Path must include entrypoint file at minimum)
- Multi-node workloads become single-node by adjustting GPU requests (notice also changes to VLLM pipeline parallel)
- recommendation to use separate cluster for online inference workloads (due to resource contention)
- Note about typical secrets and environment variables (s3 keys, HF TOKEN, etc)
- Note about how secrets are managed (ExternalSecrets, etc)

## Contributing to Aiwo

TODO

We welcome contributions to Aiwo! Please refer to the [Contributing Guidelines]() for more information on how to contribute to the project.
