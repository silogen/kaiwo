# Overview for Administrators

Kaiwo provides a layer on top of Kubernetes, Kueue, and Ray to streamline the management and execution of AI workloads, particularly focusing on efficient GPU utilization. As an administrator, your role involves deploying, configuring, and maintaining the Kaiwo system.

## Key Components and Concepts

1.  **Kaiwo Operator**:
    *   Runs as a deployment within the Kubernetes cluster.
    *   Manages the lifecycle of Kaiwo Custom Resources (`KaiwoJob`, `KaiwoService`, `KaiwoQueueConfig`).
    *   **Controllers**: Includes specific controllers for each CRD:
        *   `KaiwoJobController`: Translates `KaiwoJob` into `batchv1.Job` or `rayv1.RayJob`, manages dependencies (like download jobs, PVCs), and updates status.
        *   `KaiwoServiceController`: Translates `KaiwoService` into `appsv1.Deployment` or `rayv1.RayService` (wrapped in an `AppWrapper`), manages dependencies, and updates status.
        *   `KaiwoQueueConfigController`: Manages Kueue resources (`ClusterQueue`, `ResourceFlavor`, `WorkloadPriorityClass`) based on the cluster-scoped `KaiwoQueueConfig` CRD. Ensures a default configuration exists.
    *   **Integration**: Interacts with the Kubernetes API, Kueue, and Ray operators.

2.  **Kaiwo CRDs**:
    *   **`KaiwoJob` / `KaiwoService`**: User-facing resources defined by AI Scientists to describe their workloads. They abstract away much of the underlying Kubernetes/Ray/Kueue complexity.
    *   **`KaiwoQueueConfig`**: A cluster-scoped resource (typically one named `kaiwo`) used by administrators to define and manage Kueue configurations centrally. This includes defining queues, resource types (flavors), and priorities.

3.  **Kueue Integration**:
    *   Kaiwo relies on Kueue for job queueing, scheduling, and resource quota management.
    *   The Kaiwo Operator, specifically the `KaiwoQueueConfigController`, manages the creation and synchronization of Kueue `ClusterQueue`, `ResourceFlavor`, and `WorkloadPriorityClass` resources based on the `KaiwoQueueConfig` CRD.
    *   Workloads (`KaiwoJob`/`KaiwoService`) are submitted to a specific `ClusterQueue` (via the `kueue.x-k8s.io/queue-name` label, derived from `spec.clusterQueue`).

4.  **Ray Integration**:
    *   If `spec.ray: true` is set in a `KaiwoJob`, the operator creates a `RayJob` instead of a standard Kubernetes Job. If `spec.type: ray` is set in a `KaiwoService` (or `spec.ray` block is provided without `type`), the operator creates a `RayService` instead of a Deployment.
    *   This leverages Ray for distributed execution capabilities. Requires the KubeRay operator to be installed.

5.  **Kaiwo CLI**:
    *   The primary user interface for AI Scientists.
    *   Communicates with the Kubernetes API to create and manage Kaiwo CRDs.
    *   Requires `kubeconfig` access similar to `kubectl`.

## Administrator Responsibilities

*   **Installation**: Deploying the Kaiwo operator and its dependencies (Kueue, Ray Operator, Cert-Manager, GPU Operator, etc.).
*   **Configuration**: Defining cluster-wide queuing policies, resource flavors (mapping to node types/pools), and priorities using the `KaiwoQueueConfig` CRD. Managing storage classes referenced by users.
*   **Maintenance**: Upgrading Kaiwo components, monitoring operator health, managing certificates.
*   **Monitoring**: Observing cluster resource utilization, queue lengths, and workload statuses. Integrating with monitoring tools like Prometheus.
*   **User Management**: Potentially managing namespaces and ensuring users target appropriate Kueue queues.
*   **Troubleshooting**: Diagnosing issues related to scheduling, resource allocation, operator errors, or workload failures.
