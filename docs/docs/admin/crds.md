# Admin Custom Resource Descriptions (CRDs)

!!!note
    Please see [here](../reference/crds/kaiwo.silogen.ai.md) for the full technical description of the available CRDs.

## Config CRDs

### [KaiwoQueueConfig](../reference/crds/kaiwo.silogen.ai.md#kaiwoqueueconfig)

The `KaiwoQueueConfig` CRD (typically a single cluster-scoped resource named `kaiwo`) defines how Kaiwo should configure Kueue resources within the cluster. Kaiwo automatically creates a default `KaiwoQueueConfig` if one doesn't exist, discovering node pools based on the `kaiwo/nodepool` label.

#### `clusterQueues`

A list defining Kueue `ClusterQueue` resources that Kaiwo should manage. Each entry contains:

*   `name`: The name of the ClusterQueue.
*   `spec`: The full `kueuev1beta1.ClusterQueueSpec`, defining resource groups, flavors, cohort, preemption policies, etc.
*   `namespaces`: (Optional) A list of namespaces where Kaiwo should automatically create corresponding `LocalQueue` resources pointing to this `ClusterQueue`.

The Kaiwo controller ensures these `ClusterQueue` resources exist and match the defined spec.

#### `resourceFlavors`

A list defining Kueue `ResourceFlavor` resources that Kaiwo should manage. Each entry contains:

*   `name`: The name of the ResourceFlavor (e.g., `amd-mi300-8gpu-192core-768gi`).
*   `nodeLabels`: A map of labels that pods scheduled with this flavor must match on nodes (e.g., `kaiwo/nodepool: amd-mi300-8gpu-192core-768gi`).
*   `taints`: (Optional) A list of taints associated with this flavor. Pods scheduled with this flavor will automatically get tolerations for these taints.
*   `tolerations`: (Optional) A list of tolerations associated with this flavor (rarely needed if using taints).

The Kaiwo controller ensures these `ResourceFlavor` resources exist and match the defined spec.

#### `workloadPriorityClasses`

A list defining Kueue `WorkloadPriorityClass` resources. Each entry matches the `kueuev1beta1.WorkloadPriorityClass` structure (primarily `name` and `value`). The Kaiwo controller ensures these priority classes exist.