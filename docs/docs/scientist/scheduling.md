# Scheduling

Kaiwo allows you to control workload scheduling by requesting compute resources and by specifying Kueue Queues, priorities and Topologies.

!!!note
    Note that this has only been tested on AMD data center GPUs, and while NVIDIA support is planned, it may currently not function as expected, or at all.

!!!note
    The scheduling logic assumes that each node has only single type of GPU installed.

## GPU Resources

You can request GPU resources by including the `gpuResources` field in your workload manifest:

```yaml
spec:
    gpuResources:
        count: 4
```

This will add a `request` and `limit` of `amd.com/gpu: 4` to each of the workload container's resource requests. Any conflicting resource name that you provide in `spec.resources.requests` or `spec.resources.limits` will be overwritten with this value.

!!! info
    All values specified in the `gpuResources` field are *per replica*

!!!info
    As these resource requirements are propagated to each container, increasing the number of replicas will also increase the total number of GPUs requested. For example, if you request a deployment with 2 GPUs and 2 replicas, in total your workload will end up requesting 4 GPUs.

### GPU Models and Vendors

You can specify that your workload should run with only one or more specific GPU model or vendor (default is `amd`), for example:

```yaml
spec:
    gpuResources:
        count: 1
        models: ["mi300x"]
        vendor: amd
```

This ensures that the workload is scheduled on a node with at least one AMD MI300X GPU.

### Partitioned GPUs

Kaiwo detects and supports nodes with partitioned AMD GPUs. In order to ensure compatibility with external tools and dependencies such as Ray, you need to request partitioned GPUs directly by specifying

```yaml
spec:
    gpuResources:
        partitioned: true
```

inside the workload manifest. By default the value is false. This ensures that workloads run only on nodes with GPUs that are either partitioned or not, since by default, both partitioned and non-partitioned nodes report the same `amd.com/gpu` resource and the Kubernetes scheduler and Kueue cannot distringuish between them.

### Requesting vRAM

Rather than directly specifying the number of GPUs you need, you can also specify just the amount of GPU vRAM (per replica) that your workload requires:

```yaml
spec:
    gpuResources:
        totalVram: 230Gi
```

Kaiwo will then inspect the cluster and choose a node type which will minimize the amount of wasted vRAM.  For example, in the above case, if your cluster has both MI300X (192 GB) GPUs and MI250X (128GB) GPUs, Kaiwo would choose the latter, as two MI300X GPUs would leave 154Gi of vRAM unutilized, while two MI250X GPUs would leave only 26 Gi of vRAM unutilized. If you need to control which GPU models your workload uses, you can combine this with the `models` and `vendor` fields.

In case you provide both `count` and `totalVram`, Kaiwo will check if any node in the cluster can satisfy the combination (`nodeVramPerGpu * count <= totalVram`) before scheduling the workload.

### Handling Insufficient Resources

In the case that the cluster does not have the resources that you have requested, or the total amount of the resources are not enough to meet your requests (for example requesting five replicas of 6 GPUs in a cluster with 4 x 8 GPUs), your workload will have a condition `Schedulable: false` and a status `ERROR`.

## Kueue Queues and Resource Flavors

### Cluster inspection

The Kaiwo operator automatically inspects the cluster nodes and adds labels to them to assist with scheduling.

### Resource flavors

The Kaiwo operator creates a resource flavor for each unique combination of:

- CPU cores
- Memory
- AMD GPU number of logical devices
- AMD GPU vRAM per logical device

### Cluster Queues

Kaiwo creates a default queue and associates flavors to them. You or your admin can create other queues and flavors to narrow down the nodes that should be used for any particular workload.

## Replicas

You can specify the number of replicas you want for your workload in the `spec.replicas` field. This field is supported for all workloads apart from the non-Ray KaiwoJob, since Kubernetes Batch Jobs do not support replicas.
