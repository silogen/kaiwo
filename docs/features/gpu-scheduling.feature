Feature: Kaiwo supports scheduling GPU workloads

  While CPU and memory requests and limits can be placed in the `spec.resources` field, GPU requests are placed in the `spec.gpuResources` field. In short, this configuration contains the following fields (not all are required):

  ```yaml
  spec:
    gpuResources:
      # Three logical GPUs
      count: 3

      # A total of 72Gi of logical vRAM
      totalVram: 72Gi

      # Choosing GPU partitioning
      partitioned: null|true|false

      # Choosing AMD GPUs (the default)
      vendor: amd|nvidia

      # Limiting to certain GPUs
      models:
        - mi300x
  ```

  These resources are defined as the _per replica_ resource requirements.

  Glossary:
    - *Logical* GPUs / vRAM: The number of resources that are seen by Kubernetes and by containers that are scheduled onto a node. If the GPU is not partitioned, these values are the same as what the physical GPU provides

  Background:
    Given a cluster with GPU nodes, each labeled (by the Kaiwo operator) with:
      | label                                      | meaning                         |
      |--------------------------------------------|---------------------------------|
      | kaiwo.silogen.ai/partitioned               | true, false                     |
      | kaiwo.silogen.ai/gpu.logical-count         | 64, 8                           |
      | kaiwo.silogen.ai/gpu.logical-vram          | 24Gi, 192Gi                     |
      | kaiwo.silogen.ai/gpu.vendor                | amd, nvidia                     |
      | kaiwo.silogen.ai/gpu.model                 | mi300x, mi250                   |
    And the Kaiwo operator is running

  Rule: Basic GPU requests
    Scenario: Requesting GPUs by count only
      When the user specifies:
        | count | 3 |
      And the cluster has at least one node with ≥ 3 logical GPUs
      Then exactly 3 GPUs are allocated on a non-partitioned node by default

    Scenario: Requesting GPUs by totalVram only
      When the user specifies:
        | totalVram | 72Gi |
      And the cluster has nodes where some GPU combinations meet or exceed 72Gi vRAM
      Then Kaiwo computes the minimal number of GPUs (rounded up) per node
      And schedules on the node whose GPUs minimize (nodeVram*gpuCount − 72Gi)

  Rule: Combined resource requests
    Scenario: Count plus vRAM
      When the user specifies:
        | count               | 4 |
        | totalVram           | 80Gi |
      And there exist nodes where 4 GPUs provide ≥ 80Gi vRAM
      Then Kaiwo schedules on such node

    Scenario: vRAM only
      When the user specifies:
        | totalVram         | 100Gi |
      And the cluster has nodes that can satisfy the GPU vRAM on a single node
      Then Kaiwo chooses the node minimizing resource waste

  Rule: Filters and defaults
    Scenario: GPU partitioning filter
      When the user specifies `partitioned: true|false`
      Then only partitioned|unpartitioned nodes are considered

    Scenario: No GPU partition specified
      When the user omits `partitioned` or sets it to `null` (the default)
      Then all node types are considered (partitioned or unpartitioned)

    Scenario: GPU vendor filter
      When the user specifies `vendor: nvidia`
      Then only NVIDIA-GPU nodes are considered

    Scenario: Vendor default
      When the user omits `vendor`
      Then `amd` is used by default

    Scenario: GPU models as filter
      When the user specifies `models: [mi300x, mi250]`
      Then only nodes whose GPU model is in that list are considered

  Rule: Minimize GPUs scheduling strategy

    The minimize-gpus strategy optimizes for the least number of GPUs, even if some of the GPUs are not completely filled.

    Background:
      Given a cluster with GPU nodes offering either 24Gi or 192Gi vRAM per GPU
      And a KaiwoConfig with:
        | spec.scheduling.gpuFillThreshold | 0.5           |
        | spec.scheduling.gpuFillStrategy  | minimize-gpus |

    Scenario: Low-vRAM request uses multiple small GPUs with minimize-gpus
      When the user requests:
        | spec.gpuResources.totalVram | 50Gi |
      Then Kaiwo computes fill ratios for each tier:
        | tier  | size | n | totalCap | fillRatio |
        | small | 24Gi | 3 | 72Gi     | 0.69      |
        | large | 192Gi| 1 | 192Gi    | 0.26      |
      And filters out tiers with fillRatio < 0.5
      And selects three 24Gi GPUs (3×24Gi) rather than 1×192Gi

    Scenario: High-vRAM request uses one large GPU with minimize-gpus
      When the user requests:
        | spec.gpuResources.totalVram | 140Gi |
      Then Kaiwo computes fill ratios for each tier:
        | tier  | size | n | totalCap | fillRatio |
        | small | 24Gi | 6 | 144Gi    | 0.97      |
        | large | 192Gi| 1 | 192Gi    | 0.73      |
      And filters tiers with fillRatio ≥ 0.5
      And selects one 192Gi GPU (1×192Gi) rather than six 24Gi GPUs

  Rule: Minimize GPU waste scheduling strategy

    The `minimize-waste` strategy minimizes the amount of wasted resources, even if it increases the number of GPUs requested.

    Background:
      Given a cluster with GPU nodes offering either 24Gi or 192Gi vRAM per GPU
      And a KaiwoConfig with:
        | spec.scheduling.gpuFillThreshold | 0.5            |
        | spec.scheduling.gpuFillStrategy  | minimize-waste |

    Scenario: Low-vRAM request uses multiple small GPUs with minimize-waste
      When the user requests:
        | spec.gpuResources.totalVram | 50Gi |
      Then Kaiwo computes waste for each tier:
        | tier  | size | n | totalCap | waste |
        | small | 24Gi | 3 | 72Gi     | 22Gi |
        | large | 192Gi| 1 | 192Gi    | 142Gi|
      And selects the tier with minimal waste → three 24Gi GPUs (3×24Gi)

    Scenario: High-vRAM request uses multiple small GPUs with minimize-waste
      When the user requests:
        | spec.gpuResources.totalVram | 140Gi |
      Then Kaiwo computes waste for each tier:
        | tier  | size | n | totalCap | waste |
        | small | 24Gi | 6 | 144Gi    | 4Gi  |
        | large | 192Gi| 1 | 192Gi    | 52Gi |
      And selects the tier with minimal waste → six 24Gi GPUs (6×24Gi)

  Rule: Multi-replica scheduling
    Scenario: All replicas must fit into the cluster
      Given `replicas: 5`
      And each replica requests `count: 2`
      And the cluster has only two nodes each with 4 GPUs
      Then scheduling succeeds (5 replicas ×2 GPUs = 10 GPUs total)

    Scenario: Replica cannot fit on any single node
      Given `replicas: 1`
      And the user requests `count: 5`
      And every node has only 4 GPUs
      Then scheduling fails

    Scenario: Workload cannot fit into cluster
      Given `replicas: 3`
      And the user requests `count: 3`
      And every node has only 4 GPUs
      And there are two nodes
      Then scheduling fails

  Rule: Validation and errors
    Scenario: At least count or vRAM are required
      When the user specified `gpuResources` but omits `count` and `totalVram`
      Then the scheduling fails

    Scenario: Vendor and models mismatch
      When the user specifies `vendor: nvidia` and `models: [mi300x]`
      Then scheduling fails because no matching nodes are found

    Scenario: Count exceeds per-node capacity
      When the user specifies `count: 10`
      And each node has only 8 GPUs
      Then scheduling fails

    Scenario: Standard resource-gpu fields are ignored
      When the user specifies:
        | spec.resources.limits."amd.com/gpu"  |  2  |
        | spec.gpuResources.count              |  3  |
        | spec.gpuResources.vendor             | amd |
      Then Kaiwo ignores the `resources.limits` setting and allocates 3 GPUs as per `spec.gpuResources.count`

    Scenario: The user requests a GPU configuration that cannot be satisfied by the resources available within the cluster
      When the user requests GPU resources, for example the number or type of GPUs, GPU partitioning, GPU models, or more GPUs than are available within the cluster
      Then scheduling fails
