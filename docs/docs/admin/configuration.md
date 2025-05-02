# Configuration Guide

Administrators configure Kaiwo primarily through

* The cluster-scoped `KaiwoQueueConfig` Custom Resource Definition (CRD)
* The cluster-scoped `KaiwoConfig` CRD
* Environment variables or flags passed to the Kaiwo operator

Users configure the CLI tool separately.

## KaiwoQueueConfig CRD

This is the central point for managing how Kaiwo interacts with Kueue. There can be only **one** `KaiwoQueueConfig` resource in the cluster, and its `metadata.name` **must be `kaiwo`** (or the value specified in the KaiwoConfig Custom Resource field `spec.defaultKaiwoQueueConfigName`, which defaults to `kaiwo`, see more below).

**Default Configuration on Startup:**

The Kaiwo operator includes a startup routine that checks if a `KaiwoQueueConfig` named `kaiwo` exists. If it does not, the operator automatically creates a default one. This default configuration aims to provide a functional baseline:

*   It attempts to **auto-discover** node pools based on common GPU labels (e.g., `amd.com/gpu.device-id`, `nvidia.com/gpu.product`, `nvidia.com/gpu.count`) and CPU/Memory capacity.
*   It creates corresponding Kueue `ResourceFlavor` resources based on this discovery, labeling the nodes with `kaiwo/nodepool=<generated-flavor-name>`.
*   It defines a single Kueue `ClusterQueue` named `kaiwo` (or the value of `DEFAULT_CLUSTER_QUEUE_NAME`), configured to use all discovered `ResourceFlavors` and their estimated capacities as `nominalQuota`.
*   It specifies that this default `ClusterQueue` should have a corresponding `LocalQueue` automatically created in the `kaiwo` namespace.
*   It does not define any `WorkloadPriorityClass` resources by default.

You can modify this automatically created configuration or create your own `kaiwo` resource manually using: `kubectl edit kaiwoqueueconfig kaiwo` or by applying a YAML manifest.

**Key Fields (`spec`):**

*   **`resourceFlavors`**: Defines the types of hardware resources available in the cluster, corresponding to Kueue `ResourceFlavor` resources.
    *   `name`: A unique name for the flavor (e.g., `amd-mi300-8gpu`, `nvidia-a100-40gb`, `cpu-standard`).
    *   `nodeLabels`: A map of labels that nodes must possess to be considered part of this flavor. This is crucial for scheduling pods onto the correct hardware. Example: `{"kaiwo/nodepool": "amd-mi300-nodes"}`.
    *   `taints`: (Optional) A list of Kubernetes taints associated with this flavor. Pods scheduled to this flavor will need corresponding tolerations. Kaiwo automatically adds tolerations for GPU taints if `ADD_TAINTS_TO_GPU_NODES` is enabled.

    !!! info "Auto-Discovery vs. Explicit Definition"
        If `spec.resourceFlavors` is empty or omitted in the `kaiwo` `KaiwoQueueConfig`, the operator's startup logic attempts to **auto-discover** node pools and create corresponding flavors as described above. While convenient for initial setup, explicitly defining `resourceFlavors` in the `KaiwoQueueConfig` provides more precise control and is generally recommended for production environments. Explicitly defined flavors will override any auto-discovered ones during reconciliation.

*   **`clusterQueues`**: Defines the Kueue `ClusterQueue` resources managed by Kaiwo.
    *   `name`: The name of the `ClusterQueue` (e.g., `team-a-queue`, `default-gpu-queue`).
    *   `spec`: The full Kueue `ClusterQueueSpec`. This is where you define resource quotas, cohorts, preemption policies, etc. See [Kueue ClusterQueue Documentation](https://kueue.sigs.k8s.io/docs/concepts/cluster_queue/).
        *   `resourceGroups`: Define sets of flavors and their associated quotas (`nominalQuota`). This links the queue to the available hardware defined in `resourceFlavors`.
        *   `namespaceSelector`: Controls which namespaces can use this queue via `LocalQueue` resources *if* those LocalQueues exist. Note that Kaiwo's automatic `LocalQueue` creation relies on the `namespaces` field below, not this selector.
    *   `namespaces`: A list of namespace names where Kaiwo should automatically create and manage a Kueue `LocalQueue` pointing to this `ClusterQueue`. The `LocalQueue` created will have the same name as the `ClusterQueue`.

*   **`workloadPriorityClasses`**: Defines Kueue `WorkloadPriorityClass` resources.
    *   Follows the standard Kueue `WorkloadPriorityClass` structure (`name`, `value`, `description`). Kaiwo ensures these exist as defined. See [Kueue Priority Documentation](https://kueue.sigs.k8s.io/docs/concepts/workload_priority_class/).

**Example `KaiwoQueueConfig`:**

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoQueueConfig
metadata:
  name: kaiwo # Must be named 'kaiwo' (or DEFAULT_KAIWO_QUEUE_CONFIG_NAME)
spec:
  resourceFlavors:
    - name: amd-mi300-8gpu
      nodeLabels:
        kaiwo/nodepool: amd-mi300-nodes # Nodes with this label belong to this flavor
        # Add other identifying labels if needed, e.g., topology.amd.com/gpu-count: '8'
      # taints: # Optional, define if specific taints apply ONLY to these nodes
      # - key: "amd.com/gpu"
      #   operator: "Exists"
      #   effect: "NoSchedule"
    - name: cpu-high-mem
      nodeLabels:
        kaiwo/nodepool: cpu-high-mem-nodes

  clusterQueues:
    - name: ai-research-queue # Name of the ClusterQueue
      namespaces: # Auto-create/manage LocalQueues in these namespaces
        - ai-research-ns-1
        - ai-research-ns-2
      spec: # Standard Kueue ClusterQueueSpec
        queueingStrategy: BestEffortFIFO
        resourceGroups:
          - coveredResources: ["cpu", "memory", "amd.com/gpu"] # Resources managed by this group
            flavors:
              - name: amd-mi300-8gpu # Reference to a defined resourceFlavor
                resources:
                  - name: "cpu"
                    nominalQuota: "192" # Total CPU quota for this flavor in this queue
                  - name: "memory"
                    nominalQuota: "1024Gi" # Total Memory quota
                  - name: "amd.com/gpu"
                    nominalQuota: "8" # Total GPU quota
          - coveredResources: ["cpu", "memory"]
            flavors:
              - name: cpu-high-mem
                resources:
                  - name: "cpu"
                    nominalQuota: "256"
                  - name: "memory"
                    nominalQuota: "2048Gi"
        # cohort: "gpu-cohort" # Optional: Group queues for borrowing/preemption
        # preemption: ...

  workloadPriorityClasses:
    - name: high-priority
      value: 1000
    - name: low-priority
      value: 100
```

### Controller Operation and Kueue Resource Synchronization

The `KaiwoQueueConfigController` acts as a translator, continuously ensuring that the Kueue resources in your cluster accurately reflect the configuration defined in the single `kaiwo` `KaiwoQueueConfig` resource. It monitors this resource and automatically manages the lifecycle of the associated Kueue objects:

*   **`spec.resourceFlavors` -> Kueue `ResourceFlavor`:**
    *   Each entry in this list directly defines a Kueue `ResourceFlavor`.
    *   The controller ensures a corresponding `ResourceFlavor` exists for each entry, creating or updating it as necessary based on the specified `name`, `nodeLabels`, and `taints`.
    *   If an entry is removed from this list, the controller deletes the corresponding `ResourceFlavor`.

*   **`spec.clusterQueues` -> Kueue `ClusterQueue` and `LocalQueue`:**
    *   Each entry in this list defines a Kueue `ClusterQueue`. The controller translates the structure into a standard `ClusterQueueSpec` and ensures the resource exists and matches the definition. Removing an entry deletes the corresponding `ClusterQueue`.
    *   The `namespaces` field within each `clusterQueues` entry dictates where Kueue `LocalQueue`s should exist. The controller automatically creates a `LocalQueue` (named after the `ClusterQueue`) in each listed namespace, pointing to the corresponding `ClusterQueue`. If a namespace is removed from the list, or the parent `ClusterQueue` entry is removed, the controller deletes the associated `LocalQueue` in that namespace.

*   **`spec.workloadPriorityClasses` -> Kueue `WorkloadPriorityClass`:**
    *   Each entry defines a Kueue `WorkloadPriorityClass`.
    *   The controller manages these resources, ensuring they exist with the specified `name`, `value`, and `description`.
    *   Removing an entry results in the deletion of the corresponding `WorkloadPriorityClass`.

!!! info "Owner References"
    The controller establishes the `kaiwo` `KaiwoQueueConfig` as the owner of all the Kueue resources it creates. This linkage ensures that if the `KaiwoQueueConfig` is deleted, Kubernetes automatically cleans up all the managed Kueue resources (`ResourceFlavor`, `ClusterQueue`, `LocalQueue`, `WorkloadPriorityClass`).

!!! warning "Kueue resource management"
    Kaiwo takes ownership of Kueue `ResourceFlavor`, `ClusterQueue`, `LocalQueue` and `WorkloadPriorityClass` resources. This means that resources of these types that are created manually, i.e. not via the `KaiwoQueueConfig`, may be deleted by the Kaiwo Controller

The controller updates the `status.status` field of the `KaiwoQueueConfig` resource (`Pending`, `Ready`, or `Failed`) to indicate the current state of synchronization between the desired configuration and the actual Kueue resources in the cluster. This continuous reconciliation keeps the Kueue setup aligned with the central `KaiwoQueueConfig`.

## KaiwoConfig CRD

The Kaiwo Operator's runtime configuration is managed through the `KaiwoConfig` Custom Resource Definition (CRD). This approach allows Kubernetes administrators to dynamically adjust operator behavior without requiring a restart. The operator always retrieves the most recent configuration values during each reconcile loop.

### Configuration Structure

The primary configuration resource is the `KaiwoConfig` CRD, typically maintained as a singleton within the Kubernetes cluster. Its key components are encapsulated in the `KaiwoConfigSpec`, which briefly includes:

- **`kueue`**: Configures default integration settings with Kueue, including the default cluster queue name.
- **`ray`**: Specifies Ray-specific parameters, including default container images and memory allocations.
- **`data`**: Manages default filesystem paths for mounting data storage and HuggingFace caches.
- **`nodes`**: Defines node-specific settings such as GPU resource keys, GPU node taints, and node pool exclusions.
- **`scheduling`**: Sets scheduling-related configurations, like the Kubernetes scheduler name.
- **`resourceMonitoring`**: Configures resource monitoring, including averaging intervals, utilization thresholds, and targeted namespaces.
- **`defaultKaiwoQueueConfigName`**: Specifies the default name for the Kaiwo queue configuration object.

### Specifying the Configuration CR

The Kaiwo Operator identifies its configuration resource via the environment variable `CONFIG_NAME`. By default, this is set to `kaiwo`. Ensure that a `KaiwoConfig` resource with this exact name exists in your cluster. Alternatively, setting the environment variable `CONFIG_GENERATE_DEFAULT=true` instructs the operator to automatically create a default configuration at startup if none exists.

!!!note 
    The operator waits up to **30 seconds** for the specified configuration resource to be found. If no resource is detected within this period, the operator pod will fail with an error.

### Example KaiwoConfig CR

Here's a minimal example of a valid `KaiwoConfig` definition:

```yaml
apiVersion: config.kaiwo.silogen.ai/v1alpha1
kind: KaiwoConfig
metadata:
  name: kaiwo
spec:
  defaultKaiwoQueueConfigName: "kaiwo"
  scheduling:
    kubeSchedulerName: "default-scheduler"
  resourceMonitoring:
    averagingTime: "20m"
    lowUtilizationThreshold: 20
    profile: "gpu"
```

!!!note
    You can safely use the default values or create a default config by setting the environment variable `CONFIG_GENERATE_DEFAULT` to true.

### Environment Variables for Resource Monitoring

To enable and configure resource monitoring within the Kaiwo Operator, the following environment variables **must** be set on the operator deployment:

- `RESOURCE_MONITORING_ENABLED=true` – Enables the resource monitoring component.
- `RESOURCE_MONITORING_PROMETHEUS_ENDPOINT=<prometheus-endpoint>` – Specifies the Prometheus endpoint to query metrics from.
- `RESOURCE_MONITORING_POLLING_INTERVAL=10m` – Sets the interval between metric polling queries.

These variables cannot be updated dynamically and require a restart of the operator pod to take effect.

---

For detailed descriptions of individual configuration fields, please see the [full API reference](../reference/crds/config.kaiwo.silogen.ai.md).

## Other configuration options

*   `WEBHOOK_CERT_DIRECTORY`: Path to manually provided webhook certificates (overrides automatic management if set). See [Installation](./installation.md).

!!! info "Forthcoming feature"
    `ENFORCE_KAIWO_ON_GPU_WORKLOADS` (Default: `false`): If `true`, the mutating admission webhook for `batchv1.Job` will automatically add the `kaiwo.silogen.ai/managed: "true"` label to any job requesting GPU resources, forcing it to be managed by Kaiwo/Kueue.

All environmental variables are typically set in the operator's `Deployment` manifest.

**Command-Line Flags:**

Refer to the output of `kaiwo-operator --help` (or check `cmd/operator/main.go`) for flags controlling metrics, health probes, leader election, and certificate paths.
