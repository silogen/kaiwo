---
hide:
  - navigation
  - toc
---

# Welcome to Kaiwo

🚀️🚀️ Kaiwo supports ***AMD*** GPUs! 🚀️🚀️

This repository is under active development, and the codebase is subject to frequent changes, including potentially some breaking updates. While we strive to maintain stability, please be aware that the **`main`** branch may not always be in a functional or stable state.

To ensure a smooth experience, we strongly recommend that users:

**1.** **Stick to Stable Releases**

- Use the tagged [releases on GitHub](https://github.com/silogen/kaiwo/releases) for the most stable and tested versions.
- Avoid building directly from the **`main`** branch unless you are comfortable with potential instability or are contributing to the project.

**2.** **Provide Feedback**

- If you encounter any issues or have suggestions, feel free to open an issue in the [Issues section](https://github.com/silogen/kaiwo/issues).

## Description

**Kaiwo** (pronunciation *"ky-voh"*) is a Kubernetes-native tool designed to optimize GPU resource utilization for AI workloads. The project is built primarily for AMD GPUs. Built on top of **Ray** and **Kueue** , Kaiwo minimizes GPU idleness and increases resource efficiency through intelligent job queueing, fair sharing of resources, guaranteed quotas and opportunistic gang scheduling.

Kaiwo supports a wide range of AI workloads, including distributed multi-node pretraining, fine-tuning, online inference, and batch inference, with seamless integration into Kubernetes environments.

## Main Features

**GPU Utilization Optimization**
:   Dynamically queues workloads to reduce GPU idle time and maximize resource utilization.

**CLI Tool**
:   Simplified workload submission using the kaiwo CLI tool

**Distributed Workload Scheduling**
:   Effortlessly schedule distributed workloads across multiple Kubernetes nodes.

**Broad Workload Support** with pre-built templates
  :   Supports running **Kubernetes Jobs**, **RayJobs** and **RayServices**.

**Integration with Ray and Kueue**
  :   Leverages the power of Ray for distributed computing and Kueue for efficient job queueing.
