# Configuration Guide

Administrators configure Kaiwo primarily through the cluster-scoped `KaiwoQueueConfig` Custom Resource Definition (CRD) and environment variables or flags passed to the Kaiwo operator. Users configure the CLI tool separately.

## KaiwoQueueConfig CRD

This is the central point for managing how Kaiwo interacts with Kueue. There can be only **one** `KaiwoQueueConfig` resource in the cluster, and its `metadata.name` **must be `kaiwo`** (or the value specified by the `DEFAULT_KAIWO_QUEUE_CONFIG_NAME` environment variable, which defaults to `kaiwo`).

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

## Kaiwo Operator Configuration

The Kaiwo operator deployment itself can be configured using environment variables or command-line flags.

**Key Environment Variables:**

*   `DEFAULT_KAIWO_QUEUE_CONFIG_NAME` (Default: `kaiwo`): The expected name of the single `KaiwoQueueConfig` resource the controller manages.
*   `DEFAULT_CLUSTER_QUEUE_NAME` (Default: `kaiwo`): The default Kueue `ClusterQueue` name used by the startup logic when creating the default `KaiwoQueueConfig` and used by workloads that do not include the `clusterQueue` field.
*   `DEFAULT_GPU_TAINT_KEY` (Default: `kaiwo.silogen.ai/gpu`): The taint key applied to GPU nodes if automatic tainting is enabled.
*   `ADD_TAINTS_TO_GPU_NODES` (Default: `false`): If `true`, the `KaiwoQueueConfigController` will attempt to taint nodes identified as having GPUs (based on flavor discovery/definition) with the `DEFAULT_GPU_TAINT_KEY`.
*   `EXCLUDE_MASTER_NODES_FROM_NODE_POOLS` (Default: `false`): If `true`, control-plane/master nodes are excluded during automatic `ResourceFlavor` discovery in the startup logic.
*   `RAY_HEAD_POD_MEMORY`: (Optional) Override the default memory request/limit for Ray head pods created by Kaiwo (which is `16Gi`).
*   `WEBHOOK_CERT_DIRECTORY`: Path to manually provided webhook certificates (overrides automatic management if set). See [Installation](./installation.md).

!!! info "Forthcoming feature"
    `ENFORCE_KAIWO_ON_GPU_WORKLOADS` (Default: `false`): If `true`, the mutating admission webhook for `batchv1.Job` will automatically add the `kaiwo.silogen.ai/managed: "true"` label to any job requesting GPU resources, forcing it to be managed by Kaiwo/Kueue.

These are typically set in the operator's `Deployment` manifest.

**Command-Line Flags:**

Refer to the output of `kaiwo-operator --help` (or check `cmd/operator/main.go`) for flags controlling metrics, health probes, leader election, and certificate paths.
