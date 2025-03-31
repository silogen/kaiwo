---
hide:
  - navigation
  - toc
---

# Welcome to Kaiwo

🚀️🚀️ Kaiwo supports ***AMD*** GPUs! 🚀️🚀️

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
