Kaiwo consists of two main components:

- **Kaiwo CLI**: A command-line interface for submitting and managing workloads to the Kaiwo Operator.
- **Kaiwo Operator**: A Kubernetes operator that manages the scheduling and execution of workloads on GPU nodes.
  The Kaiwo Operator is responsible for managing the lifecycle of workloads, including scheduling, resource allocation, and monitoring. It leverages the power of Ray and Kueue to provide efficient job queueing and scheduling.