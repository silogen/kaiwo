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

## Contributing to Kaiwo

We welcome contributions to Kaiwo! Please refer to the [Contributing Guidelines](contributing-guidelines.md) for more information on how to contribute to the project.
