# API Reference

## Packages
- [kaiwo.silogen.ai/v1alpha1](#kaiwosilogenaiv1alpha1)


## kaiwo.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the kaiwo v1 API group.

### Resource Types
- [KaiwoJob](#kaiwojob)
- [KaiwoJobList](#kaiwojoblist)
- [KaiwoQueueConfig](#kaiwoqueueconfig)
- [KaiwoQueueConfigList](#kaiwoqueueconfiglist)
- [KaiwoService](#kaiwoservice)
- [KaiwoServiceList](#kaiwoservicelist)



#### AzureBlobStorageDownloadItem







_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionString` _[ValueReference](#valuereference)_ |  |  |  |
| `containers` _[CloudDownloadBucket](#clouddownloadbucket) array_ |  |  |  |


#### CloudDownloadBucket







_Appears in:_
- [AzureBlobStorageDownloadItem](#azureblobstoragedownloaditem)
- [GCSDownloadItem](#gcsdownloaditem)
- [S3DownloadItem](#s3downloaditem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `files` _[CloudDownloadFile](#clouddownloadfile) array_ |  |  |  |
| `folders` _[CloudDownloadFolder](#clouddownloadfolder) array_ |  |  |  |






#### ClusterQueue







_Appears in:_
- [KaiwoQueueConfigSpec](#kaiwoqueueconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `spec` _[ClusterQueueSpec](#clusterqueuespec)_ |  |  |  |
| `namespaces` _string array_ |  |  |  |


#### CommonMetaSpec



CommonMetaSpec defines reusable metadata fields for workloads.



_Appears in:_
- [KaiwoJobSpec](#kaiwojobspec)
- [KaiwoServiceSpec](#kaiwoservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `annotations` _object (keys:string, values:string)_ | Annotations provides additional metadata for the workload. |  |  |
| `user` _string_ | User specifies the owner or creator of the workload.<br />If authentication is enabled, this must be email address which is checked against authenticated user for match. |  |  |
| `podTemplateSpecLabels` _object (keys:string, values:string)_ |  |  |  |
| `gpus` _integer_ | Gpus specifies the total number of GPUs allocated to the workload.<br />Default is 0. | 0 |  |
| `gpuVendor` _string_ | GpuVendor specifies the GPU vendor (e.g., AMD, NVIDIA, etc.).<br />Default is AMD. | AMD |  |
| `version` _string_ | Version is an optional field specifying the version of the workload. |  |  |
| `replicas` _integer_ | Replicas specifies the number of replicas for the workload.<br />If greater than one, the workload must use Ray.<br />Default is 0. | 1 |  |
| `gpus-per-replica` _integer_ | GpusPerReplica specifies the number of GPUs allocated per replica. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources specify the default resource requirements applied for all pods inside the workflow |  |  |
| `image` _string_ | Image defines the container image used for the workload. |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core)_ | ImagePullSecrets contains the list of secrets used to pull the container image. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core)_ | Env specifies the environment variables to be passed to the container. |  |  |
| `secretVolumes` _[SecretVolume](#secretvolume)_ | SecretVolumes list the secret volumes that should be mounted |  |  |
| `ray` _boolean_ | Ray determines whether the operator should use RayCluster for workload execution.<br />Default is false. | false |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage configuration for the workload. |  |  |
| `dangerous` _boolean_ | Dangerous disables adding the default security context to the containers | false |  |


#### DataStorageSpec







_Appears in:_
- [StorageSpec](#storagespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mountPath` _string_ | MountPath specifies where the data PVC will be mounted to in each pod |  |  |
| `storageSize` _string_ | StorageSize specifies the amount of storage allocated to the data PVC |  |  |
| `download` _[ObjectStorageDownloadSpec](#objectstoragedownloadspec)_ | Download optional object storage pre-downloads |  |  |




#### GCSDownloadItem







_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applicationCredentials` _[ValueReference](#valuereference)_ |  |  |  |
| `buckets` _[CloudDownloadBucket](#clouddownloadbucket) array_ |  |  |  |


#### GitDownloadItem







_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ |  |  |  |
| `branch` _string_ | Branch specifies the branch to use, ignored if commit is given |  |  |
| `commit` _string_ | Commit specifies the commit to use, prioritized over branch |  |  |
| `username` _[ValueReference](#valuereference)_ |  |  |  |
| `token` _[ValueReference](#valuereference)_ |  |  |  |
| `path` _string_ | Path denotes the path within the repository to copy. If not given, whole repository is copied |  |  |
| `targetPath` _string_ | TargetPath denotes where the path is copied to, relative to the data mount directory |  |  |


#### HfStorageSpec







_Appears in:_
- [StorageSpec](#storagespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mountPath` _string_ | MountPath specifies where the data HF will be mounted to in each pod.<br />This is also used to set the HF_HOME environmental variable into each container. |  |  |
| `storageSize` _string_ | StorageSize specifies the amount of storage allocated to the HF PVC |  |  |
| `preCacheRepos` _[HuggingFaceDownloadItem](#huggingfacedownloaditem) array_ | PreCacheRepos is a list of repositories (and their files) that should be cached before the workload starts |  |  |


#### HuggingFaceDownloadItem







_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [HfStorageSpec](#hfstoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repoId` _string_ |  |  |  |
| `files` _string array_ |  |  |  |


#### KaiwoJob







_Appears in:_
- [KaiwoJobList](#kaiwojoblist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoJob` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoJobSpec](#kaiwojobspec)_ |  |  |  |
| `status` _[KaiwoJobStatus](#kaiwojobstatus)_ |  |  |  |


#### KaiwoJobList









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoJobList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[KaiwoJob](#kaiwojob) array_ |  |  |  |


#### KaiwoJobSpec



KaiwoJobSpec defines the desired state of KaiwoJob.



_Appears in:_
- [KaiwoJob](#kaiwojob)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `annotations` _object (keys:string, values:string)_ | Annotations provides additional metadata for the workload. |  |  |
| `user` _string_ | User specifies the owner or creator of the workload.<br />If authentication is enabled, this must be email address which is checked against authenticated user for match. |  |  |
| `podTemplateSpecLabels` _object (keys:string, values:string)_ |  |  |  |
| `gpus` _integer_ | Gpus specifies the total number of GPUs allocated to the workload.<br />Default is 0. | 0 |  |
| `gpuVendor` _string_ | GpuVendor specifies the GPU vendor (e.g., AMD, NVIDIA, etc.).<br />Default is AMD. | AMD |  |
| `version` _string_ | Version is an optional field specifying the version of the workload. |  |  |
| `replicas` _integer_ | Replicas specifies the number of replicas for the workload.<br />If greater than one, the workload must use Ray.<br />Default is 0. | 1 |  |
| `gpus-per-replica` _integer_ | GpusPerReplica specifies the number of GPUs allocated per replica. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources specify the default resource requirements applied for all pods inside the workflow |  |  |
| `image` _string_ | Image defines the container image used for the workload. |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core)_ | ImagePullSecrets contains the list of secrets used to pull the container image. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core)_ | Env specifies the environment variables to be passed to the container. |  |  |
| `secretVolumes` _[SecretVolume](#secretvolume)_ | SecretVolumes list the secret volumes that should be mounted |  |  |
| `ray` _boolean_ | Ray determines whether the operator should use RayCluster for workload execution.<br />Default is false. | false |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage configuration for the workload. |  |  |
| `dangerous` _boolean_ | Dangerous disables adding the default security context to the containers | false |  |
| `clusterQueue` _string_ | ClusterQueue is the Kueue ClusterQueue name. |  |  |
| `priorityClass` _string_ | PriorityClass specifies the Kubernetes PriorityClass for scheduling. |  |  |
| `entrypoint` _string_ | EntryPoint specifies the command or script executed in a Job or RayJob.<br />Can also be defined inside Job struct as Command in the form of string array or<br />inside RayJob struct as Entrypoint in the form of string |  |  |
| `rayJob` _[RayJob](#rayjob)_ | RayJob defines the RayJob configuration. |  |  |
| `job` _[Job](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#job-v1-batch)_ | Job defines the Kubernetes Job configuration. |  |  |


#### KaiwoJobStatus



KaiwoJobStatus defines the observed state of KaiwoJob.



_Appears in:_
- [KaiwoJob](#kaiwojob)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ |  |  |  |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ |  |  |  |
| `status` _[Status](#status)_ |  |  |  |
| `duration` _integer_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |


#### KaiwoQueueConfig



KaiwoQueueConfig manages Kueue resources.



_Appears in:_
- [KaiwoQueueConfigList](#kaiwoqueueconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoQueueConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoQueueConfigSpec](#kaiwoqueueconfigspec)_ |  |  |  |
| `status` _[KaiwoQueueConfigStatus](#kaiwoqueueconfigstatus)_ |  |  |  |


#### KaiwoQueueConfigList









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoQueueConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[KaiwoQueueConfig](#kaiwoqueueconfig) array_ |  |  |  |


#### KaiwoQueueConfigSpec



KaiwoQueueConfigSpec defines the desired configuration for Kaiwo.



_Appears in:_
- [KaiwoQueueConfig](#kaiwoqueueconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterQueues` _[ClusterQueue](#clusterqueue) array_ |  |  | MaxItems: 10 <br /> |
| `resourceFlavors` _[ResourceFlavorSpec](#resourceflavorspec) array_ |  |  |  |
| `workloadPriorityClasses` _WorkloadPriorityClass array_ |  |  | MaxItems: 5 <br /> |


#### KaiwoQueueConfigStatus



KaiwoQueueConfigStatus represents the observed state of KaiwoQueueConfig.



_Appears in:_
- [KaiwoQueueConfig](#kaiwoqueueconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ |  |  |  |
| `Status` _[Status](#status)_ |  |  |  |


#### KaiwoService







_Appears in:_
- [KaiwoServiceList](#kaiwoservicelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoService` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoServiceSpec](#kaiwoservicespec)_ |  |  |  |
| `status` _[KaiwoServiceStatus](#kaiwoservicestatus)_ |  |  |  |


#### KaiwoServiceList









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoServiceList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[KaiwoService](#kaiwoservice) array_ |  |  |  |


#### KaiwoServiceSpec



KaiwoServiceSpec defines the desired state of KaiwoService.



_Appears in:_
- [KaiwoService](#kaiwoservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `annotations` _object (keys:string, values:string)_ | Annotations provides additional metadata for the workload. |  |  |
| `user` _string_ | User specifies the owner or creator of the workload.<br />If authentication is enabled, this must be email address which is checked against authenticated user for match. |  |  |
| `podTemplateSpecLabels` _object (keys:string, values:string)_ |  |  |  |
| `gpus` _integer_ | Gpus specifies the total number of GPUs allocated to the workload.<br />Default is 0. | 0 |  |
| `gpuVendor` _string_ | GpuVendor specifies the GPU vendor (e.g., AMD, NVIDIA, etc.).<br />Default is AMD. | AMD |  |
| `version` _string_ | Version is an optional field specifying the version of the workload. |  |  |
| `replicas` _integer_ | Replicas specifies the number of replicas for the workload.<br />If greater than one, the workload must use Ray.<br />Default is 0. | 1 |  |
| `gpus-per-replica` _integer_ | GpusPerReplica specifies the number of GPUs allocated per replica. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources specify the default resource requirements applied for all pods inside the workflow |  |  |
| `image` _string_ | Image defines the container image used for the workload. |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core)_ | ImagePullSecrets contains the list of secrets used to pull the container image. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core)_ | Env specifies the environment variables to be passed to the container. |  |  |
| `secretVolumes` _[SecretVolume](#secretvolume)_ | SecretVolumes list the secret volumes that should be mounted |  |  |
| `ray` _boolean_ | Ray determines whether the operator should use RayCluster for workload execution.<br />Default is false. | false |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage configuration for the workload. |  |  |
| `dangerous` _boolean_ | Dangerous disables adding the default security context to the containers | false |  |
| `clusterQueue` _string_ | ClusterQueue is the Kueue ClusterQueue name. |  |  |
| `priorityClass` _string_ | PriorityClass specifies the Kubernetes PriorityClass for scheduling. |  |  |
| `entrypoint` _string_ | EntryPoint specifies the command or script executed in a Deployment.<br />Can also be defined inside Deployment struct as regular command in the form of string array |  |  |
| `serveConfigV2` _string_ | Defines the applications and deployments to deploy, should be a YAML multi-line scalar string.<br />Can also be defined inside RayService struct |  |  |
| `rayService` _[RayService](#rayservice)_ | Optional workload-specific configs (Pointers to avoid bloating CRD) |  |  |
| `deployment` _[Deployment](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#deployment-v1-apps)_ |  |  |  |


#### KaiwoServiceStatus



KaiwoServiceStatus defines the observed state of KaiwoService.



_Appears in:_
- [KaiwoService](#kaiwoservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ |  |  |  |
| `status` _[Status](#status)_ |  |  |  |
| `duration` _integer_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |


#### ObjectStorageDownloadSpec







_Appears in:_
- [DataStorageSpec](#datastoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `s3` _[S3DownloadItem](#s3downloaditem) array_ | S3 lists any S3 downloads |  |  |
| `gcs` _[GCSDownloadItem](#gcsdownloaditem) array_ | GCS lists and Google Cloud Storage downloads |  |  |
| `azureBlob` _[AzureBlobStorageDownloadItem](#azureblobstoragedownloaditem) array_ | AzureBlob lists any Azure Blob Storage downloads |  |  |
| `git` _[GitDownloadItem](#gitdownloaditem) array_ | Git lists any Git downloads |  |  |


#### ResourceFlavorSpec



ResourceFlavorSpec defines the configuration for a specific resource flavor.



_Appears in:_
- [KaiwoQueueConfigSpec](#kaiwoqueueconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `nodeLabels` _object (keys:string, values:string)_ | NodeLabels must not exceed a reasonable limit to prevent CRD validation failures |  | MaxProperties: 10 <br /> |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#taint-v1-core) array_ |  |  | MaxItems: 5 <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#toleration-v1-core) array_ | // +kubebuilder:validation:MaxItems=5 |  |  |


#### S3DownloadItem







_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpointUrl` _string_ | EndpointUrl is the endpoint of the S3 API |  |  |
| `accessKeyId` _[ValueReference](#valuereference)_ | AccessKeyId |  |  |
| `secretKey` _[ValueReference](#valuereference)_ |  |  |  |
| `buckets` _[CloudDownloadBucket](#clouddownloadbucket) array_ |  |  |  |


#### SecretVolume







_Appears in:_
- [CommonMetaSpec](#commonmetaspec)
- [KaiwoJobSpec](#kaiwojobspec)
- [KaiwoServiceSpec](#kaiwoservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `secretName` _string_ |  |  |  |
| `key` _string_ |  |  |  |
| `subPath` _string_ |  |  |  |
| `mountPath` _string_ |  |  |  |


#### Status

_Underlying type:_ _string_





_Appears in:_
- [KaiwoJobStatus](#kaiwojobstatus)
- [KaiwoQueueConfigStatus](#kaiwoqueueconfigstatus)
- [KaiwoServiceStatus](#kaiwoservicestatus)

| Field | Description |
| --- | --- |
| `` |  |
| `PENDING` |  |
| `STARTING` |  |
| `READY` |  |
| `RUNNING` |  |
| `COMPLETE` |  |
| `FAILED` |  |


#### StorageSpec



StorageSpec defines the storage configuration for the workload.



_Appears in:_
- [CommonMetaSpec](#commonmetaspec)
- [KaiwoJobSpec](#kaiwojobspec)
- [KaiwoServiceSpec](#kaiwoservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `storageEnabled` _boolean_ | StorageEnabled tells whether to enable persistent storage. |  |  |
| `storageClassName` _string_ | StorageClassName specifies the storage class used for PVC. |  |  |
| `accessMode` _[PersistentVolumeAccessMode](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#persistentvolumeaccessmode-v1-core)_ | AccessMode determines the access mode for the storage | ReadWriteMany |  |
| `data` _[DataStorageSpec](#datastoragespec)_ | Data specifies the main workload PVC and optional object storage pre-downloads |  |  |
| `huggingFace` _[HfStorageSpec](#hfstoragespec)_ | HuggingFace specifies any hugging face models that should be cached before the workload starts |  |  |


#### ValueReference







_Appears in:_
- [AzureBlobStorageDownloadItem](#azureblobstoragedownloaditem)
- [GCSDownloadItem](#gcsdownloaditem)
- [GitDownloadItem](#gitdownloaditem)
- [S3DownloadItem](#s3downloaditem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `file` _string_ | File determines the location of the secret value mounted on the disk |  |  |
| `secretName` _string_ | SecretName is the name of the secret where the value is kept |  |  |
| `secretKey` _string_ | SecretKey is the name of the key within the secret where the value is kept |  |  |


