# Scheduling

## Topology Aware Scheduling (TAS)

Kaiwo supports Kueue's [Topology Aware Scheduling](https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/) to co-locate workload pods within the same topology domain (e.g., same rack or network block). This can significantly improve performance for distributed training by reducing inter-node communication latency.

**TAS is opt-in at the workload level.** If you do not set either field, your workload is scheduled normally without topology constraints.

**Fields:**

*   `preferredTopologyLabel`: Kueue will *try* to place all pods within the topology domain identified by this label. If the pods cannot fit at that level, Kueue moves up to the next topology level. If they cannot fit even at the highest level, pods are distributed across multiple domains. This is a best-effort preference.
*   `requiredTopologyLabel`: Kueue *must* place all pods within a single topology domain at the specified level. If this is not possible, the workload will not be admitted.

**Topology label values** correspond to the levels defined in the cluster's `Topology` resource. Common levels from most specific to least specific:

| Label | Meaning |
| :---- | :------ |
| `kubernetes.io/hostname` | Same physical node |
| `kaiwo/topology-rack` | Same network rack |
| `kaiwo/topology-block` | Same network block |

**Example:**

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: distributed-training
  namespace: ai-research
spec:
  gpus: 16
  gpuVendor: amd
  ray: true
  preferredTopologyLabel: kaiwo/topology-rack  # Try to place all workers in the same rack
  image: my-training-image:latest
```

To *require* placement within a single rack (workload will wait if not possible):

```yaml
spec:
  requiredTopologyLabel: kaiwo/topology-rack
```

!!! note "Prerequisites"
    TAS requires that the cluster administrator has configured a `Topology` and that the `ResourceFlavor` used by your workload has `topologyName` set. If the flavor does not reference a topology, setting these fields will have no effect on scheduling. See the [admin configuration guide](/admin/configuration#topology-aware-scheduling-tas) for setup details.

## Resource Allocation

#### `replicas`, `gpus`, `gpusPerReplica`, and `gpuVendor`

These fields collectively control the number of workload instances and how GPUs are allocated across them. Their interaction depends on the workload type (Job/Service) and whether Ray is used (`ray: true`).

**Purpose:**

*   `replicas`: Sets the desired number of instances (pods). Default: 1. *Ignored for non-Ray Jobs.*
*   `gpus`: Specifies the *total* number of GPUs requested across all replicas. Default: 0.
*   `gpusPerReplica`: Specifies the number of GPUs requested *per* replica. Default: 0.
*   `gpuVendor`: Either `amd` (default) or `nvidia`. Determines the GPU resource key (e.g., `amd.com/gpu`, `nvidia.com/gpu`).

**Behavior:**

1.  **Non-Ray Workloads (`ray: false`)**:
    *   **KaiwoJob:** Only one pod is created. `replicas` is ignored. `gpus` or `gpusPerReplica` (if set > 0) determines the GPU request for the single pod's container. If both `gpus` and `gpusPerReplica` are set, `gpusPerReplica` takes precedence if > 0, otherwise `gpus` is used.
    *   **KaiwoService (Deployment):** `replicas` directly sets the `deployment.spec.replicas`. `gpus` or `gpusPerReplica` (if set > 0) determines the GPU request for *each* replica's container. If both `gpus` and `gpusPerReplica` are set, `gpusPerReplica` takes precedence if > 0, otherwise `gpus` is used (implying `gpusPerReplica = gpus / replicas`, though this division isn't explicitly performed; the request per pod is set based on the determined `gpusPerReplica` value).

2.  **Ray Workloads (`ray: true`)**:
    *   The controller performs a calculation (`CalculateNumberOfReplicas`) considering cluster node capacity (specifically, the minimum GPU capacity available on nodes matching the `gpuVendor`, referred to as `minGpusPerNode`).
    *   **User Precedence:** If the user explicitly sets both `replicas` (> 0) and `gpusPerReplica` (> 0), these values are used *directly*, provided the total requested GPUs (`replicas * gpusPerReplica`) does not exceed the total available GPUs of the specified `gpuVendor` in the cluster. The `gpus` field is ignored in this case.
    *   **Calculation Fallback:** If the user does not explicitly set both `replicas` and `gpusPerReplica`, or if the requested total exceeds cluster capacity, the controller calculates the optimal `replicas` and `gpusPerReplica` based on the `gpus` field and the cluster's `minGpusPerNode`.
        *   The `totalUserRequestedGpus` is determined (using `gpus` field, capped at total cluster capacity).
        *   The final `replicas` is calculated as `ceil(totalUserRequestedGpus / minGpusPerNode)`.
        *   The final `gpusPerReplica` is calculated as `totalUserRequestedGpus / replicas`.
    *   The calculated or user-provided `replicas` value sets the Ray worker group replica count (`minReplicas`, `maxReplicas`, `replicas`). This is due the fact that Kueue does not support Ray's autoscaling.
    *   The calculated or user-provided `gpusPerReplica` value sets the GPU resource request/limit for each Ray worker pod's container.

**Summary Table (Ray Workloads):**

| User Input (`spec.*`)                  | Calculation Performed? | Outcome (`replicas`, `gpusPerReplica`)                                  | Notes                                                                        |
| :------------------------------------- | :--------------------- | :---------------------------------------------------------------------- | :--------------------------------------------------------------------------- |
| `replicas > 0`, `gpusPerReplica > 0` | No\*                   | Uses user's `replicas`, user's `gpusPerReplica`                       | \*If total fits cluster. `gpus` ignored. Highest precedence.                 |
| `gpus > 0` (only)                      | Yes                    | Calculated based on `gpus` and `minGpusPerNode`                         | Aims to maximize GPUs per node up to `minGpusPerNode`.                       |
| `replicas > 0`, `gpus > 0`             | Yes                    | Calculated based on `gpus` and `minGpusPerNode` (user `replicas` ignored) | Falls back to calculation based on total `gpus`.                             |
| `gpusPerReplica > 0`, `gpus > 0`       | Yes                    | Calculated based on `gpus` and `minGpusPerNode` (user `gpusPerReplica` ignored) | Falls back to calculation based on total `gpus`.                             |
| All three set                          | No\*                   | Uses user's `replicas`, user's `gpusPerReplica`                       | \*If total fits cluster (like row 1). Otherwise, calculates based on `gpus`. |
| None set (or only `gpuVendor`)         | No                     | `replicas=1`, `gpusPerReplica=0`                                        | No GPUs requested.                                                           |