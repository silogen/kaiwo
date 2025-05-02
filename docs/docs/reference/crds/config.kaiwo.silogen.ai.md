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
| `data` _[KaiwoDataConfig](#kaiwodataconfig)_ | Data defines the data-specific settings | \{  \} |  |
| `nodes` _[KaiwoNodeConfig](#kaiwonodeconfig)_ | Nodes defines the node configuration settings | \{  \} |  |
| `scheduling` _[KaiwoSchedulingConfig](#kaiwoschedulingconfig)_ | Scheduling contains the configuration Kaiwo uses for workload scheduling | \{  \} |  |
| `resourceMonitoring` _[KaiwoResourceMonitoringConfig](#kaiworesourcemonitoringconfig)_ | ResourceMonitoring defines the resource-monitoring specific settings | \{  \} |  |
| `defaultKaiwoQueueConfigName` _string_ | DefaultKaiwoQueueConfigName is the name of the singleton Kaiwo Queue Config object that is used | kaiwo |  |


#### KaiwoDataConfig







_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultDataMountPath` _string_ | DefaultDataMountPath is the default path for the data storage and downloads that gets mounted in the workload pods.<br />This value can be overwritten in the workload CRD. | /workload |  |
| `defaultHfMountPath` _string_ | DefaultHfMountPath is the default path for the HuggingFace that gets mounted in the workload pods. The `HF_HOME` environmental variable<br />is also set to this value. This value can be overwritten in the workload CRD. | /hf_cache |  |


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
| `defaultRayImage` _string_ | DefaultRayImage is the image that is used for Ray workloads if no image is provided in the workload CRD | ghcr.io/silogen/rocm-ray:v0.9 |  |
| `headPodMemory` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | HeadPodMemory is the amount of memory that is requested for the Ray head pod |  |  |


#### KaiwoResourceMonitoringConfig



KaiwoResourceMonitoringConfig configures the resource monitoring feature.
Note that the following must be set as environmental variables inside the Kaiwo controller manager as these cannot be updated without restarting the operator process.


* Enabling the resource monitoring feature (`RESOURCE_MONITORING_ENABLED=true`)
* Setting the Prometheus endpoint (`RESOURCE_MONITORING_PROMETHEUS_ENDPOINT=...`)
* Setting the polling interval (`RESOURCE_MONITORING_POLLING_INTERVAL=10m`)



_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `averagingTime` _string_ | AveragingTime is the time to use to average the metrics over | 20m | Pattern: `^([0-9]+(s\|m\|h))+$` <br /> |
| `minAliveTime` _string_ | MinAliveTime is the time that a pod must have been alive for in order to qualify for inspection | 20m | Pattern: `^([0-9]+(s\|m\|h))+$` <br /> |
| `lowUtilizationThreshold` _float_ | LowUtilizationThreshold is the threshold which, if the metric goes under, the workload is considered underutilized. The threshold is interpreted as the percentage utilization versus the requested capacity. | 20 | Minimum: 0 <br /> |
| `targetNamespaces` _string array_ | TargetNamespaces is a list of namespaces to apply the monitoring to. If not supplied or empty, all namespaces apart from kube-system will be inspected. However, only pods associated with KaiwoJobs or KaiwoServices are impacted. |  |  |
| `profile` _string_ | Profile chooses the target resource to monitor. | gpu | Enum: [gpu cpu] <br /> |


#### KaiwoSchedulingConfig



KaiwoSchedulingConfig contains the configuration Kaiwo uses for workload scheduling



_Appears in:_
- [KaiwoConfigSpec](#kaiwoconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kubeSchedulerName` _string_ | KubeSchedulerName defines the default scheduler name that is used to schedule the workload | kaiwo-scheduler |  |


