# API Reference

## Packages
- [config.kaiwo.silogen.ai/v1alpha1](#configkaiwosilogenaiv1alpha1)


## config.kaiwo.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the kaiwo configuration v1alpha1 API group.

### Resource Types
- [KaiwoConfig](#kaiwoconfig)
- [KaiwoConfigList](#kaiwoconfiglist)



#### KaiwoConfig



KaiwoConfig manages the Kaiwo operator's configuration which can be modified during runtime.



_Appears in:_
- [KaiwoConfigList](#kaiwoconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `config.kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoConfigSpec](#kaiwoconfigspec)_ | Spec defines the desired state for the Kaiwo operator configuration. |  |  |


#### KaiwoConfigList



KaiwoConfigList contains a list of KaiwoConfig resources.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `config.kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[KaiwoConfig](#kaiwoconfig) array_ |  |  |  |


#### KaiwoConfigSpec



KaiwoConfigSpec defines the desired configuration for the Kaiwo operator's configuration.
There should typically be only one KaiwoConfig resource in the cluster.



_Appears in:_
- [KaiwoConfig](#kaiwoconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ray` _[KaiwoRayConfig](#kaiworayconfig)_ | Ray defines the Ray-specific settings | \{  \} |  |
| `storage` _[KaiwoStorageConfig](#kaiwostorageconfig)_ | Storage defines the storage-specific settings | \{  \} |  |
| `nodes` _[KaiwoNodeConfig](#kaiwonodeconfig)_ | Nodes defines the node configuration settings | \{  \} |  |
| `scheduling` _[KaiwoSchedulingConfig](#kaiwoschedulingconfig)_ | Scheduling contains the configuration Kaiwo uses for workload scheduling | \{  \} |  |
| `resourceMonitoring` _[KaiwoResourceMonitoringConfig](#kaiworesourcemonitoringconfig)_ | ResourceMonitoring defines the resource-monitoring specific settings | \{  \} |  |
| `defaultClusterQueueName` _string_ | DefaultClusterQueueName is the name of the default cluster queue that is used for workloads that don't explicitly specify a cluster queue. | kaiwo |  |
| `defaultClusterQueueCohortName` _string_ | DefaultClusterQueueCohortName is the name of the default cohort that is used for the default cluster queue.<br />ClusterQueues in the same cohort can share resources. | kaiwo |  |
| `dynamicallyUpdateDefaultClusterQueue` _boolean_ | DynamicallyUpdateDefaultClusterQueue defines whether the Kaiwo operator should dynamically update default "kaiwo" clusterqueue.<br />If set to true, the operator will make sure that the default clusterqueue is always up to date and reflects total resources available.<br />If nodes are added or removed, the operator will update the default clusterqueue to reflect the current state of the cluster. | false |  |


#### KaiwoNodeConfig







_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultGpuResourceKey` _string_ | DefaultGpuResourceKey defines the default GPU resource key that is used to reserve GPU capacity for pods | amd.com/gpu |  |
| `defaultGpuTaintKey` _string_ | DefaultGpuTaintKey is the key that is used to taint GPU nodes | kaiwo.silogen.ai/gpu |  |
| `excludeMasterNodesFromNodePools` _boolean_ | ExcludeMasterNodesFromNodePools allows excluding the master node(s) from the node pools | false |  |
| `addTaintsToGpuNodes` _boolean_ | AddTaintsToGpuNodes if set to true, will add the DefaultGpuTaintKey taint to the GPU nodes | false |  |


#### KaiwoRayConfig



KaiwoRayConfig contains the Ray-specific configuration that Kaiwo uses.



_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultRayImage` _string_ | DefaultRayImage is the image that is used for Ray workloads if no image is provided in the workload CRD | ghcr.io/silogen/rocm-ray:6.4 |  |
| `headPodMemory` _string_ | HeadPodMemory is the amount of memory that is requested for the Ray head pod | 16Gi |  |


#### KaiwoResourceMonitoringConfig



KaiwoResourceMonitoringConfig configures the resource monitoring feature.
Note that the following must be set as environmental variables inside the Kaiwo controller manager as these cannot be updated without restarting the operator process.

* Enabling the resource monitoring feature (`RESOURCE_MONITORING_ENABLED=true`)
* Setting the metrics endpoint (`RESOURCE_MONITORING_METRICS_ENDPOINT=...`)
* Setting the polling interval (`RESOURCE_MONITORING_POLLING_INTERVAL=30s`)



_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled turns on resource monitoring | false |  |
| `metricsServiceConfig` _[MetricsServiceConfig](#metricsserviceconfig)_ | MetricsServiceConfig contains the configuration required to connect to the GPU metrics service |  |  |
| `pollInterval` _string_ | PollInterval is the interval which is used to poll the metrics exporters | 30s | Pattern: `^([0-9]+(s\|m\|h))+$` <br /> |
| `lowUtilizationThreshold` _float_ | LowUtilizationThreshold is the threshold which, if the metric goes under, the workload is considered underutilized. The threshold is interpreted as the percentage utilization versus the requested capacity. | 1 | Minimum: 0 <br /> |
| `targetNamespaces` _string array_ | TargetNamespaces is a list of namespaces to apply the monitoring to. If not supplied or empty, all namespaces apart from kube-system will be inspected. However, only pods associated with KaiwoJobs or KaiwoServices are impacted. |  |  |
| `profile` _string_ | Profile chooses the target resource to monitor. | gpu | Enum: [gpu] <br /> |
| `terminateUnderutilized` _boolean_ | TerminateUnderutilized will terminate workloads that are underutilizing resources if set to `true` | false |  |
| `terminateUnderutilizedAfter` _string_ | TerminateUnderutilizedAfter specifies the duration after which the workload will be terminated if it has been underutilizing resources (for this amount of time) | 24h | Pattern: `^([0-9]+(s\|m\|h))+$` <br /> |


#### KaiwoSchedulingConfig



KaiwoSchedulingConfig contains the configuration Kaiwo uses for workload scheduling



_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kubeSchedulerName` _string_ | KubeSchedulerName defines the default scheduler name that is used to schedule the workload | kaiwo-scheduler |  |
| `pendingThresholdForPreemption` _string_ | PendingThresholdForPreemption is the threshold that is used to determine if a workload is awaiting for compute resources to be available.<br />If the workload is requesting GPUs and pending for longer than this threshold, kaiwo will start preempting workloads that have exceeded their duration deadline and are using GPUs of the same vendor as the pending workload. | 5m |  |
| `strictNodeMatching` _boolean_ | StrictNodeMatching, if true, will only consider nodes that have all the expected labels present (logical vs. physical count / vRAM, partitioning, etc.)<br />If the value is false and the workload request does not request partitioned GPUs, scheduling onto nodes that do not specify the partitioning type are also allowed. | true |  |


#### KaiwoStorageConfig







_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultStorageClass` _string_ | DefaultStorageClass is the storage class that is used for workloads that don't explicitly specify a storage class. |  |  |
| `defaultDataMountPath` _string_ | DefaultDataMountPath is the default path for the data storage and downloads that gets mounted in the workload pods.<br />This value can be overwritten in the workload CRD. | /workload |  |
| `defaultHfMountPath` _string_ | DefaultHfMountPath is the default path for the HuggingFace that gets mounted in the workload pods. The `HF_HOME` environmental variable<br />is also set to this value. This value can be overwritten in the workload CRD. | /hf_cache |  |


#### MetricsServiceConfig







_Appears in:_
- [KaiwoResourceMonitoringConfig](#kaiworesourcemonitoringconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespace` _string_ |  |  |  |
| `name` _string_ |  |  |  |
| `port` _integer_ |  |  |  |


