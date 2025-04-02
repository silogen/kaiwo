# Configuration Guide

Administrators configure Kaiwo primarily through the cluster-scoped `KaiwoQueueConfig` Custom Resource Definition (CRD) and environment variables or flags passed to the Kaiwo operator. Users configure the CLI tool separately.

## KaiwoQueueConfig CRD

This is the central point for managing how Kaiwo interacts with Kueue. There should typically be only **one** `KaiwoQueueConfig` resource in the cluster, named `kaiwo`. The Kaiwo operator automatically creates a default one upon startup if it doesn't exist.

You can edit the configuration using:
`kubectl edit kaiwoqueueconfig kaiwo`

**Key Fields (`spec`):**

*   **`resourceFlavors`**: Defines the types of hardware resources available in the cluster, corresponding to Kueue `ResourceFlavor` resources.
    *   `name`: A unique name for the flavor (e.g., `amd-mi300-8gpu`, `nvidia-a100-40gb`, `cpu-standard`).
    *   `nodeLabels`: A map of labels that nodes must possess to be considered part of this flavor. This is crucial for scheduling pods onto the correct hardware. Example: `{"kaiwo/nodepool": "amd-mi300-nodes"}`.
    *   `taints`: (Optional) A list of Kubernetes taints associated with this flavor. Pods scheduled to this flavor will need corresponding tolerations. Kaiwo automatically adds tolerations for GPU taints if `ADD_TAINTS_TO_GPU_NODES` is enabled.

    !!! info "Auto-Discovery"
        If `spec.resourceFlavors` is empty or omitted in the `kaiwo` `KaiwoQueueConfig`, the operator attempts to **auto-discover** node pools based on common GPU labels (e.g., `amd.com/gpu.device-id`, `nvidia.com/gpu.product`, `nvidia.com/gpu.count`) and CPU/Memory capacity. It creates `ResourceFlavor` resources with names like `amd-mi300-8gpu-192core-1024gi` and sets the `nodeLabels` to `{"kaiwo/nodepool": "<generated-flavor-name>"}`. It also labels the corresponding nodes with this label. While convenient, explicitly defining flavors often provides more control.

*   **`clusterQueues`**: Defines the Kueue `ClusterQueue` resources managed by Kaiwo.
    *   `name`: The name of the `ClusterQueue` (e.g., `team-a-queue`, `default-gpu-queue`).
    *   `spec`: The full Kueue `ClusterQueueSpec`. This is where you define resource quotas, cohorts, preemption policies, etc. See [Kueue ClusterQueue Documentation](https://kueue.sigs.k8s.io/docs/concepts/cluster_queue/).
        *   `resourceGroups`: Define sets of flavors and their associated quotas (`nominalQuota`). This links the queue to the available hardware defined in `resourceFlavors`.
        *   `namespaceSelector`: Controls which namespaces can use this queue (via `LocalQueue` resources).
    *   `namespaces`: (Optional) A list of namespace names where Kaiwo should automatically create a Kueue `LocalQueue` pointing to this `ClusterQueue`.

*   **`workloadPriorityClasses`**: Defines Kueue `WorkloadPriorityClass` resources.
    *   Follows the standard Kueue `WorkloadPriorityClass` structure (`name`, `value`, `description`). Kaiwo ensures these exist as defined. See [Kueue Priority Documentation](https://kueue.sigs.k8s.io/docs/concepts/workload_priority_class/).

**Example `KaiwoQueueConfig`:**

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoQueueConfig
metadata:
  name: kaiwo # Must be named 'kaiwo'
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
      namespaces: # Auto-create LocalQueues in these namespaces
        - ai-research-ns-1
        - ai-research-ns-2
      spec: # Standard Kueue ClusterQueueSpec
        # namespaceSelector: {} # Allow all namespaces (if LocalQueues exist)
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

**Synchronization**: The `KaiwoQueueConfigController` continuously watches the `kaiwo` resource and creates, updates, or deletes the corresponding Kueue `ResourceFlavor`, `ClusterQueue`, and `WorkloadPriorityClass` resources to match the spec. It also creates `LocalQueue` resources in the specified namespaces.

## Kaiwo Operator Configuration

The Kaiwo operator deployment itself can be configured using environment variables or command-line flags.

**Key Environment Variables:**

*   `DEFAULT_KAIWO_QUEUE_CONFIG_NAME` (Default: `kaiwo`): The expected name of the single `KaiwoQueueConfig` resource. *Should not typically be changed.*
*   `DEFAULT_CLUSTER_QUEUE_NAME` (Default: `kaiwo`): The default Kueue `ClusterQueue` name used if a user doesn't specify one.
*   `DEFAULT_GPU_TAINT_KEY` (Default: `kaiwo.silogen.ai/gpu`): The taint key applied to GPU nodes if automatic tainting is enabled.
*   `ADD_TAINTS_TO_GPU_NODES` (Default: `true`): If `true`, the `KaiwoQueueConfigController` will attempt to taint nodes identified as having GPUs (based on flavor discovery/definition) with the `DEFAULT_GPU_TAINT_KEY`.
*   `EXCLUDE_MASTER_NODES_FROM_NODE_POOLS` (Default: `false`): If `true`, control-plane/master nodes are excluded during automatic `ResourceFlavor` discovery.
*   `ENFORCE_KAIWO_ON_GPU_WORKLOADS` (Default: `false`): If `true`, the mutating admission webhook for `batchv1.Job` will automatically add the `kaiwo.silogen.ai/managed: "true"` label to any job requesting GPU resources, forcing it to be managed by Kaiwo/Kueue.
*   `RAY_HEAD_POD_MEMORY`: (Optional) Override the default memory request/limit for Ray head pods created by Kaiwo (e.g., `8Gi`).
*   `WEBHOOK_CERT_DIRECTORY`: Path to manually provided webhook certificates (overrides automatic management if set). See [Installation](./installation.md).

These are typically set in the operator's `Deployment` manifest.

**Command-Line Flags:**

Refer to the output of `kaiwo-operator --help` (or check `cmd/operator/main.go`) for flags controlling metrics, health probes, leader election, and certificate paths.

## CLI Configuration (`kaiwoconfig.yaml`)

Users configure the `kaiwo` CLI tool via a YAML file.

**Location Precedence:**

1.  Path specified by `--config <path>` flag in `kaiwo submit`.
2.  Path specified by the `KAIWOCONFIG` environment variable.
3.  Default path: `~/.config/kaiwo/kaiwoconfig.yaml`.

**Fields:**

*   `user`: The user's identifier (typically email) to be associated with submitted workloads (sets `spec.user` and `kaiwo.silogen.ai/user` label).
*   `clusterQueue`: The default Kueue `ClusterQueue` to submit workloads to (sets `spec.clusterQueue` and `kueue.x-k8s.io/queue-name` label).

**Example:**

```yaml
# ~/.config/kaiwo/kaiwoconfig.yaml
user: scientist@example.com
clusterQueue: team-a-queue
```

The `kaiwo submit` command provides an interactive prompt to create this file if it's missing and user/queue information isn't provided via flags.
