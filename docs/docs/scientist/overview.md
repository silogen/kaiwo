# Overview 

This section is targeted at AI Scientists, engineers, and researchers who are interested in using Kaiwo to deploy AI workloads. It provides an overview of Kaiwo's features and benefits, as well as guidance on how to get started with deploying AI workloads with Kaiwo.

The two main components of Kaiwo are the Kaiwo CLI and the Kaiwo Operator. The components are described [here](../general/main-components.md).

Features and Benefits to AI Scientists

- **Easy Deployment**: Kaiwo simplifies the process of deploying AI workloads on Kubernetes, allowing scientists to focus on their research rather than infrastructure management.
- **Broad Workload Support**: Kaiwo supports a wide range of AI workloads, including distributed multi-node pretraining, fine-tuning, online inference, and batch inference. This flexibility allows scientists to run various types of workloads without needing to switch tools. At the moment, we support Batch Jobs, Deployments, RayJobs, and RayServices. We will continue to add support for other workload types in the future.
- **Scalability**: Kaiwo leverages the power of Kubernetes to scale workloads up or down based on demand, ensuring efficient resource utilization.
- **Resource Management**: Kaiwo provides advanced resource management capabilities, allowing scientists to allocate resources based on workload requirements. By default, Kaiwo monitors workloads' GPU usage and terminates workloads that underutilize GPUs for too long, ensuring capacity is released back to those that need it.
- **Monitoring and Logging**: Kaiwo offers built-in monitoring and logging features, enabling scientists to track the performance of their workloads and troubleshoot issues easily.
- **Integration with Ray**: Kaiwo integrates seamlessly with Ray, a powerful distributed computing framework, enabling scientists to run large-scale AI workloads efficiently.
- **Integration with Kueue**: Kaiwo uses Kueue for queueing and scheduling of all supported workload types, ensuring efficient management of workloads in a Kubernetes environment.

To get started with Kaiwo, scientists can follow the [quickstart guides](quickstart.md). These guides cover various aspects of deploying AI workloads, including training, distributed training, inference, and distributed inference. The quickstart guides are designed to help scientists quickly understand how to use Kaiwo and get their workloads up and running on Kubernetes.