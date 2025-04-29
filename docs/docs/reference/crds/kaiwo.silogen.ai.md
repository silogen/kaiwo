---
hide:
  - navigation
---

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



AzureBlobStorageDownloadItem defines parameters for downloading data from Azure Blob Storage.



_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionString` _[ValueReference](#valuereference)_ | ConnectionString references a Kubernetes Secret containing the Azure Storage connection string. See `ValueReference`. |  |  |
| `containers` _[CloudDownloadBucket](#clouddownloadbucket) array_ | Containers lists the Azure Blob Storage containers and the specific files/folders to download from them. See `CloudDownloadBucket`. |  |  |


#### CloudDownloadBucket



CloudDownloadBucket represents a specific bucket (S3, GCS) or container (Azure) to download from.



_Appears in:_
- [AzureBlobStorageDownloadItem](#azureblobstoragedownloaditem)
- [GCSDownloadItem](#gcsdownloaditem)
- [S3DownloadItem](#s3downloaditem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the bucket or container. |  |  |
| `files` _[CloudDownloadFile](#clouddownloadfile) array_ | Files lists specific files to download from this bucket/container. |  |  |
| `folders` _[CloudDownloadFolder](#clouddownloadfolder) array_ | Folders lists specific folders (prefixes) to download from this bucket/container. |  |  |






#### ClusterQueue



ClusterQueue defines the configuration for a Kueue ClusterQueue managed by Kaiwo.



_Appears in:_
- [KaiwoQueueConfigSpec](#kaiwoqueueconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name specifies the name of the Kueue ClusterQueue resource. |  |  |
| `spec` _[ClusterQueueSpec](#clusterqueuespec)_ | Spec contains the desired Kueue `ClusterQueueSpec`. Kaiwo ensures the corresponding ClusterQueue resource matches this spec. See Kueue documentation for `ClusterQueueSpec` fields like `resourceGroups`, `cohort`, `preemption`, etc. |  |  |
| `namespaces` _string array_ | Namespaces optionally lists Kubernetes namespaces where Kaiwo should automatically create a Kueue `LocalQueue` resource pointing to this ClusterQueue. |  |  |


#### CommonMetaSpec



CommonMetaSpec defines reusable metadata fields for workloads.



_Appears in:_
- [KaiwoJobSpec](#kaiwojobspec)
- [KaiwoServiceSpec](#kaiwoservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `user` _string_ | User specifies the owner or creator of the workload. It should typically be the user's email address. This value is primarily used for labeling (`kaiwo.silogen.ai/user`) the generated resources (like Pods, Jobs, Deployments) for identification and filtering (e.g., with `kaiwo list --user <email>`).<br /><br />In the future, if authentication is enabled, this must be the email address which is checked against authenticated user for match. |  |  |
| `podTemplateSpecLabels` _object (keys:string, values:string)_ | PodTemplateSpecLabels allows you to specify custom labels that will be added to the `template.metadata.labels` section of the generated Pods (within Jobs, Deployments, or RayCluster specs). Standard Kaiwo system labels (like `kaiwo.silogen.ai/user`, `kaiwo.silogen.ai/name`, etc.) are added automatically and take precedence if there are conflicts. |  |  |
| `gpus` _integer_ | Gpus specifies the total number of GPUs allocated to the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | 0 |  |
| `gpuVendor` _string_ | GpuVendor specifies the GPU vendor (e.g., amd, nvidia, etc.). See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | amd |  |
| `version` _string_ | Version allows you to specify an optional version string for the workload. This can be useful for tracking different iterations or configurations of the same logical workload. It does not directly affect resource creation but serves as metadata. |  |  |
| `replicas` _integer_ | Replicas specifies the number of replicas for the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | 1 |  |
| `gpusPerReplica` _integer_ | GpusPerReplica specifies the number of GPUs allocated per replica. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.<br /><br />If you specify `gpusPerReplica`, you must also specify `replicas`. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources specify the default resource requirements applied for all pods inside the workflow.<br /><br />This field defines default Kubernetes `ResourceRequirements` (requests and limits for CPU,<br />memory, ephemeral-storage) applied to *all* containers (including init containers) within<br />the workload's pods.<br /><br />**Behavior:**<br /><br />These values act as **defaults**. If a container within the underlying Job, Deployment,<br />or Ray spec (if provided by the user) already defines a specific request or limit<br />(e.g., `memory` limit), the value from `resources` for that specific metric **will not** override it.<br /><br />**Interaction with GPU fields:** The GPU requests/limits (`amd.com/gpu` or `nvidia.com/gpu`)<br />are controlled exclusively by the `gpus`, `gpusPerReplica`, and `gpuVendor` fields<br />(and the associated calculation logic described above). Any GPU specifications within<br />the `resources` field are **ignored**.<br /><br />**Default CPU/Memory with GPUs:** When Kaiwo *generates* the underlying<br />Job/Deployment/RayCluster spec (i.e., the user did *not* provide `spec.job`,<br />`spec.deployment`, or `spec.rayService`/`spec.rayJob`), and GPUs are requested<br />(`gpusPerReplica` > 0), Kaiwo applies default CPU and Memory requests/limits<br />based on the GPU count (e.g., 4 CPU cores and 32Gi Memory per GPU).<br />These GPU-derived defaults *will* override any CPU/Memory settings defined in<br />the `resources` field in this specific scenario. If the user *does* provide<br />the underlying spec, these GPU-derived CPU/Memory defaults are not applied,<br />respecting the user's definition or the values from the `resources` field. |  |  |
| `image` _string_ | Image specifies the default container image to be used for the primary workload container(s).<br /><br />- If containers defined within the underlying Job, Deployment, or Ray spec do *not* specify an image, this image will be used.<br />- If this field is also empty, the latest tag of ghcr.io/silogen/rocm-ray is used |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets is a list of Kubernetes `LocalObjectReference` (containing just the secret `name`) referencing secrets needed to pull the container image(s). These are added to the `imagePullSecrets` field of the PodSpec for all generated pods. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env is a list of Kubernetes `EnvVar` structs. These environment variables are added to the primary workload container(s) in the generated pods. They are appended to any environment variables already defined in the underlying Job, Deployment, or Ray spec. |  |  |
| `secretVolumes` _[SecretVolume](#secretvolume) array_ | SecretVolumes allows you to mount specific keys from Kubernetes Secrets as files into the workload containers. |  |  |
| `ray` _boolean_ | Ray determines whether the operator should use RayCluster for workload execution.<br />If `true`, Kaiwo will create Ray-specific resources.<br />If `false` (default), Kaiwo will create standard Kubernetes resources (BatchJob for `KaiwoJob`, Deployment for `KaiwoService`).<br />This setting dictates which underlying spec (`job`/`rayJob` or `deployment`/`rayService`) is primarily used. | false |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage configures persistent storage using Kubernetes PersistentVolumeClaims (PVCs).<br /><br />Enabling `storage.data.download` or `storage.huggingFace.preCacheRepos` will cause Kaiwo to create a temporary Kubernetes Job (the "download job") before starting the main workload. This job runs a container that performs the downloads into the respective PVCs. The main workload only starts after the download job completes successfully. |  |  |
| `dangerous` _boolean_ | Dangerous, if when set to `true`, Kaiwo will *not* add the default `PodSecurityContext` (which normally sets `runAsUser: 1000`, `runAsGroup: 1000`, `fsGroup: 1000`) to the generated pods. Use this only if you need to run containers as root or a different specific user and understand the security implications. | false |  |


#### DataStorageSpec



DataStorageSpec configures the primary data volume for the workload.



_Appears in:_
- [StorageSpec](#storagespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mountPath` _string_ | MountPath specifies the path inside the workload containers where the data PersistentVolumeClaim will be mounted. | /workload |  |
| `storageSize` _string_ | StorageSize specifies the requested size for the data PersistentVolumeClaim (e.g., "100Gi", "1Ti"). If set, a PVC will be created. |  |  |
| `download` _[ObjectStorageDownloadSpec](#objectstoragedownloadspec)_ | Download configures optional tasks to download data from various sources into the data volume *before* the main workload starts. See `ObjectStorageDownloadSpec`. |  |  |




#### GCSDownloadItem



GCSDownloadItem defines parameters for downloading data from Google Cloud Storage.



_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applicationCredentials` _[ValueReference](#valuereference)_ | ApplicationCredentials references a Kubernetes Secret containing the GCS service account key JSON file content. See `ValueReference`. |  |  |
| `buckets` _[CloudDownloadBucket](#clouddownloadbucket) array_ | Buckets lists the GCS buckets and the specific files/folders to download from them. See `CloudDownloadBucket`. |  |  |


#### GitDownloadItem



GitDownloadItem defines parameters for cloning a Git repository or parts of it.



_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ | Repository specifies the Git repository URL (e.g., "https://github.com/user/repo.git"). |  |  |
| `branch` _string_ | Branch specifies the branch to clone. This takes precedence over `commit`.  |  |  |
| `commit` _string_ | Commit specifies the exact commit hash to check out. This is ignored if `branch` is specified. |  |  |
| `username` _[ValueReference](#valuereference)_ | Username optionally references a Secret containing the Git username for authentication. See `ValueReference`. |  |  |
| `token` _[ValueReference](#valuereference)_ | Token optionally references a Secret containing the Git token (or password) for authentication. See `ValueReference`. |  |  |
| `path` _string_ | Path specifies a sub-path within the repository to copy. If omitted, the entire repository is copied. |  |  |
| `targetPath` _string_ | TargetPath specifies the destination path relative to the data volume's mount point (`DataStorageSpec.MountPath`) where the repository or `path` content should be copied. |  |  |


#### HfStorageSpec



HfStorageSpec configures storage specifically for Hugging Face model caching.



_Appears in:_
- [StorageSpec](#storagespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mountPath` _string_ | MountPath specifies the path inside workload containers where the Hugging Face cache PVC will be mounted.<br />This path is also automatically set as the `HF_HOME` environment variable in the containers. | /hf_cache |  |
| `storageSize` _string_ | StorageSize specifies the requested size for the Hugging Face cache PersistentVolumeClaim (e.g., "50Gi", "200Gi"). If set, a PVC will be created. |  |  |
| `preCacheRepos` _[HuggingFaceDownloadItem](#huggingfacedownloaditem) array_ | PreCacheRepos is a list of Hugging Face repositories to download into the cache volume *before* the main workload starts. |  |  |


#### HuggingFaceDownloadItem



HuggingFaceDownloadItem defines parameters for pre-caching a Hugging Face repository or specific files from it.



_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [HfStorageSpec](#hfstoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repoId` _string_ | RepoID is the Hugging Face Hub repository ID (e.g., "meta-llama/Llama-2-7b-chat-hf"). |  |  |
| `files` _string array_ | Files is an optional list of specific files to download from the repository. If omitted, the entire repository is downloaded. |  |  |


#### KaiwoJob



KaiwoJob represents a batch workload managed by Kaiwo. It encapsulates either a standard Kubernetes Job or a RayJob, along with common metadata, storage configurations, and scheduling preferences. The Kaiwo controller reconciles this resource to create and manage the underlying workload objects.



_Appears in:_
- [KaiwoJobList](#kaiwojoblist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoJob` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoJobSpec](#kaiwojobspec)_ | Spec defines the desired state of the KaiwoJob, including workload type (Job/RayJob), configuration, resources, and common metadata. |  |  |
| `status` _[KaiwoJobStatus](#kaiwojobstatus)_ | Status reflects the most recently observed state of the KaiwoJob, including its phase, start/completion times, and conditions. |  |  |


#### KaiwoJobList



KaiwoJobList





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
| `user` _string_ | User specifies the owner or creator of the workload. It should typically be the user's email address. This value is primarily used for labeling (`kaiwo.silogen.ai/user`) the generated resources (like Pods, Jobs, Deployments) for identification and filtering (e.g., with `kaiwo list --user <email>`).<br /><br />In the future, if authentication is enabled, this must be the email address which is checked against authenticated user for match. |  |  |
| `podTemplateSpecLabels` _object (keys:string, values:string)_ | PodTemplateSpecLabels allows you to specify custom labels that will be added to the `template.metadata.labels` section of the generated Pods (within Jobs, Deployments, or RayCluster specs). Standard Kaiwo system labels (like `kaiwo.silogen.ai/user`, `kaiwo.silogen.ai/name`, etc.) are added automatically and take precedence if there are conflicts. |  |  |
| `gpus` _integer_ | Gpus specifies the total number of GPUs allocated to the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | 0 |  |
| `gpuVendor` _string_ | GpuVendor specifies the GPU vendor (e.g., amd, nvidia, etc.). See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | amd |  |
| `version` _string_ | Version allows you to specify an optional version string for the workload. This can be useful for tracking different iterations or configurations of the same logical workload. It does not directly affect resource creation but serves as metadata. |  |  |
| `replicas` _integer_ | Replicas specifies the number of replicas for the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | 1 |  |
| `gpusPerReplica` _integer_ | GpusPerReplica specifies the number of GPUs allocated per replica. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.<br /><br />If you specify `gpusPerReplica`, you must also specify `replicas`. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources specify the default resource requirements applied for all pods inside the workflow.<br /><br />This field defines default Kubernetes `ResourceRequirements` (requests and limits for CPU,<br />memory, ephemeral-storage) applied to *all* containers (including init containers) within<br />the workload's pods.<br /><br />**Behavior:**<br /><br />These values act as **defaults**. If a container within the underlying Job, Deployment,<br />or Ray spec (if provided by the user) already defines a specific request or limit<br />(e.g., `memory` limit), the value from `resources` for that specific metric **will not** override it.<br /><br />**Interaction with GPU fields:** The GPU requests/limits (`amd.com/gpu` or `nvidia.com/gpu`)<br />are controlled exclusively by the `gpus`, `gpusPerReplica`, and `gpuVendor` fields<br />(and the associated calculation logic described above). Any GPU specifications within<br />the `resources` field are **ignored**.<br /><br />**Default CPU/Memory with GPUs:** When Kaiwo *generates* the underlying<br />Job/Deployment/RayCluster spec (i.e., the user did *not* provide `spec.job`,<br />`spec.deployment`, or `spec.rayService`/`spec.rayJob`), and GPUs are requested<br />(`gpusPerReplica` > 0), Kaiwo applies default CPU and Memory requests/limits<br />based on the GPU count (e.g., 4 CPU cores and 32Gi Memory per GPU).<br />These GPU-derived defaults *will* override any CPU/Memory settings defined in<br />the `resources` field in this specific scenario. If the user *does* provide<br />the underlying spec, these GPU-derived CPU/Memory defaults are not applied,<br />respecting the user's definition or the values from the `resources` field. |  |  |
| `image` _string_ | Image specifies the default container image to be used for the primary workload container(s).<br /><br />- If containers defined within the underlying Job, Deployment, or Ray spec do *not* specify an image, this image will be used.<br />- If this field is also empty, the latest tag of ghcr.io/silogen/rocm-ray is used |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets is a list of Kubernetes `LocalObjectReference` (containing just the secret `name`) referencing secrets needed to pull the container image(s). These are added to the `imagePullSecrets` field of the PodSpec for all generated pods. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env is a list of Kubernetes `EnvVar` structs. These environment variables are added to the primary workload container(s) in the generated pods. They are appended to any environment variables already defined in the underlying Job, Deployment, or Ray spec. |  |  |
| `secretVolumes` _[SecretVolume](#secretvolume) array_ | SecretVolumes allows you to mount specific keys from Kubernetes Secrets as files into the workload containers. |  |  |
| `ray` _boolean_ | Ray determines whether the operator should use RayCluster for workload execution.<br />If `true`, Kaiwo will create Ray-specific resources.<br />If `false` (default), Kaiwo will create standard Kubernetes resources (BatchJob for `KaiwoJob`, Deployment for `KaiwoService`).<br />This setting dictates which underlying spec (`job`/`rayJob` or `deployment`/`rayService`) is primarily used. | false |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage configures persistent storage using Kubernetes PersistentVolumeClaims (PVCs).<br /><br />Enabling `storage.data.download` or `storage.huggingFace.preCacheRepos` will cause Kaiwo to create a temporary Kubernetes Job (the "download job") before starting the main workload. This job runs a container that performs the downloads into the respective PVCs. The main workload only starts after the download job completes successfully. |  |  |
| `dangerous` _boolean_ | Dangerous, if when set to `true`, Kaiwo will *not* add the default `PodSecurityContext` (which normally sets `runAsUser: 1000`, `runAsGroup: 1000`, `fsGroup: 1000`) to the generated pods. Use this only if you need to run containers as root or a different specific user and understand the security implications. | false |  |
| `clusterQueue` _string_ | ClusterQueue specifies the name of the Kueue `ClusterQueue` that the job should be submitted to for scheduling and resource management.<br /><br />This value is set as the `kueue.x-k8s.io/queue-name` label on the underlying Kubernetes Job or RayJob.<br /><br />If omitted, it defaults to the value specified by the `DEFAULT_CLUSTER_QUEUE_NAME` environment variable in the Kaiwo controller (typically "kaiwo"), which is set during installation.<br /><br />Note! If the applied KaiwoQueueConfig includes no quota for the default queue, no workload will run that tries to fall back on it.<br /><br />The `kaiwo submit` CLI command can override this using the `--queue` flag or the `clusterQueue` field in the `kaiwoconfig.yaml` file. |  |  |
| `priorityClass` _string_ | PriorityClass specifies the name of a Kubernetes `PriorityClass` to be assigned to the job's pods. This influences the scheduling priority relative to other pods in the cluster. |  |  |
| `entrypoint` _string_ | EntryPoint defines the command or script that the primary container in the job's pod(s) should execute.<br /><br />It can be a multi-line string. Shell script shebangs (`#!/bin/bash`) are detected.<br /><br />For standard Kubernetes Jobs (`ray: false`), this populates the `command` and `args` fields of the container spec (typically `["/bin/sh", "-c", "<entrypoint_script>"]`).<br /><br />For RayJobs (`ray: true`), this populates the `rayJob.spec.entrypoint` field. For RayJobs, this must reference a Python script.<br /><br />This overrides any default command specified in the container image or the underlying `job` or `rayJob` spec sections if they are also defined. |  |  |
| `rayJob` _[RayJob](#rayjob)_ | RayJob defines the RayJob configuration.<br /><br />If this field is present (or if `spec.ray` is `true`), Kaiwo will create a `RayJob` resource instead of a standard `batchv1.Job`.<br /><br />Common fields like `image`, `resources`, `gpus`, `replicas`, etc., will be merged into this spec, potentially overriding values defined here unless explicitly configured otherwise.<br /><br />This provides fine-grained control over the Ray cluster configuration (head/worker groups) and Ray job submission parameters. |  |  |
| `job` _[Job](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#job-v1-batch)_ | Job defines the Kubernetes Job configuration.<br /><br />If this field is present and `spec.ray` is `false`, Kaiwo will use this as the base for the created `batchv1.Job`.<br /><br />Common fields like `image`, `resources`, `gpus`, `entrypoint`, etc., will be merged into this spec, potentially overriding values defined here.<br /><br />This provides fine-grained control over standard Kubernetes Job parameters like `backoffLimit`, `ttlSecondsAfterFinished`, pod template details, etc. |  |  |


#### KaiwoJobStatus



KaiwoJobStatus defines the observed state of KaiwoJob.



_Appears in:_
- [KaiwoJob](#kaiwojob)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | StartTime records the timestamp when the first pod associated with the KaiwoJob started running. |  |  |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | CompletionTime records the timestamp when the KaiwoJob finished execution (either successfully or with failure). |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions lists the observed conditions of the KaiwoJob resource, following standard Kubernetes conventions. |  |  |
| `status` _[Status](#status)_ | Status reflects the current high-level phase of the KaiwoJob lifecycle (e.g., PENDING, RUNNING, COMPLETE, FAILED). |  |  |
| `duration` _integer_ | Duration indicates the total time the job ran, calculated from StartTime to CompletionTime, in seconds. |  |  |
| `observedGeneration` _integer_ | ObservedGeneration records the `.metadata.generation` of the KaiwoJob resource that was last processed by the controller. |  |  |


#### KaiwoQueueConfig



KaiwoQueueConfig manages Kueue resources like ClusterQueues, ResourceFlavors, and WorkloadPriorityClasses based on its spec. It acts as a central configuration point for Kaiwo's integration with Kueue. Typically, only one cluster-scoped resource named 'kaiwo' should exist. The controller ensures that the specified Kueue resources are created, updated, or deleted to match the desired state defined here.
KaiwoQueueConfig manages Kueue resources.



_Appears in:_
- [KaiwoQueueConfigList](#kaiwoqueueconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoQueueConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoQueueConfigSpec](#kaiwoqueueconfigspec)_ | Spec defines the desired state for Kueue resources managed by Kaiwo. |  |  |
| `status` _[KaiwoQueueConfigStatus](#kaiwoqueueconfigstatus)_ | Status reflects the most recently observed state of the Kueue resource synchronization. |  |  |


#### KaiwoQueueConfigList



KaiwoQueueConfigList contains a list of KaiwoQueueConfig resources.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoQueueConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[KaiwoQueueConfig](#kaiwoqueueconfig) array_ |  |  |  |


#### KaiwoQueueConfigSpec



KaiwoQueueConfigSpec defines the desired configuration for Kaiwo's management of Kueue resources.
There should typically be only one KaiwoQueueConfig resource in the cluster, named 'kaiwo'.



_Appears in:_
- [KaiwoQueueConfig](#kaiwoqueueconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterQueues` _[ClusterQueue](#clusterqueue) array_ | ClusterQueues defines a list of Kueue ClusterQueues that Kaiwo should manage. Kaiwo ensures these ClusterQueues exist and match the provided specs. |  | MaxItems: 10 <br /> |
| `resourceFlavors` _[ResourceFlavorSpec](#resourceflavorspec) array_ | ResourceFlavors defines a list of Kueue ResourceFlavors that Kaiwo should manage. Kaiwo ensures these ResourceFlavors exist and match the provided specs. If omitted or empty, Kaiwo attempts to automatically discover node pools and create default flavors based on node labels. |  |  |
| `workloadPriorityClasses` _WorkloadPriorityClass array_ | WorkloadPriorityClasses defines a list of Kueue WorkloadPriorityClasses that Kaiwo should manage. Kaiwo ensures these priority classes exist with the specified values. See Kueue documentation for `WorkloadPriorityClass`. |  | MaxItems: 5 <br /> |


#### KaiwoQueueConfigStatus



KaiwoQueueConfigStatus represents the observed state of KaiwoQueueConfig.



_Appears in:_
- [KaiwoQueueConfig](#kaiwoqueueconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions lists the observed conditions of the KaiwoQueueConfig resource, such as whether the managed Kueue resources are synchronized and ready. |  |  |
| `status` _[Status](#status)_ | Status reflects the overall status of the Kueue resource synchronization managed by this config (e.g., PENDING, READY, FAILED). |  |  |


#### KaiwoService



KaiwoService represents a long-running service workload managed by Kaiwo. It encapsulates either a standard Kubernetes Deployment  or a RayService (via an AppWrapper), along with common metadata, storage configurations, and scheduling preferences. The Kaiwo controller reconciles this resource to create and manage the underlying workload objects.



_Appears in:_
- [KaiwoServiceList](#kaiwoservicelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `kaiwo.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `KaiwoService` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KaiwoServiceSpec](#kaiwoservicespec)_ | Spec defines the desired state of the KaiwoService, including workload type (Deployment/RayService), configuration, resources, and common metadata. |  |  |
| `status` _[KaiwoServiceStatus](#kaiwoservicestatus)_ | Status reflects the most recently observed state of the KaiwoService, including its phase, start time, duration, and conditions. |  |  |


#### KaiwoServiceList



KaiwoServiceList





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
| `user` _string_ | User specifies the owner or creator of the workload. It should typically be the user's email address. This value is primarily used for labeling (`kaiwo.silogen.ai/user`) the generated resources (like Pods, Jobs, Deployments) for identification and filtering (e.g., with `kaiwo list --user <email>`).<br /><br />In the future, if authentication is enabled, this must be the email address which is checked against authenticated user for match. |  |  |
| `podTemplateSpecLabels` _object (keys:string, values:string)_ | PodTemplateSpecLabels allows you to specify custom labels that will be added to the `template.metadata.labels` section of the generated Pods (within Jobs, Deployments, or RayCluster specs). Standard Kaiwo system labels (like `kaiwo.silogen.ai/user`, `kaiwo.silogen.ai/name`, etc.) are added automatically and take precedence if there are conflicts. |  |  |
| `gpus` _integer_ | Gpus specifies the total number of GPUs allocated to the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | 0 |  |
| `gpuVendor` _string_ | GpuVendor specifies the GPU vendor (e.g., amd, nvidia, etc.). See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | amd |  |
| `version` _string_ | Version allows you to specify an optional version string for the workload. This can be useful for tracking different iterations or configurations of the same logical workload. It does not directly affect resource creation but serves as metadata. |  |  |
| `replicas` _integer_ | Replicas specifies the number of replicas for the workload. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling. | 1 |  |
| `gpusPerReplica` _integer_ | GpusPerReplica specifies the number of GPUs allocated per replica. See [here](/scientist/scheduling#replicas-gpus-gpusperreplica-and-gpuvendor) for more details on how this field impacts scheduling.<br /><br />If you specify `gpusPerReplica`, you must also specify `replicas`. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources specify the default resource requirements applied for all pods inside the workflow.<br /><br />This field defines default Kubernetes `ResourceRequirements` (requests and limits for CPU,<br />memory, ephemeral-storage) applied to *all* containers (including init containers) within<br />the workload's pods.<br /><br />**Behavior:**<br /><br />These values act as **defaults**. If a container within the underlying Job, Deployment,<br />or Ray spec (if provided by the user) already defines a specific request or limit<br />(e.g., `memory` limit), the value from `resources` for that specific metric **will not** override it.<br /><br />**Interaction with GPU fields:** The GPU requests/limits (`amd.com/gpu` or `nvidia.com/gpu`)<br />are controlled exclusively by the `gpus`, `gpusPerReplica`, and `gpuVendor` fields<br />(and the associated calculation logic described above). Any GPU specifications within<br />the `resources` field are **ignored**.<br /><br />**Default CPU/Memory with GPUs:** When Kaiwo *generates* the underlying<br />Job/Deployment/RayCluster spec (i.e., the user did *not* provide `spec.job`,<br />`spec.deployment`, or `spec.rayService`/`spec.rayJob`), and GPUs are requested<br />(`gpusPerReplica` > 0), Kaiwo applies default CPU and Memory requests/limits<br />based on the GPU count (e.g., 4 CPU cores and 32Gi Memory per GPU).<br />These GPU-derived defaults *will* override any CPU/Memory settings defined in<br />the `resources` field in this specific scenario. If the user *does* provide<br />the underlying spec, these GPU-derived CPU/Memory defaults are not applied,<br />respecting the user's definition or the values from the `resources` field. |  |  |
| `image` _string_ | Image specifies the default container image to be used for the primary workload container(s).<br /><br />- If containers defined within the underlying Job, Deployment, or Ray spec do *not* specify an image, this image will be used.<br />- If this field is also empty, the latest tag of ghcr.io/silogen/rocm-ray is used |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets is a list of Kubernetes `LocalObjectReference` (containing just the secret `name`) referencing secrets needed to pull the container image(s). These are added to the `imagePullSecrets` field of the PodSpec for all generated pods. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env is a list of Kubernetes `EnvVar` structs. These environment variables are added to the primary workload container(s) in the generated pods. They are appended to any environment variables already defined in the underlying Job, Deployment, or Ray spec. |  |  |
| `secretVolumes` _[SecretVolume](#secretvolume) array_ | SecretVolumes allows you to mount specific keys from Kubernetes Secrets as files into the workload containers. |  |  |
| `ray` _boolean_ | Ray determines whether the operator should use RayCluster for workload execution.<br />If `true`, Kaiwo will create Ray-specific resources.<br />If `false` (default), Kaiwo will create standard Kubernetes resources (BatchJob for `KaiwoJob`, Deployment for `KaiwoService`).<br />This setting dictates which underlying spec (`job`/`rayJob` or `deployment`/`rayService`) is primarily used. | false |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage configures persistent storage using Kubernetes PersistentVolumeClaims (PVCs).<br /><br />Enabling `storage.data.download` or `storage.huggingFace.preCacheRepos` will cause Kaiwo to create a temporary Kubernetes Job (the "download job") before starting the main workload. This job runs a container that performs the downloads into the respective PVCs. The main workload only starts after the download job completes successfully. |  |  |
| `dangerous` _boolean_ | Dangerous, if when set to `true`, Kaiwo will *not* add the default `PodSecurityContext` (which normally sets `runAsUser: 1000`, `runAsGroup: 1000`, `fsGroup: 1000`) to the generated pods. Use this only if you need to run containers as root or a different specific user and understand the security implications. | false |  |
| `clusterQueue` _string_ | ClusterQueue specifies the name of the Kueue `ClusterQueue` that the service should be submitted to for scheduling and resource management.<br /><br />This value is set as the `kueue.x-k8s.io/queue-name` label on the underlying Kubernetes Deployment or RayService.<br /><br />If omitted, it defaults to the value specified by the `DEFAULT_CLUSTER_QUEUE_NAME` environment variable in the Kaiwo controller (typically "kaiwo").<br /><br />The `kaiwo submit` CLI command can override this using the `--queue` flag or the `clusterQueue` field in the `kaiwoconfig.yaml` file. |  |  |
| `priorityClass` _string_ | PriorityClass specifies the name of a Kubernetes `PriorityClass` to be assigned to the service's pods. This influences the scheduling priority relative to other pods in the cluster. |  |  |
| `entrypoint` _string_ | EntryPoint specifies the command or script executed in a Deployment.<br />Can also be defined inside Deployment struct as regular command in the form of string array.<br /><br />It is *not* used when `ray: true` (use `serveConfigV2` or the `rayService` spec instead for Ray entrypoints). |  |  |
| `serveConfigV2` _string_ | Defines the applications and deployments to deploy, should be a YAML multi-line scalar string.<br />Can also be defined inside RayService struct |  |  |
| `rayService` _[RayService](#rayservice)_ | RayService allows providing a full `rayv1.RayService` spec.<br /><br />If present (or `spec.ray` is `true`), Kaiwo creates a `RayService` (wrapped in an AppWrapper for Kueue integration) instead of a `Deployment`.<br /><br />Common fields are merged into the `RayClusterSpec` within this spec.<br /><br />Allows fine-grained control over the Ray cluster and Ray Serve configurations. |  |  |
| `deployment` _[Deployment](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#deployment-v1-apps)_ | Deployment allows providing a full `appsv1.Deployment` spec.<br /><br />If present and `spec.ray` is `false`, this is used as the base for the created `Deployment`.<br /><br />Common fields are merged into this spec.<br /><br />Allows fine-grained control over Kubernetes Deployment parameters (strategy, selectors, pod template, etc.). |  |  |


#### KaiwoServiceStatus



KaiwoServiceStatus defines the observed state of KaiwoService.



_Appears in:_
- [KaiwoService](#kaiwoservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | StartTime records the timestamp when the first pod associated with the KaiwoService started running. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions lists the observed conditions of the KaiwoService resource, following standard Kubernetes conventions. May include conditions reflecting the underlying Deployment or RayService state. |  |  |
| `status` _[Status](#status)_ | Status reflects the current high-level phase of the KaiwoService lifecycle (e.g., PENDING, STARTING, READY, FAILED). |  |  |
| `duration` _integer_ | Duration indicates how long the service has been running since StartTime, in seconds. Calculated periodically while running. |  |  |
| `observedGeneration` _integer_ | ObservedGeneration records the `.metadata.generation` of the KaiwoService resource that was last processed by the controller. |  |  |


#### ObjectStorageDownloadSpec



ObjectStorageDownloadSpec aggregates download tasks for various object storage and Git sources within the `DataStorageSpec`.



_Appears in:_
- [DataStorageSpec](#datastoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `s3` _[S3DownloadItem](#s3downloaditem) array_ | S3 lists any S3 downloads |  |  |
| `gcs` _[GCSDownloadItem](#gcsdownloaditem) array_ | GCS lists and Google Cloud Storage downloads |  |  |
| `azureBlob` _[AzureBlobStorageDownloadItem](#azureblobstoragedownloaditem) array_ | AzureBlob lists any Azure Blob Storage downloads |  |  |
| `git` _[GitDownloadItem](#gitdownloaditem) array_ | Git lists any Git downloads |  |  |


#### ResourceFlavorSpec



ResourceFlavorSpec defines the configuration for a Kueue ResourceFlavor managed by Kaiwo.



_Appears in:_
- [KaiwoQueueConfigSpec](#kaiwoqueueconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name specifies the name of the Kueue ResourceFlavor resource (e.g., "amd-mi300-8gpu"). |  |  |
| `nodeLabels` _object (keys:string, values:string)_ | NodeLabels specifies the labels that pods requesting this flavor must match on nodes. This is used by Kueue for scheduling decisions. Keys and values should correspond to actual node labels. Example: `\{"kaiwo/nodepool": "amd-gpu-nodes"\}` |  | MaxProperties: 10 <br /> |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#taint-v1-core) array_ | Taints specifies a list of taints associated with this flavor. |  | MaxItems: 5 <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#toleration-v1-core) array_ | Tolerations specifies a list of tolerations associated with this flavor. This is less common than using Taints; Kueue primarily uses Taints to derive Tolerations. |  | MaxItems: 5 <br /> |


#### S3DownloadItem



S3DownloadItem defines parameters for downloading data from an S3-compatible object store.



_Appears in:_
- [DownloadTaskConfig](#downloadtaskconfig)
- [ObjectStorageDownloadSpec](#objectstoragedownloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpointUrl` _string_ | EndpointUrl specifies the S3 API endpoint URL (e.g., "https://s3.us-east-1.amazonaws.com" or a MinIO endpoint). |  |  |
| `accessKeyId` _[ValueReference](#valuereference)_ | AccessKeyId optionally references a Kubernetes Secret containing the S3 access key ID. See `ValueReference`. |  |  |
| `secretKey` _[ValueReference](#valuereference)_ | SecretKey optionally references a Kubernetes Secret containing the S3 secret access key. See `ValueReference`. |  |  |
| `buckets` _[CloudDownloadBucket](#clouddownloadbucket) array_ | Buckets lists the S3 buckets and the specific files/folders to download from them. See `CloudDownloadBucket`. |  |  |


#### SecretVolume



SecretVolume defines how to mount a specific key from a Kubernetes Secret into the workload's containers.



_Appears in:_
- [CommonMetaSpec](#commonmetaspec)
- [KaiwoJobSpec](#kaiwojobspec)
- [KaiwoServiceSpec](#kaiwoservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name defines the name of the Kubernetes Volume that will be created. Should be unique within the pod. |  |  |
| `secretName` _string_ | SecretName specifies the name of the Kubernetes Secret resource to mount from. |  |  |
| `key` _string_ | Key specifies the key within the Secret whose value should be mounted. If omitted, the entire secret might be mounted as files (depending on Kubernetes behavior). |  |  |
| `subPath` _string_ | SubPath defines the filename within the `MountPath` directory where the secret `Key`'s content will be placed. Useful for mounting a single secret key as a file. |  |  |
| `mountPath` _string_ | MountPath defines the directory path inside the container where the secret volume (or the `SubPath` file) should be mounted. |  |  |


#### Status

_Underlying type:_ _string_





_Appears in:_
- [KaiwoJobStatus](#kaiwojobstatus)
- [KaiwoQueueConfigStatus](#kaiwoqueueconfigstatus)
- [KaiwoServiceStatus](#kaiwoservicestatus)

| Field | Description |
| --- | --- |
| `` | StatusNew indicates the resource has been created but not yet processed by the controller.<br /> |
| `PENDING` | StatusPending indicates the resource is waiting for prerequisites (like download jobs or Kueue admission) to complete.<br /> |
| `STARTING` | StatusStarting indicates the underlying workload (Job, Deployment, RayService) is being created or started.<br /> |
| `READY` | StatusReady indicates a KaiwoService is fully deployed and ready to serve requests (Deployment ready or RayService healthy). Not applicable to KaiwoJob.<br /> |
| `RUNNING` | StatusRunning indicates the workload pods are running. For KaiwoJob, this means the job has started execution. For KaiwoService, pods are up but may not yet be fully ready/healthy.<br /> |
| `COMPLETE` | StatusComplete indicates a KaiwoJob has finished successfully.<br /> |
| `FAILED` | StatusFailed indicates the workload (KaiwoJob or KaiwoService) or its prerequisites (like download jobs) encountered an error and cannot proceed or recover.<br /> |


#### StorageSpec



StorageSpec defines the storage configuration for the workload.



_Appears in:_
- [CommonMetaSpec](#commonmetaspec)
- [KaiwoJobSpec](#kaiwojobspec)
- [KaiwoServiceSpec](#kaiwoservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `storageEnabled` _boolean_ | StorageEnabled must be `true` to enable the creation of any PersistentVolumeClaims defined within this spec. If `false`, `data` and `huggingFace` sections are ignored. |  |  |
| `storageClassName` _string_ | StorageClassName specifies the name of the Kubernetes `StorageClass` to use when creating PersistentVolumeClaims for `data` and `huggingFace` volumes. Must refer to an existing StorageClass in the cluster. |  |  |
| `accessMode` _[PersistentVolumeAccessMode](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#persistentvolumeaccessmode-v1-core)_ | AccessMode determines the access mode (e.g., `ReadWriteOnce`, `ReadWriteMany`, `ReadOnlyMany`) for the created PersistentVolumeClaims.<br /><br />In a multi-node setting, ReadWriteMany is generally required, as pods scheduled on different nodes cannot access ReadWriteOnce PVCs. This is true even when `replicas: 1` if you are using download jobs, as the download pod may get scheduled on a different pod than the main workload pod. | ReadWriteMany |  |
| `data` _[DataStorageSpec](#datastoragespec)_ | Data configures the main data PersistentVolumeClaim and optional pre-download tasks for it. |  |  |
| `huggingFace` _[HfStorageSpec](#hfstoragespec)_ | HuggingFace configures a PersistentVolumeClaim specifically for caching Hugging Face models and datasets, with options for pre-caching. |  |  |


#### ValueReference



ValueReference provides a way to reference sensitive values stored in Kubernetes Secrets, typically used for credentials needed by download tasks.



_Appears in:_
- [AzureBlobStorageDownloadItem](#azureblobstoragedownloaditem)
- [GCSDownloadItem](#gcsdownloaditem)
- [GitDownloadItem](#gitdownloaditem)
- [S3DownloadItem](#s3downloaditem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `file` _string_ | File specifies the expected path within the download job's container where the secret value will be mounted as a file. This path is usually automatically generated by the controller based on SecretName and SecretKey. |  |  |
| `secretName` _string_ | SecretName is the name of the Kubernetes Secret resource containing the value. |  |  |
| `secretKey` _string_ | SecretKey is the key within the specified Secret whose value should be used. |  |  |


