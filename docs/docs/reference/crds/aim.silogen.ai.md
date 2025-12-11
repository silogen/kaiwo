# API Reference

## Packages
- [aim.silogen.ai/v1alpha1](#aimsilogenaiv1alpha1)


## aim.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the AIM v1alpha1 API group.

### Resource Types
- [AIMClusterModel](#aimclustermodel)
- [AIMClusterModelList](#aimclustermodellist)
- [AIMClusterModelSource](#aimclustermodelsource)
- [AIMClusterModelSourceList](#aimclustermodelsourcelist)
- [AIMClusterRuntimeConfig](#aimclusterruntimeconfig)
- [AIMClusterRuntimeConfigList](#aimclusterruntimeconfiglist)
- [AIMClusterServiceTemplate](#aimclusterservicetemplate)
- [AIMClusterServiceTemplateList](#aimclusterservicetemplatelist)
- [AIMKVCache](#aimkvcache)
- [AIMKVCacheList](#aimkvcachelist)
- [AIMModel](#aimmodel)
- [AIMModelCache](#aimmodelcache)
- [AIMModelCacheList](#aimmodelcachelist)
- [AIMModelList](#aimmodellist)
- [AIMRuntimeConfig](#aimruntimeconfig)
- [AIMRuntimeConfigList](#aimruntimeconfiglist)
- [AIMService](#aimservice)
- [AIMServiceList](#aimservicelist)
- [AIMServiceTemplate](#aimservicetemplate)
- [AIMServiceTemplateList](#aimservicetemplatelist)
- [AIMTemplateCache](#aimtemplatecache)
- [AIMTemplateCacheList](#aimtemplatecachelist)



#### AIMClusterModel



AIMClusterModel is the Schema for cluster-scoped AIM model catalog entries.



_Appears in:_
- [AIMClusterModelList](#aimclustermodellist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterModel` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMModelSpec](#aimmodelspec)_ |  |  |  |
| `status` _[AIMModelStatus](#aimmodelstatus)_ |  |  |  |


#### AIMClusterModelList



AIMClusterModelList contains a list of AIMClusterModel.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterModelList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMClusterModel](#aimclustermodel) array_ |  |  |  |


#### AIMClusterModelSource



AIMClusterModelSource automatically discovers and syncs AI model images from container registries.



_Appears in:_
- [AIMClusterModelSourceList](#aimclustermodelsourcelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterModelSource` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMClusterModelSourceSpec](#aimclustermodelsourcespec)_ |  |  |  |
| `status` _[AIMClusterModelSourceStatus](#aimclustermodelsourcestatus)_ |  |  |  |


#### AIMClusterModelSourceList



AIMClusterModelSourceList contains a list of AIMClusterModelSource.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterModelSourceList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMClusterModelSource](#aimclustermodelsource) array_ |  |  |  |


#### AIMClusterModelSourceSpec



AIMClusterModelSourceSpec defines the desired state of AIMClusterModelSource.



_Appears in:_
- [AIMClusterModelSource](#aimclustermodelsource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `registry` _string_ | Registry to sync from (e.g., docker.io, ghcr.io, gcr.io).<br />Defaults to docker.io if not specified. | docker.io |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets contains references to secrets for authenticating to private registries.<br />Secrets must exist in the operator namespace (typically kaiwo-system).<br />Used for both registry catalog listing and image metadata extraction. |  |  |
| `filters` _[ModelSourceFilter](#modelsourcefilter) array_ | Filters define which images to discover and sync.<br />Each filter specifies an image pattern with optional version constraints and exclusions.<br />Multiple filters are combined with OR logic (any match includes the image). |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `syncInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#duration-v1-meta)_ | SyncInterval defines how often to sync with the registry.<br />Defaults to 1h. Minimum recommended interval is 15m to avoid rate limiting.<br />Format: duration string (e.g., "30m", "1h", "2h30m"). | 1h |  |
| `versions` _string array_ | Versions specifies global semantic version constraints applied to all filters.<br />Individual filters can override this with their own version constraints.<br />Constraints use semver syntax: >=1.0.0, <2.0.0, ~1.2.0, ^1.0.0, etc.<br />Non-semver tags (e.g., "latest", "dev") are silently skipped.<br />Version ranges work on all registries (including ghcr.io, gcr.io) when combined with<br />exact repository names (no wildcards). The controller uses the Tags List API to fetch<br />all tags for the repository and filters them by the semver constraint.<br />Example: registry=ghcr.io, filters=[\{image: "silogen/aim-llama"\}], versions=[">=1.0.0"]<br />will fetch all tags from ghcr.io/silogen/aim-llama and include only those >=1.0.0. |  |  |
| `maxModels` _integer_ | MaxModels is the maximum number of AIMClusterModel resources to create from this source.<br />Once this limit is reached, no new models will be created, even if more matching images are discovered.<br />Existing models are never deleted.<br />This prevents runaway model creation from overly broad filters. | 100 | Maximum: 10000 <br />Minimum: 1 <br /> |


#### AIMClusterModelSourceStatus



AIMClusterModelSourceStatus defines the observed state of AIMClusterModelSource.



_Appears in:_
- [AIMClusterModelSource](#aimclustermodelsource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `status` _string_ | Status represents the overall state of the model source. |  | Enum: [Pending Starting Progressing Ready Running Degraded NotAvailable Failed] <br /> |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | LastSyncTime is the timestamp of the last successful registry sync.<br />Updated after each successful sync operation. |  |  |
| `discoveredModels` _integer_ | DiscoveredModels is the count of AIMClusterModel resources managed by this source.<br />Includes both existing and newly created models. |  |  |
| `availableModels` _integer_ | AvailableModels is the total count of images discovered in the registry that match the filters.<br />This may be higher than DiscoveredModels if maxModels limit was reached. |  |  |
| `modelsLimitReached` _boolean_ | ModelsLimitReached indicates whether the maxModels limit has been reached.<br />When true, no new models will be created even if more matching images are discovered. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the source's state.<br />Standard conditions: Ready, Syncing, RegistryReachable. |  |  |
| `observedGeneration` _integer_ | ObservedGeneration reflects the generation of the most recently observed spec. |  |  |


#### AIMClusterRuntimeConfig



AIMClusterRuntimeConfig defines cluster-scoped runtime defaults for AIM resources.



_Appears in:_
- [AIMClusterRuntimeConfigList](#aimclusterruntimeconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterRuntimeConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMClusterRuntimeConfigSpec](#aimclusterruntimeconfigspec)_ |  |  |  |
| `status` _[AIMRuntimeConfigStatus](#aimruntimeconfigstatus)_ |  |  |  |


#### AIMClusterRuntimeConfigList



AIMClusterRuntimeConfigList contains a list of AIMClusterRuntimeConfig.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterRuntimeConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMClusterRuntimeConfig](#aimclusterruntimeconfig) array_ |  |  |  |


#### AIMClusterRuntimeConfigSpec



AIMClusterRuntimeConfigSpec defines cluster-wide defaults for AIM resources.



_Appears in:_
- [AIMClusterRuntimeConfig](#aimclusterruntimeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultStorageClassName` _string_ | DefaultStorageClassName specifies the storage class to use for model caches and PVCs<br />when the consuming resource (AIMModelCache, AIMTemplateCache, AIMServiceTemplate) does not<br />specify a storage class. If this field is empty, the cluster's default storage class is used. |  |  |
| `model` _[AIMModelConfig](#aimmodelconfig)_ | Model controls model creation and discovery defaults. |  |  |
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |
| `pvcHeadroomPercent` _integer_ | PVCHeadroomPercent specifies the percentage of extra space to add to PVCs<br />for model storage. This accounts for filesystem overhead and temporary files<br />during model loading. The value represents a percentage (e.g., 10 means 10% extra space).<br />If not specified, defaults to 10%. |  | Minimum: 0 <br /> |
| `labelPropagation` _[AIMRuntimeConfigLabelPropagationSpec](#aimruntimeconfiglabelpropagationspec)_ | LabelPropagation controls how labels from parent AIM resources are propagated to child resources.<br />When enabled, labels matching the specified patterns are automatically copied from parent resources<br />(e.g., AIMService, AIMTemplateCache) to their child resources (e.g., Deployments, Services, PVCs).<br />This is useful for propagating organizational metadata like cost centers, team identifiers,<br />or compliance labels through the resource hierarchy. |  |  |


#### AIMClusterServiceTemplate







_Appears in:_
- [AIMClusterServiceTemplateList](#aimclusterservicetemplatelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterServiceTemplate` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)_ |  |  |  |
| `status` _[AIMServiceTemplateStatus](#aimservicetemplatestatus)_ |  |  |  |


#### AIMClusterServiceTemplateList









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterServiceTemplateList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMClusterServiceTemplate](#aimclusterservicetemplate) array_ |  |  |  |


#### AIMClusterServiceTemplateSpec



AIMClusterServiceTemplateSpec defines the desired state of AIMClusterServiceTemplate (cluster-scoped).

A cluster-scoped template that selects a runtime profile for a given AIM model.



_Appears in:_
- [AIMClusterServiceTemplate](#aimclusterservicetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `modelName` _string_ | ModelName is the model name. Matches `metadata.name` of an AIMModel or AIMClusterModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template. | default |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets lists secrets containing credentials for pulling container images.<br />These secrets are used for:<br />- Discovery dry-run jobs that inspect the model container<br />- Pulling the image for inference services<br />The secrets are merged with any model or runtime config defaults.<br />For namespace-scoped templates, secrets must exist in the same namespace.<br />For cluster-scoped templates, secrets must exist in the operator namespace. |  |  |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for workloads related to this template.<br />This includes discovery dry-run jobs and inference services created from this template.<br />If empty, the default service account for the namespace is used. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default container resource requirements applied to services derived from this template.<br />Service-specific values override the template defaults. |  |  |
| `modelSources` _[AIMModelSource](#aimmodelsource) array_ | ModelSources specifies the model artifacts required to run this template.<br />When provided, the discovery dry-run will be skipped and these sources will be used directly.<br />This allows users to explicitly declare model dependencies without requiring a discovery job.<br />If omitted, a discovery job will be run to automatically determine the required model sources. |  |  |
| `profileId` _string_ | ProfileId is the specific AIM profile ID that this template should use |  |  |




#### AIMDiscoveryProfileMetadata



AIMDiscoveryProfileMetadata describes the characteristics of a discovered deployment profile.



_Appears in:_
- [AIMDiscoveryProfile](#aimdiscoveryprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `engine` _string_ | Engine identifies the inference engine used for this profile (e.g., "vllm", "tgi"). |  |  |
| `gpu` _string_ | GPU specifies the GPU model this profile is optimized for (e.g., "MI300X", "MI325X"). |  |  |
| `gpu_count` _integer_ | GPUCount indicates how many GPUs are required per replica for this profile. |  |  |
| `metric` _[AIMMetric](#aimmetric)_ | Metric indicates the optimization goal for this profile ("latency" or "throughput"). |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision specifies the numeric precision used in this profile (e.g., "fp16", "fp8"). |  | Enum: [bf16 fp16 fp8 int8] <br /> |
| `type` _[AIMProfileType](#aimprofiletype)_ | Type specifies the optimization level of this profile |  |  |


#### AIMGpuSelector



AIMGpuSelector specifies GPU requirements for a deployment.
It defines the number and type of GPUs needed for each replica.



_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMRuntimeParameters](#aimruntimeparameters)
- [AIMServiceOverrides](#aimserviceoverrides)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `count` _integer_ | Count is the number of GPU resources requested per replica.<br />Must be at least 1. |  | Minimum: 1 <br /> |
| `model` _string_ | Model is the GPU model name required for this deployment.<br />Examples: "MI300X", "MI325X" |  | MinLength: 1 <br /> |
| `resourceName` _string_ | ResourceName is the Kubernetes resource name for GPU resources.<br />Defaults to "amd.com/gpu" if not specified. | amd.com/gpu |  |


#### AIMKVCache



AIMKVCache is the Schema for the KV caches API



_Appears in:_
- [AIMKVCacheList](#aimkvcachelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMKVCache` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMKVCacheSpec](#aimkvcachespec)_ |  |  |  |
| `status` _[AIMKVCacheStatus](#aimkvcachestatus)_ |  |  |  |


#### AIMKVCacheList



AIMKVCacheList contains a list of AIMKVCache





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMKVCacheList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMKVCache](#aimkvcache) array_ |  |  |  |


#### AIMKVCacheSpec



AIMKVCacheSpec defines the desired state of AIMKVCache



_Appears in:_
- [AIMKVCache](#aimkvcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kvCacheType` _string_ | KVCacheType specifies the type of key-value cache to create | redis | Enum: [redis] <br /> |
| `image` _string_ | Image specifies the container image to use for the KV cache service.<br />If not specified, defaults to appropriate images based on KVCacheType:<br />- redis: redis:7.2.4 |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to set in the KV cache container.<br />If not specified (nil), no additional environment variables are set.<br />If explicitly set to an empty array, no environment variables are added. |  |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage defines the persistent storage configuration for the KV cache |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the KV cache container.<br />If not specified, defaults to 1 CPU and 1Gi memory for both requests and limits. |  |  |


#### AIMKVCacheStatus



AIMKVCacheStatus defines the observed state of AIMKVCache



_Appears in:_
- [AIMKVCache](#aimkvcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the KV cache's state |  |  |
| `status` _[AIMKVCacheStatusEnum](#aimkvcachestatusenum)_ | Status represents the current status of the KV cache | Pending | Enum: [Pending Progressing Ready Failed] <br /> |
| `statefulSetName` _string_ | StatefulSetName represents the name of the created statefulset |  |  |
| `serviceName` _string_ | ServiceName represents the name of the created service |  |  |
| `endpoint` _string_ | Endpoint provides the connection information for accessing the KV cache.<br />Format depends on the backend type (e.g., "redis://service-name:6379" for Redis). |  |  |
| `replicas` _integer_ | Replicas is the total number of replicas configured for the StatefulSet. |  |  |
| `readyReplicas` _integer_ | ReadyReplicas is the number of pods that are ready and serving traffic. |  |  |
| `storageSize` _string_ | StorageSize represents the total storage capacity allocated for the KV cache.<br />This reflects the size specified in the PersistentVolumeClaim. |  |  |
| `lastError` _string_ | LastError contains details about the most recent error encountered.<br />This field is cleared when the error is resolved. |  |  |


#### AIMKVCacheStatusEnum

_Underlying type:_ _string_



_Validation:_
- Enum: [Pending Progressing Ready Failed]

_Appears in:_
- [AIMKVCacheStatus](#aimkvcachestatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMKVCacheStatusPending denotes that the KV cache is being created<br /> |
| `Progressing` | AIMKVCacheStatusProgressing denotes that the KV cache is being deployed<br /> |
| `Ready` | AIMKVCacheStatusReady denotes that the KV cache is ready to be used<br /> |
| `Failed` | AIMKVCacheStatusFailed denotes that the KV cache deployment has failed<br /> |


#### AIMMetric

_Underlying type:_ _string_

AIMMetric enumerates the targeted service characteristic

_Validation:_
- Enum: [latency throughput]

_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMDiscoveryProfileMetadata](#aimdiscoveryprofilemetadata)
- [AIMProfileMetadata](#aimprofilemetadata)
- [AIMRuntimeParameters](#aimruntimeparameters)
- [AIMServiceOverrides](#aimserviceoverrides)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)

| Field | Description |
| --- | --- |
| `latency` |  |
| `throughput` |  |


#### AIMModel



AIMModel is the Schema for namespace-scoped AIM model catalog entries.



_Appears in:_
- [AIMModelList](#aimmodellist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMModel` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMModelSpec](#aimmodelspec)_ |  |  |  |
| `status` _[AIMModelStatus](#aimmodelstatus)_ |  |  |  |


#### AIMModelCache



AIMModelCache is the Schema for the modelcaches API



_Appears in:_
- [AIMModelCacheList](#aimmodelcachelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMModelCache` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMModelCacheSpec](#aimmodelcachespec)_ |  |  |  |
| `status` _[AIMModelCacheStatus](#aimmodelcachestatus)_ |  |  |  |


#### AIMModelCacheList



AIMModelCacheList contains a list of AIMModelCache





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMModelCacheList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMModelCache](#aimmodelcache) array_ |  |  |  |


#### AIMModelCacheSpec



AIMModelCacheSpec defines the desired state of AIMModelCache



_Appears in:_
- [AIMModelCache](#aimmodelcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceUri` _string_ | SourceURI is the source of the model to be downloaded. This is the only<br />identifier |  | MinLength: 1 <br />Pattern: `^(hf\|s3)://[^ \t\r\n]+$` <br /> |
| `storageClassName` _string_ | StorageClassName specifies the storage class for the cache volume |  |  |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | Size specifies the size of the cache volume |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env lists the environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `modelDownloadImage` _string_ | ModelDownloadImage is the image used to download the model | kserve/storage-initializer:v0.16.0 |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this model cache.<br />This determines PVC headroom and other runtime settings. | default |  |


#### AIMModelCacheStatus



AIMModelCacheStatus defines the observed state of AIMModelCache



_Appears in:_
- [AIMModelCache](#aimmodelcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the model cache's state |  |  |
| `status` _[AIMModelCacheStatusEnum](#aimmodelcachestatusenum)_ | Status represents the current status of the model cache | Pending | Enum: [Pending Progressing Available Failed] <br /> |
| `lastUsed` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | LastUsed represents the last time a model was deployed that used this cache |  |  |
| `persistentVolumeClaim` _string_ | PersistentVolumeClaim represents the name of the created PVC |  |  |


#### AIMModelCacheStatusEnum

_Underlying type:_ _string_



_Validation:_
- Enum: [Pending Progressing Available Failed]

_Appears in:_
- [AIMModelCacheStatus](#aimmodelcachestatus)
- [AIMResolvedModelCache](#aimresolvedmodelcache)

| Field | Description |
| --- | --- |
| `Pending` | AIMModelCacheStatusPending denotes that the model cache has not been created yet<br /> |
| `Progressing` | AIMModelCacheStatusProgressing denotes that the model cache is currently being filled<br /> |
| `Available` | AIMModelCacheStatusAvailable denotes that a model cache is filled and ready to be used<br /> |
| `Failed` | AIMModelCacheStatusFailed denotes that the model cache has failed. A more detailed reason will be available in the conditions.<br /> |


#### AIMModelConfig



AIMModelConfig controls model creation and discovery behavior.



_Appears in:_
- [AIMClusterRuntimeConfigSpec](#aimclusterruntimeconfigspec)
- [AIMRuntimeConfigCommon](#aimruntimeconfigcommon)
- [AIMRuntimeConfigSpec](#aimruntimeconfigspec)
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `autoDiscovery` _boolean_ | AutoDiscovery controls whether models run discovery by default.<br />When true, models run discovery jobs to extract metadata and auto-create templates.<br />When false, discovery is skipped. Discovery failures are non-fatal and reported via conditions. | true |  |


#### AIMModelDiscoveryConfig



AIMModelDiscoveryConfig controls discovery behavior for a model.



_Appears in:_
- [AIMModelSpec](#aimmodelspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether discovery runs for this model.<br />When unset (nil), uses the runtime config's model.autoDiscovery setting.<br />When true, discovery always runs regardless of runtime config.<br />When false, discovery never runs regardless of runtime config. |  |  |
| `autoCreateTemplates` _boolean_ | AutoCreateTemplates controls whether templates are auto-created from discovery results.<br />When unset, templates are created if discovery succeeds and returns recommended deployments.<br />When false, discovery runs but templates are not created (metadata extraction only).<br />When true, templates are always created from discovery results. |  |  |


#### AIMModelList



AIMModelList contains a list of AIMModel.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMModelList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMModel](#aimmodel) array_ |  |  |  |


#### AIMModelSource



AIMModelSource describes a model artifact that must be downloaded for inference.
Discovery extracts these from the container's configuration to enable caching and validation.



_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)
- [AIMTemplateCacheSpec](#aimtemplatecachespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is a human-readable identifier for this model artifact.<br />May be empty if the source represents the primary model. |  |  |
| `sourceUri` _string_ | SourceURI is the location from which the model should be downloaded.<br />Supported schemes:<br />- hf://org/model - Hugging Face Hub model<br />- s3://bucket/key - S3-compatible storage |  | Pattern: `^(hf\|s3)://[^ \t\r\n]+$` <br /> |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | Size is the expected storage space required for this model artifact.<br />Used for PVC sizing and capacity planning during cache creation. |  |  |


#### AIMModelSpec



AIMModelSpec defines the desired state of AIMModel.



_Appears in:_
- [AIMClusterModel](#aimclustermodel)
- [AIMModel](#aimmodel)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Image is the container image URI for this AIM model.<br />This image is inspected by the operator to select runtime profiles used by templates.<br />Discovery behavior is controlled by the discovery field and runtime config's AutoDiscovery setting. |  | MinLength: 1 <br /> |
| `discovery` _[AIMModelDiscoveryConfig](#aimmodeldiscoveryconfig)_ | Discovery controls discovery behavior for this model.<br />When unset, uses runtime config defaults. |  |  |
| `defaultServiceTemplate` _string_ | DefaultServiceTemplate is the default template to use for this image, if the user does not provide any |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this image.<br />The runtime config controls discovery behavior and model creation scope. | default |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets lists secrets containing credentials for pulling the model container image.<br />These secrets are used for:<br />- OCI registry metadata extraction during discovery<br />- Pulling the image for inference services<br />The secrets are merged with any runtime config defaults.<br />For namespace-scoped models, secrets must exist in the same namespace.<br />For cluster-scoped models, secrets must exist in the operator namespace. |  |  |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for workloads related to this model.<br />This includes metadata extraction jobs and any other model-related operations.<br />If empty, the default service account for the namespace is used. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default resource requirements for services using this image.<br />Template- or service-level values override these defaults. |  |  |


#### AIMModelStatus



AIMModelStatus defines the observed state of AIMModel.



_Appears in:_
- [AIMClusterModel](#aimclustermodel)
- [AIMModel](#aimmodel)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller |  |  |
| `status` _[AIMModelStatusEnum](#aimmodelstatusenum)_ | Status represents the overall status of the image based on its templates | Pending | Enum: [Pending Progressing Ready NotAvailable Degraded Failed] <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the model's state |  |  |
| `resolvedRuntimeConfig` _[AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)_ | ResolvedRuntimeConfig captures metadata about the runtime config that was resolved. |  |  |
| `imageMetadata` _[ImageMetadata](#imagemetadata)_ | ImageMetadata is the metadata extracted from an AIM image |  |  |


#### AIMModelStatusEnum

_Underlying type:_ _string_

AIMModelStatusEnum represents the overall status of an AIMModel.

_Validation:_
- Enum: [Pending Progressing Ready NotAvailable Degraded Failed]

_Appears in:_
- [AIMModelStatus](#aimmodelstatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMModelStatusPending indicates the image has been created but template generation has not started.<br /> |
| `Progressing` | AIMModelStatusProgressing indicates one or more templates are still being discovered.<br /> |
| `Ready` | AIMModelStatusReady indicates all templates are available and ready.<br /> |
| `NotAvailable` | AIMModelStatusNotAvailable indicates all templates are not available (e.g., required GPUs not present in cluster).<br /> |
| `Degraded` | AIMModelStatusDegraded indicates one or more templates are degraded or failed.<br /> |
| `Failed` | AIMModelStatusFailed indicates all templates are degraded or failed.<br /> |


#### AIMPrecision

_Underlying type:_ _string_

AIMPrecision enumerates supported numeric precisions

_Validation:_
- Enum: [bf16 fp16 fp8 int8]

_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMDiscoveryProfileMetadata](#aimdiscoveryprofilemetadata)
- [AIMProfileMetadata](#aimprofilemetadata)
- [AIMRuntimeParameters](#aimruntimeparameters)
- [AIMServiceOverrides](#aimserviceoverrides)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)

| Field | Description |
| --- | --- |
| `auto` |  |
| `fp4` |  |
| `fp8` |  |
| `fp16` |  |
| `fp32` |  |
| `bf16` |  |
| `int4` |  |
| `int8` |  |


#### AIMProfile



AIMProfile contains the cached discovery results for a template.
This is the processed and validated version of AIMDiscoveryProfile that is stored
in the template's status after successful discovery.

The profile serves as a cache of runtime configuration, eliminating the need to
re-run discovery for each service that uses this template. Services and caching
mechanisms reference this cached profile for deployment parameters and model sources.

See discovery.go for AIMDiscoveryProfile (the raw discovery output) and the
relationship between these types.



_Appears in:_
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `engine_args` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#json-v1-apiextensions-k8s-io)_ | EngineArgs contains runtime-specific engine configuration as a free-form JSON object.<br />The structure depends on the inference engine being used (e.g., vLLM, TGI).<br />These arguments are passed to the runtime container to configure model loading and inference. |  | Schemaless: \{\} <br /> |
| `env_vars` _object (keys:string, values:string)_ | EnvVars contains environment variables required by the runtime for this profile.<br />These may include engine-specific settings, optimization flags, or hardware configuration. |  |  |
| `metadata` _[AIMProfileMetadata](#aimprofilemetadata)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |


#### AIMProfileMetadata



AIMProfileMetadata describes the characteristics of a cached deployment profile.
This is identical to AIMDiscoveryProfileMetadata but exists in the template status namespace.



_Appears in:_
- [AIMProfile](#aimprofile)
- [AIMServiceResolvedTemplateProfile](#aimserviceresolvedtemplateprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `engine` _string_ | Engine identifies the inference engine used for this profile (e.g., "vllm", "tgi"). |  |  |
| `gpu` _string_ | GPU specifies the GPU model this profile is optimized for (e.g., "MI300X", "MI325X"). |  |  |
| `gpuCount` _integer_ | GPUCount indicates how many GPUs are required per replica for this profile. |  |  |
| `metric` _[AIMMetric](#aimmetric)_ | Metric indicates the optimization goal for this profile ("latency" or "throughput"). |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision specifies the numeric precision used in this profile (e.g., "fp16", "fp8"). |  | Enum: [bf16 fp16 fp8 int8] <br /> |
| `type` _[AIMProfileType](#aimprofiletype)_ | Type specifies the designation of the profile |  |  |


#### AIMProfileType

_Underlying type:_ _string_





_Appears in:_
- [AIMDiscoveryProfileMetadata](#aimdiscoveryprofilemetadata)
- [AIMProfileMetadata](#aimprofilemetadata)

| Field | Description |
| --- | --- |
| `optimized` |  |
| `unoptimized` |  |
| `preview` |  |


#### AIMResolutionScope

_Underlying type:_ _string_

AIMResolutionScope describes the scope of a resolved reference.

_Validation:_
- Enum: [Namespace Cluster Merged Unknown]

_Appears in:_
- [AIMResolvedReference](#aimresolvedreference)
- [AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)
- [AIMServiceResolvedTemplate](#aimserviceresolvedtemplate)

| Field | Description |
| --- | --- |
| `Namespace` | AIMResolutionScopeNamespace denotes a namespace-scoped resource.<br /> |
| `Cluster` | AIMResolutionScopeCluster denotes a cluster-scoped resource.<br /> |
| `Merged` | AIMResolutionScopeMerged denotes that both cluster and namespace configs were merged.<br /> |
| `Unknown` | AIMResolutionScopeUnknown denotes that the scope could not be determined.<br /> |


#### AIMResolvedModelCache



AIMResolvedModelCache contains reference info and status for a cached model.



_Appears in:_
- [AIMServiceStatus](#aimservicestatus)
- [AIMTemplateCacheStatus](#aimtemplatecachestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `uid` _string_ | UID of the AIMModelCache resource |  |  |
| `name` _string_ | Name of the AIMModelCache resource |  |  |
| `model` _string_ | Model is the name of the model that is cached |  |  |
| `status` _[AIMModelCacheStatusEnum](#aimmodelcachestatusenum)_ | Status of the model cache |  | Enum: [Pending Progressing Available Failed] <br /> |
| `persistentVolumeClaim` _string_ | PersistentVolumeClaim name if available |  |  |
| `mountPoint` _string_ | MountPoint is the mount point for the model cache |  |  |


#### AIMResolvedReference



AIMResolvedReference captures metadata about a resolved reference.



_Appears in:_
- [AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)
- [AIMServiceResolvedTemplate](#aimserviceresolvedtemplate)
- [AIMServiceStatus](#aimservicestatus)
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the resource name that satisfied the reference. |  |  |
| `namespace` _string_ | Namespace identifies where the resource was found when namespace-scoped.<br />Empty indicates a cluster-scoped resource. |  |  |
| `scope` _[AIMResolutionScope](#aimresolutionscope)_ | Scope indicates whether the resolved resource was namespace or cluster scoped. |  | Enum: [Namespace Cluster Merged Unknown] <br /> |
| `kind` _string_ | Kind is the fully-qualified kind of the resolved reference, when known. |  |  |
| `uid` _[UID](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#uid-types-pkg)_ | UID captures the unique identifier of the resolved reference, when known. |  |  |


#### AIMResolvedRuntimeConfig



AIMResolvedRuntimeConfig captures metadata about the runtime config that was resolved.
This follows the same pattern as AIMServiceResolvedTemplate for consistency.



_Appears in:_
- [AIMModelStatus](#aimmodelstatus)
- [AIMServiceStatus](#aimservicestatus)
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)
- [AIMTemplateCacheStatus](#aimtemplatecachestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the resource name that satisfied the reference. |  |  |
| `namespace` _string_ | Namespace identifies where the resource was found when namespace-scoped.<br />Empty indicates a cluster-scoped resource. |  |  |
| `scope` _[AIMResolutionScope](#aimresolutionscope)_ | Scope indicates whether the resolved resource was namespace or cluster scoped. |  | Enum: [Namespace Cluster Merged Unknown] <br /> |
| `kind` _string_ | Kind is the fully-qualified kind of the resolved reference, when known. |  |  |
| `uid` _[UID](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#uid-types-pkg)_ | UID captures the unique identifier of the resolved reference, when known. |  |  |


#### AIMRuntimeConfig



AIMRuntimeConfig defines namespace-scoped runtime overrides for AIM resources.



_Appears in:_
- [AIMRuntimeConfigList](#aimruntimeconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMRuntimeConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMRuntimeConfigSpec](#aimruntimeconfigspec)_ |  |  |  |
| `status` _[AIMRuntimeConfigStatus](#aimruntimeconfigstatus)_ |  |  |  |


#### AIMRuntimeConfigCommon



AIMRuntimeConfigCommon captures configuration fields shared across cluster and namespace scopes.
These settings apply to both AIMRuntimeConfig (namespace-scoped) and AIMClusterRuntimeConfig (cluster-scoped).



_Appears in:_
- [AIMClusterRuntimeConfigSpec](#aimclusterruntimeconfigspec)
- [AIMRuntimeConfigSpec](#aimruntimeconfigspec)
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultStorageClassName` _string_ | DefaultStorageClassName specifies the storage class to use for model caches and PVCs<br />when the consuming resource (AIMModelCache, AIMTemplateCache, AIMServiceTemplate) does not<br />specify a storage class. If this field is empty, the cluster's default storage class is used. |  |  |
| `model` _[AIMModelConfig](#aimmodelconfig)_ | Model controls model creation and discovery defaults. |  |  |
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |
| `pvcHeadroomPercent` _integer_ | PVCHeadroomPercent specifies the percentage of extra space to add to PVCs<br />for model storage. This accounts for filesystem overhead and temporary files<br />during model loading. The value represents a percentage (e.g., 10 means 10% extra space).<br />If not specified, defaults to 10%. |  | Minimum: 0 <br /> |
| `labelPropagation` _[AIMRuntimeConfigLabelPropagationSpec](#aimruntimeconfiglabelpropagationspec)_ | LabelPropagation controls how labels from parent AIM resources are propagated to child resources.<br />When enabled, labels matching the specified patterns are automatically copied from parent resources<br />(e.g., AIMService, AIMTemplateCache) to their child resources (e.g., Deployments, Services, PVCs).<br />This is useful for propagating organizational metadata like cost centers, team identifiers,<br />or compliance labels through the resource hierarchy. |  |  |


#### AIMRuntimeConfigLabelPropagationSpec







_Appears in:_
- [AIMClusterRuntimeConfigSpec](#aimclusterruntimeconfigspec)
- [AIMRuntimeConfigCommon](#aimruntimeconfigcommon)
- [AIMRuntimeConfigSpec](#aimruntimeconfigspec)
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled, if true, allows propagating parent labels to all child resources it creates directly<br />Only label keys that match the ones in Match are propagated. | false |  |
| `match` _string array_ | Match is a list of label keys that will be propagated to any child resources created.<br />Wildcards are supported, so for example `org.my/my-key-*` would match any label with that prefix. |  |  |


#### AIMRuntimeConfigList



AIMRuntimeConfigList contains a list of AIMRuntimeConfig.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMRuntimeConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMRuntimeConfig](#aimruntimeconfig) array_ |  |  |  |


#### AIMRuntimeConfigSpec



AIMRuntimeConfigSpec defines namespace-scoped overrides for AIM resources.



_Appears in:_
- [AIMRuntimeConfig](#aimruntimeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultStorageClassName` _string_ | DefaultStorageClassName specifies the storage class to use for model caches and PVCs<br />when the consuming resource (AIMModelCache, AIMTemplateCache, AIMServiceTemplate) does not<br />specify a storage class. If this field is empty, the cluster's default storage class is used. |  |  |
| `model` _[AIMModelConfig](#aimmodelconfig)_ | Model controls model creation and discovery defaults. |  |  |
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |
| `pvcHeadroomPercent` _integer_ | PVCHeadroomPercent specifies the percentage of extra space to add to PVCs<br />for model storage. This accounts for filesystem overhead and temporary files<br />during model loading. The value represents a percentage (e.g., 10 means 10% extra space).<br />If not specified, defaults to 10%. |  | Minimum: 0 <br /> |
| `labelPropagation` _[AIMRuntimeConfigLabelPropagationSpec](#aimruntimeconfiglabelpropagationspec)_ | LabelPropagation controls how labels from parent AIM resources are propagated to child resources.<br />When enabled, labels matching the specified patterns are automatically copied from parent resources<br />(e.g., AIMService, AIMTemplateCache) to their child resources (e.g., Deployments, Services, PVCs).<br />This is useful for propagating organizational metadata like cost centers, team identifiers,<br />or compliance labels through the resource hierarchy. |  |  |


#### AIMRuntimeConfigStatus



AIMRuntimeConfigStatus records the resolved config reference surfaced to consumers.



_Appears in:_
- [AIMClusterRuntimeConfig](#aimclusterruntimeconfig)
- [AIMRuntimeConfig](#aimruntimeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the last reconciled generation. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions communicate reconciliation progress. |  |  |


#### AIMRuntimeParameters



AIMRuntimeParameters contains the runtime configuration parameters shared
across templates and services. Fields use pointers to allow optional usage
in different contexts (required in templates, optional in service overrides).



_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMServiceOverrides](#aimserviceoverrides)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |


#### AIMRuntimeRoutingConfig



AIMRuntimeRoutingConfig configures HTTP routing defaults for inference services.
These settings control how Gateway API HTTPRoutes are created and configured.



_Appears in:_
- [AIMClusterRuntimeConfigSpec](#aimclusterruntimeconfigspec)
- [AIMRuntimeConfigCommon](#aimruntimeconfigcommon)
- [AIMRuntimeConfigSpec](#aimruntimeconfigspec)
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether HTTP routing is managed for inference services using this config.<br />When true, the operator creates HTTPRoute resources for services that reference this config.<br />When false or unset, routing must be explicitly enabled on each service.<br />This provides a namespace or cluster-wide default that individual services can override. |  |  |
| `gatewayRef` _[ParentReference](#parentreference)_ | GatewayRef specifies the Gateway API Gateway resource that should receive HTTPRoutes.<br />This identifies the parent gateway for routing traffic to inference services.<br />The gateway can be in any namespace (cross-namespace references are supported).<br />If routing is enabled but GatewayRef is not specified, service reconciliation will fail<br />with a validation error. |  |  |
| `pathTemplate` _string_ | PathTemplate defines the HTTP path template for routes, evaluated using JSONPath expressions.<br />The template is rendered against the AIMService object to generate unique paths.<br />Example templates:<br />- `/\{.metadata.namespace\}/\{.metadata.name\}` - namespace and service name<br />- `/\{.metadata.namespace\}/\{.metadata.labels['team']\}/inference` - with label<br />- `/models/\{.spec.aimModelName\}` - based on model name<br />The template must:<br />- Use valid JSONPath expressions wrapped in \{...\}<br />- Reference fields that exist on the service<br />- Produce a path ≤ 200 characters after rendering<br />- Result in valid URL path segments (lowercase, RFC 1123 compliant)<br />If evaluation fails, the service enters Degraded state with PathTemplateInvalid reason.<br />Individual services can override this template via spec.Routing.pathTemplate. |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations defines additional annotations to add to the HTTPRoute resource.<br />These annotations can be used for various purposes such as configuring ingress<br />behavior, adding metadata, or triggering external integrations.<br />Individual services can override these via spec.routing.annotations. |  |  |
| `requestTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#duration-v1-meta)_ | RequestTimeout defines the HTTP request timeout for routes.<br />This sets the maximum duration for a request to complete before timing out.<br />The timeout applies to the entire request/response cycle.<br />If not specified, no timeout is set on the route.<br />Individual services can override this value via spec.routing.requestTimeout. |  |  |


#### AIMService



AIMService manages a KServe-based AIM inference service for the selected model and template.



_Appears in:_
- [AIMServiceList](#aimservicelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMService` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMServiceSpec](#aimservicespec)_ |  |  |  |
| `status` _[AIMServiceStatus](#aimservicestatus)_ |  |  |  |


#### AIMServiceAutoScaling



AIMServiceAutoScaling mirrors KServe's AutoScalingSpec for advanced autoscaling configuration.
Supports custom metrics from various backends including Prometheus, OpenTelemetry, and KEDA.



_Appears in:_
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metrics` _[AIMServiceMetricsSpec](#aimservicemetricsspec) array_ | Metrics is a list of metrics spec to be used for autoscaling.<br />Each metric defines a source (Resource, External, or PodMetric) and target values. |  |  |


#### AIMServiceKVCache



AIMServiceKVCache specifies KV cache configuration for the service.
The controller will use an existing AIMKVCache if found, otherwise it will create one.



_Appears in:_
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name specifies the name of the AIMKVCache resource to use.<br />If an AIMKVCache with this name exists, it will be used.<br />If it doesn't exist, a new AIMKVCache will be created with this name.<br />If not specified, defaults to "kvcache-\{namespace\}". |  |  |
| `type` _string_ | Type specifies the type of KV cache backend.<br />Only used when creating a new AIMKVCache (ignored if referencing existing). | redis | Enum: [redis] <br /> |
| `image` _string_ | Image specifies the container image to use for the KV cache service.<br />Only used when creating a new AIMKVCache (ignored if referencing existing).<br />If not specified, defaults to appropriate images based on Type. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to set in the KV cache container.<br />Only used when creating a new AIMKVCache (ignored if referencing existing).<br />If not specified (nil), no additional environment variables are set.<br />If explicitly set to an empty array, no environment variables are added. |  |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage defines the persistent storage configuration for the KV cache.<br />Only used when creating a new AIMKVCache (ignored if referencing existing). |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the KV cache container.<br />Only used when creating a new AIMKVCache (ignored if referencing existing).<br />If not specified, defaults to 1 CPU and 1Gi memory for both requests and limits. |  |  |
| `lmCacheConfig` _string_ | LMCacheConfig specifies the custom LMCache configuration YAML content.<br />When specified, this exact configuration is used for the lmcache_config.yaml file.<br />When empty, a default configuration is generated with standard LMCache settings.<br />Note: The remote_url field in custom configs will have the \{SERVICE_URL\} placeholder<br />replaced with the actual KV cache service URL. |  |  |


#### AIMServiceList



AIMServiceList contains a list of AIMService.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMServiceList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMService](#aimservice) array_ |  |  |  |


#### AIMServiceMetricTarget



AIMServiceMetricTarget defines the target value for a metric.
Specifies how the metric value should be interpreted and what target to maintain.



_Appears in:_
- [AIMServicePodMetricSource](#aimservicepodmetricsource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type specifies how to interpret the metric value.<br />"Value": absolute value target (use Value field)<br />"AverageValue": average value across all pods (use AverageValue field)<br />"Utilization": percentage utilization for resource metrics (use AverageUtilization field) |  | Enum: [Value AverageValue Utilization] <br /> |
| `value` _string_ | Value is the target value of the metric (as a quantity).<br />Used when Type is "Value".<br />Example: "1" for 1 request, "100m" for 100 millicores |  |  |
| `averageValue` _string_ | AverageValue is the target value of the average of the metric across all relevant pods (as a quantity).<br />Used when Type is "AverageValue".<br />Example: "100m" for 100 millicores per pod |  |  |
| `averageUtilization` _integer_ | AverageUtilization is the target value of the average of the resource metric across all relevant pods,<br />represented as a percentage of the requested value of the resource for the pods.<br />Used when Type is "Utilization". Only valid for Resource metric source type.<br />Example: 80 for 80% utilization |  |  |


#### AIMServiceMetricsSpec



AIMServiceMetricsSpec defines a single metric for autoscaling.
Specifies the metric source type and configuration.



_Appears in:_
- [AIMServiceAutoScaling](#aimserviceautoscaling)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the type of metric source.<br />Valid values: "PodMetric" (per-pod custom metrics). Features to come: Resource, External |  | Enum: [PodMetric] <br /> |
| `podmetric` _[AIMServicePodMetricSource](#aimservicepodmetricsource)_ | PodMetric refers to a metric describing each pod in the current scale target.<br />Used when Type is "PodMetric". Supports backends like OpenTelemetry for custom metrics. |  |  |


#### AIMServiceModel



AIMServiceModel specifies which model to deploy. Exactly one field must be set.



_Appears in:_
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ref` _string_ | Ref references an existing AIMModel or AIMClusterModel by metadata.name.<br />The controller looks for a namespace-scoped AIMModel first, then falls back to cluster-scoped AIMClusterModel.<br />Example: `meta-llama-3-8b`. |  |  |
| `image` _string_ | Image specifies a container image URI directly.<br />The controller searches for an existing model with this image, or creates one if none exists.<br />The scope of the created model is controlled by the runtime config's ModelCreationScope field.<br />Example: `ghcr.io/silogen/llama-3-8b:v1.2.0`. |  |  |


#### AIMServiceOverrides



AIMServiceOverrides allows overriding template parameters at the service level.
All fields are optional. When specified, they override the corresponding values
from the referenced AIMServiceTemplate.



_Appears in:_
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |


#### AIMServicePodMetric



AIMServicePodMetric identifies the pod metric and its backend.
Supports multiple metrics backends including OpenTelemetry, Prometheus, etc.



_Appears in:_
- [AIMServicePodMetricSource](#aimservicepodmetricsource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `backend` _string_ | Backend defines the metrics backend to use.<br />If not specified, defaults to "opentelemetry". | opentelemetry | Enum: [opentelemetry] <br /> |
| `serverAddress` _string_ | ServerAddress specifies the address of the metrics backend server.<br />If not specified, defaults to "keda-otel-scaler.keda.svc:4317" for OpenTelemetry backend. |  |  |
| `metricNames` _string array_ | MetricNames specifies which metrics to collect from pods and send to ServerAddress.<br />Example: ["vllm:num_requests_running"] |  |  |
| `query` _string_ | Query specifies the query to run to retrieve metrics from the backend.<br />The query syntax depends on the backend being used.<br />Example: "vllm:num_requests_running" for OpenTelemetry. |  |  |
| `operationOverTime` _string_ | OperationOverTime specifies the operation to aggregate metrics over time.<br />Valid values: "last_one", "avg", "max", "min", "rate", "count"<br />Default: "last_one" |  |  |


#### AIMServicePodMetricSource



AIMServicePodMetricSource defines pod-level metrics configuration.
Specifies the metric identification and target values for pod-based autoscaling.



_Appears in:_
- [AIMServiceMetricsSpec](#aimservicemetricsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metric` _[AIMServicePodMetric](#aimservicepodmetric)_ | Metric contains the metric identification and backend configuration.<br />Defines which metrics to collect and how to query them. |  |  |
| `target` _[AIMServiceMetricTarget](#aimservicemetrictarget)_ | Target specifies the target value for the metric.<br />The autoscaler will scale to maintain this target value. |  |  |


#### AIMServiceResolvedTemplate



AIMServiceResolvedTemplate retains the historical name while reusing the shared structure.



_Appears in:_
- [AIMServiceStatus](#aimservicestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the resource name that satisfied the reference. |  |  |
| `namespace` _string_ | Namespace identifies where the resource was found when namespace-scoped.<br />Empty indicates a cluster-scoped resource. |  |  |
| `scope` _[AIMResolutionScope](#aimresolutionscope)_ | Scope indicates whether the resolved resource was namespace or cluster scoped. |  | Enum: [Namespace Cluster Merged Unknown] <br /> |
| `kind` _string_ | Kind is the fully-qualified kind of the resolved reference, when known. |  |  |
| `uid` _[UID](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#uid-types-pkg)_ | UID captures the unique identifier of the resolved reference, when known. |  |  |
| `profile` _[AIMServiceResolvedTemplateProfile](#aimserviceresolvedtemplateprofile)_ | Profile is the profile that the resolved template points to |  |  |


#### AIMServiceResolvedTemplateProfile







_Appears in:_
- [AIMServiceResolvedTemplate](#aimserviceresolvedtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[AIMProfileMetadata](#aimprofilemetadata)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |


#### AIMServiceRoutingStatus



AIMServiceRoutingStatus captures observed routing details.



_Appears in:_
- [AIMServiceStatus](#aimservicestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | Path is the HTTP path prefix used when routing is enabled.<br />Example: `/tenant/svc-uuid`. |  |  |


#### AIMServiceSpec



AIMServiceSpec defines the desired state of AIMService.

Binds a canonical model to an AIMServiceTemplate and configures replicas,
caching behavior, and optional overrides. The template governs the base
runtime selection knobs, while the overrides field allows service-specific
customization.



_Appears in:_
- [AIMService](#aimservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `model` _[AIMServiceModel](#aimservicemodel)_ | Model specifies which model to deploy using one of the available reference methods.<br />Use `ref` to reference an existing AIMModel/AIMClusterModel by name, or use `image`<br />to specify a container image URI directly (which will auto-create a model if needed). |  |  |
| `templateRef` _string_ | TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to use.<br />The template selects the runtime profile and GPU parameters. |  |  |
| `template` _[AIMServiceTemplateConfig](#aimservicetemplateconfig)_ | Template contains the AIMServiceTemplate selection configuration |  |  |
| `cacheModel` _boolean_ | CacheModel requests that model sources be cached when starting the service<br />if the template itself does not warm the cache.<br />When `warmCache: false` on the template, this setting ensures caching is<br />performed before the service becomes ready. | false |  |
| `replicas` _integer_ | Replicas overrides the number of replicas for this service.<br />Other runtime settings remain governed by the template unless overridden. | 1 |  |
| `minReplicas` _integer_ | MinReplicas specifies the minimum number of replicas for autoscaling.<br />Defaults to 1. Scale to zero not supported.<br />When specified with MaxReplicas, enables autoscaling for the service. |  | Minimum: 1 <br /> |
| `maxReplicas` _integer_ | MaxReplicas specifies the maximum number of replicas for autoscaling.<br />Required when MinReplicas is set or when AutoScaling configuration is provided. |  | Minimum: 1 <br /> |
| `autoScaling` _[AIMServiceAutoScaling](#aimserviceautoscaling)_ | AutoScaling configures advanced autoscaling behavior using HPA or KEDA.<br />Supports custom metrics from various backends (Prometheus, OpenTelemetry, etc.)<br />When specified, MinReplicas and MaxReplicas should also be set. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this service. | default |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources overrides the container resource requirements for this service.<br />When specified, these values take precedence over the template and image defaults. |  |  |
| `kvCache` _[AIMServiceKVCache](#aimservicekvcache)_ | KVCache specifies KV cache configuration for the service.<br />When specified, enables LMCache with the configured KV cache backend. |  |  |
| `overrides` _[AIMServiceOverrides](#aimserviceoverrides)_ | Overrides allows overriding specific template parameters for this service.<br />When specified, these values take precedence over the template values. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for the inference workload.<br />This service account is used by the deployed inference pods.<br />If empty, the default service account for the namespace is used. |  |  |
| `defaultStorageClassName` _string_ | DefaultStorageClassName specifies the storage class to use for model caches and PVCs<br />when the consuming resource (AIMModelCache, AIMTemplateCache, AIMServiceTemplate) does not<br />specify a storage class. If this field is empty, the cluster's default storage class is used. |  |  |
| `model` _[AIMModelConfig](#aimmodelconfig)_ | Model controls model creation and discovery defaults. |  |  |
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |
| `pvcHeadroomPercent` _integer_ | PVCHeadroomPercent specifies the percentage of extra space to add to PVCs<br />for model storage. This accounts for filesystem overhead and temporary files<br />during model loading. The value represents a percentage (e.g., 10 means 10% extra space).<br />If not specified, defaults to 10%. |  | Minimum: 0 <br /> |
| `labelPropagation` _[AIMRuntimeConfigLabelPropagationSpec](#aimruntimeconfiglabelpropagationspec)_ | LabelPropagation controls how labels from parent AIM resources are propagated to child resources.<br />When enabled, labels matching the specified patterns are automatically copied from parent resources<br />(e.g., AIMService, AIMTemplateCache) to their child resources (e.g., Deployments, Services, PVCs).<br />This is useful for propagating organizational metadata like cost centers, team identifiers,<br />or compliance labels through the resource hierarchy. |  |  |


#### AIMServiceStatus



AIMServiceStatus defines the observed state of AIMService.



_Appears in:_
- [AIMService](#aimservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of template state. |  |  |
| `resolvedRuntimeConfig` _[AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)_ | ResolvedRuntimeConfig captures metadata about the runtime config that was resolved. |  |  |
| `resolvedImage` _[AIMResolvedReference](#aimresolvedreference)_ | ResolvedImage captures metadata about the image that was resolved. |  |  |
| `status` _[AIMServiceStatusEnum](#aimservicestatusenum)_ | Status represents the current high‑level status of the service lifecycle.<br />Values: `Pending`, `Starting`, `Running`, `Failed`, `Degraded`. | Pending | Enum: [Pending Starting Running Failed Degraded] <br /> |
| `routing` _[AIMServiceRoutingStatus](#aimserviceroutingstatus)_ | Routing surfaces information about the configured HTTP routing, when enabled. |  |  |
| `resolvedTemplate` _[AIMServiceResolvedTemplate](#aimserviceresolvedtemplate)_ | ResolvedTemplate captures metadata about the template that satisfied the reference. |  |  |
| `resolvedTemplateCache` _[AIMResolvedReference](#aimresolvedreference)_ | ResolvedTemplateCache captures metadata about the template cache being used, if any. |  |  |
| `modelCaches` _object (keys:string, values:[AIMResolvedModelCache](#aimresolvedmodelcache))_ | ModelCaches maps model names to their resolved AIMModelCache resources if they exist. |  |  |
| `resolvedKVCache` _[AIMResolvedReference](#aimresolvedreference)_ | ResolvedKVCache captures metadata about the KV cache being used, if any. |  |  |
| `templateMatching` _[AIMTemplateMatchingStatus](#aimtemplatematchingstatus)_ | TemplateMatching provides detailed information about template selection,<br />including which templates were evaluated and why each was chosen or rejected. |  |  |


#### AIMServiceStatusEnum

_Underlying type:_ _string_

AIMServiceStatusEnum defines coarse-grained states for a service.

_Validation:_
- Enum: [Pending Starting Running Failed Degraded]

_Appears in:_
- [AIMServiceStatus](#aimservicestatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMServiceStatusPending denotes that the template has been created and discovery has not yet started.<br /> |
| `Starting` | AIMServiceStatusStarting denotes that discovery and/or cache warm is in progress.<br /> |
| `Running` | AIMServiceStatusRunning denotes that discovery succeeded and, if requested, caches are warmed.<br /> |
| `Failed` | AIMServiceStatusFailed denotes a terminal failure for discovery or warm operations.<br /> |
| `Degraded` | AIMServiceStatusDegraded denotes a recoverable failure state.<br /> |


#### AIMServiceTemplate







_Appears in:_
- [AIMServiceTemplateList](#aimservicetemplatelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMServiceTemplate` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMServiceTemplateSpec](#aimservicetemplatespec)_ |  |  |  |
| `status` _[AIMServiceTemplateStatus](#aimservicetemplatestatus)_ |  |  |  |


#### AIMServiceTemplateConfig







_Appears in:_
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allowUnoptimized` _boolean_ | AllowUnoptimized, if true, will allow automatic selection of templates that resolve to an unoptimized profile. |  |  |


#### AIMServiceTemplateList









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMServiceTemplateList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMServiceTemplate](#aimservicetemplate) array_ |  |  |  |




#### AIMServiceTemplateSpec



AIMServiceTemplateSpec defines the desired state of AIMServiceTemplate (namespace-scoped).

A namespaced and versioned template that selects a runtime profile
for a given AIM model (by canonical name). Templates are intentionally
narrow: they describe runtime selection knobs for the AIM container and do
not redefine the full Kubernetes deployment shape.



_Appears in:_
- [AIMServiceTemplate](#aimservicetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `modelName` _string_ | ModelName is the model name. Matches `metadata.name` of an AIMModel or AIMClusterModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template. | default |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets lists secrets containing credentials for pulling container images.<br />These secrets are used for:<br />- Discovery dry-run jobs that inspect the model container<br />- Pulling the image for inference services<br />The secrets are merged with any model or runtime config defaults.<br />For namespace-scoped templates, secrets must exist in the same namespace.<br />For cluster-scoped templates, secrets must exist in the operator namespace. |  |  |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for workloads related to this template.<br />This includes discovery dry-run jobs and inference services created from this template.<br />If empty, the default service account for the namespace is used. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default container resource requirements applied to services derived from this template.<br />Service-specific values override the template defaults. |  |  |
| `modelSources` _[AIMModelSource](#aimmodelsource) array_ | ModelSources specifies the model artifacts required to run this template.<br />When provided, the discovery dry-run will be skipped and these sources will be used directly.<br />This allows users to explicitly declare model dependencies without requiring a discovery job.<br />If omitted, a discovery job will be run to automatically determine the required model sources. |  |  |
| `profileId` _string_ | ProfileId is the specific AIM profile ID that this template should use |  |  |
| `caching` _[AIMTemplateCachingConfig](#aimtemplatecachingconfig)_ | Caching configures model caching behavior for this namespace-scoped template.<br />When enabled, models will be cached using the specified environment variables<br />during download. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |


#### AIMServiceTemplateSpecCommon



AIMServiceTemplateSpecCommon contains the shared fields for both cluster-scoped
and namespace-scoped service templates.



_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `modelName` _string_ | ModelName is the model name. Matches `metadata.name` of an AIMModel or AIMClusterModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template. | default |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets lists secrets containing credentials for pulling container images.<br />These secrets are used for:<br />- Discovery dry-run jobs that inspect the model container<br />- Pulling the image for inference services<br />The secrets are merged with any model or runtime config defaults.<br />For namespace-scoped templates, secrets must exist in the same namespace.<br />For cluster-scoped templates, secrets must exist in the operator namespace. |  |  |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for workloads related to this template.<br />This includes discovery dry-run jobs and inference services created from this template.<br />If empty, the default service account for the namespace is used. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default container resource requirements applied to services derived from this template.<br />Service-specific values override the template defaults. |  |  |
| `modelSources` _[AIMModelSource](#aimmodelsource) array_ | ModelSources specifies the model artifacts required to run this template.<br />When provided, the discovery dry-run will be skipped and these sources will be used directly.<br />This allows users to explicitly declare model dependencies without requiring a discovery job.<br />If omitted, a discovery job will be run to automatically determine the required model sources. |  |  |
| `profileId` _string_ | ProfileId is the specific AIM profile ID that this template should use |  |  |


#### AIMServiceTemplateStatus



AIMServiceTemplateStatus defines the observed state of AIMServiceTemplate.



_Appears in:_
- [AIMClusterServiceTemplate](#aimclusterservicetemplate)
- [AIMServiceTemplate](#aimservicetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of template state. |  |  |
| `resolvedRuntimeConfig` _[AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)_ | ResolvedRuntimeConfig captures metadata about the runtime config that was resolved. |  |  |
| `resolvedModel` _[AIMResolvedReference](#aimresolvedreference)_ | ResolvedModel captures metadata about the image that was resolved. |  |  |
| `status` _[AIMTemplateStatusEnum](#aimtemplatestatusenum)_ | Status represents the current high‑level status of the template lifecycle.<br />Values: `Pending`, `Progressing`, `Ready`, `Failed`, `NotAvailable`. | Pending | Enum: [Pending Progressing NotAvailable Ready Degraded Failed] <br /> |
| `modelSources` _[AIMModelSource](#aimmodelsource) array_ | ModelSources list the models that this template requires to run. These are the models that will be<br />cached, if this template is cached. |  |  |
| `profile` _[AIMProfile](#aimprofile)_ | Profile contains the full discovery result profile as a free-form JSON object.<br />This includes metadata, engine args, environment variables, and model details. |  |  |




#### AIMTemplateCache



AIMTemplateCache pre-warms model caches for a specified template.



_Appears in:_
- [AIMTemplateCacheList](#aimtemplatecachelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMTemplateCache` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMTemplateCacheSpec](#aimtemplatecachespec)_ |  |  |  |
| `status` _[AIMTemplateCacheStatus](#aimtemplatecachestatus)_ |  |  |  |


#### AIMTemplateCacheList



AIMTemplateCacheList contains a list of AIMTemplateCache.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMTemplateCacheList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMTemplateCache](#aimtemplatecache) array_ |  |  |  |


#### AIMTemplateCacheSpec



AIMTemplateCacheSpec defines the desired state of AIMTemplateCache



_Appears in:_
- [AIMTemplateCache](#aimtemplatecache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _string_ | TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to cache.<br />The controller will first look for a namespace-scoped AIMServiceTemplate in the same namespace.<br />If not found, it will look for a cluster-scoped AIMClusterServiceTemplate with the same name.<br />Namespace-scoped templates take priority over cluster-scoped templates. |  | MinLength: 1 <br /> |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |
| `storageClassName` _string_ | StorageClassName is the name for the storage class to use for this cache<br />If not set the cluster default will be used |  |  |
| `downloadImage` _string_ | The image that should be used to download the models<br />If not set the model cache controller will decide |  |  |
| `modelSources` _[AIMModelSource](#aimmodelsource) array_ | ModelSources are set by the template that wants these cached |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template cache. | default |  |


#### AIMTemplateCacheStatus



AIMTemplateCacheStatus defines the observed state of AIMTemplateCache



_Appears in:_
- [AIMTemplateCache](#aimtemplatecache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of the template cache state. |  |  |
| `resolvedRuntimeConfig` _[AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)_ | ResolvedRuntimeConfig captures metadata about the runtime config that was resolved. |  |  |
| `status` _[AIMTemplateCacheStatusEnum](#aimtemplatecachestatusenum)_ | Status represents the current high-level status of the template cache. | Pending | Enum: [Pending Progressing Available Failed] <br /> |
| `resolvedTemplateKind` _string_ | ResolvedTemplateKind indicates whether the template resolved to a namespace-scoped<br />AIMServiceTemplate or cluster-scoped AIMClusterServiceTemplate.<br />Values: "AIMServiceTemplate", "AIMClusterServiceTemplate" |  |  |
| `modelCaches` _object (keys:string, values:[AIMResolvedModelCache](#aimresolvedmodelcache))_ | ModelCaches maps model names to their resolved AIMModelCache resources. |  |  |


#### AIMTemplateCacheStatusEnum

_Underlying type:_ _string_

AIMTemplateCacheStatusEnum defines the status of the template cache.

_Validation:_
- Enum: [Pending Progressing Available Failed]

_Appears in:_
- [AIMTemplateCacheStatus](#aimtemplatecachestatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMTemplateCacheStatusPending denotes that the template cache has been created but not yet processed.<br /> |
| `Progressing` | AIMTemplateCacheStatusProgressing denotes that the template cache is being warmed.<br /> |
| `Available` | AIMTemplateCacheStatusAvailable denotes that the template cache is ready and models are cached.<br /> |
| `Failed` | AIMTemplateCacheStatusFailed denotes that the template cache operation has failed.<br /> |


#### AIMTemplateCachingConfig



AIMTemplateCachingConfig configures model caching behavior for namespace-scoped templates.



_Appears in:_
- [AIMServiceTemplateSpec](#aimservicetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether caching is enabled for this template.<br />Defaults to `false`. | false |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use when downloading the model.<br />These variables are available to the model download process and can be used<br />to configure download behavior, authentication, proxies, etc. |  |  |


#### AIMTemplateCandidateResult



AIMTemplateCandidateResult describes why a template was chosen or rejected.



_Appears in:_
- [AIMTemplateMatchingStatus](#aimtemplatematchingstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the template. |  |  |
| `status` _string_ | Status indicates whether this template was chosen or rejected. |  | Enum: [chosen rejected] <br /> |
| `reason` _string_ | Reason explains why the template was chosen or rejected.<br />Possible rejection reasons:<br />- TemplatePending: Template status is Pending<br />- TemplateDegraded: Template status is Degraded<br />- TemplateFailed: Template status is Failed<br />- TemplateNotAvailable: Template status is NotAvailable<br />- UnoptimizedTemplateFiltered: Template is unoptimized and allowUnoptimized is false<br />- ServiceOverridesNotMatched: Template doesn't match service overrides<br />- RequiredGPUNotInCluster: Template requires a GPU not available in cluster<br />- LowerPreferenceRank: Template passed all filters but scored lower in preference ranking<br />Chosen reason:<br />- BestMatch: Template was selected as the best match |  |  |


#### AIMTemplateMatchingStatus



AIMTemplateMatchingStatus captures the result of template selection for a service.



_Appears in:_
- [AIMServiceStatus](#aimservicestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `candidates` _[AIMTemplateCandidateResult](#aimtemplatecandidateresult) array_ | Candidates lists all templates that were evaluated for this service. |  |  |


#### AIMTemplateStatusEnum

_Underlying type:_ _string_

AIMTemplateStatusEnum defines coarse-grained states for a template.

_Validation:_
- Enum: [Pending Progressing NotAvailable Ready Degraded Failed]

_Appears in:_
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMTemplateStatusPending denotes that the template has been created and discovery has not yet started.<br /> |
| `Progressing` | AIMTemplateStatusProgressing denotes that discovery and/or cache warm is in progress.<br /> |
| `NotAvailable` | AIMTemplateStatusNotAvailable denotes that the template cannot run because the required GPU resources are not present in the cluster.<br /> |
| `Ready` | AIMTemplateStatusReady denotes that discovery succeeded and, if requested, caches are warmed.<br /> |
| `Degraded` | AIMTemplateStatusDegraded denotes that the template is non-functional for some reason, for example that the cluster doesn't have the resources specified.<br /> |
| `Failed` | AIMTemplateStatusFailed denotes a terminal failure for discovery or warm operations.<br /> |


#### ImageMetadata



ImageMetadata contains metadata extracted from or provided for a container image.



_Appears in:_
- [AIMModelStatus](#aimmodelstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `model` _[ModelMetadata](#modelmetadata)_ | Model contains AMD Silogen model-specific metadata. |  |  |
| `oci` _[OCIMetadata](#ocimetadata)_ | OCI contains standard OCI image metadata. |  |  |
| `originalLabels` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#json-v1-apiextensions-k8s-io)_ | OriginalLabels contains the originally parsed metadata from the image registry.<br />This is stored as JSON to preserve the raw label data. |  | Type: object <br /> |


#### ModelMetadata



ModelMetadata contains AMD Silogen model-specific metadata extracted from image labels.



_Appears in:_
- [ImageMetadata](#imagemetadata)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `canonicalName` _string_ | CanonicalName is the canonical model identifier (e.g., mistralai/Mixtral-8x22B-Instruct-v0.1).<br />Extracted from: org.amd.silogen.model.canonicalName |  |  |
| `source` _string_ | Source is the URL where the model can be found.<br />Extracted from: org.amd.silogen.model.source |  |  |
| `tags` _string array_ | Tags are descriptive tags (e.g., ["text-generation", "chat", "instruction"]).<br />Extracted from: org.amd.silogen.model.tags (comma-separated) |  |  |
| `versions` _string array_ | Versions lists available versions.<br />Extracted from: org.amd.silogen.model.versions (comma-separated) |  |  |
| `variants` _string array_ | Variants lists model variants.<br />Extracted from: org.amd.silogen.model.variants (comma-separated) |  |  |
| `hfTokenRequired` _boolean_ | HFTokenRequired indicates if a HuggingFace token is required.<br />Extracted from: org.amd.silogen.hfToken.required |  |  |
| `title` _string_ | Title is the Silogen-specific title for the model.<br />Extracted from: org.amd.silogen.title |  |  |
| `descriptionFull` _string_ | DescriptionFull is the full description.<br />Extracted from: org.amd.silogen.description.full |  |  |
| `releaseNotes` _string_ | ReleaseNotes contains release notes for this version.<br />Extracted from: org.amd.silogen.release.notes |  |  |
| `recommendedDeployments` _[RecommendedDeployment](#recommendeddeployment) array_ | RecommendedDeployments contains recommended deployment configurations.<br />Extracted from: org.amd.silogen.model.recommendedDeployments (parsed from JSON array) |  |  |


#### ModelSourceFilter



ModelSourceFilter defines a pattern for discovering images.
Supports multiple formats:
- Repository patterns: "org/repo*" - matches repositories with wildcards
- Repository with tag: "org/repo:1.0.0" - exact tag match
- Full URI: "ghcr.io/org/repo:1.0.0" - overrides registry and tag
- Full URI with wildcard: "ghcr.io/org/repo*" - overrides registry, matches pattern



_Appears in:_
- [AIMClusterModelSourceSpec](#aimclustermodelsourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Image pattern with wildcard and full URI support.<br />Supported formats:<br />- Repository pattern: "amdenterpriseai/aim-*"<br />- Repository with tag: "silogen/aim-llama:1.0.0" (overrides versions field)<br />- Full URI: "ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1" (overrides spec.registry and versions)<br />- Full URI with wildcard: "ghcr.io/silogen/aim-*" (overrides spec.registry)<br />When a full URI is specified (including registry like ghcr.io), only images from that<br />registry will match. When a tag is included, it takes precedence over the versions field.<br />Wildcard: * matches any sequence of characters. |  | MaxLength: 512 <br /> |
| `exclude` _string array_ | Exclude lists specific repository names to skip (exact match on repository name only, not registry).<br />Useful for excluding base images or experimental versions.<br />Examples:<br />- ["amdenterpriseai/aim-base", "amdenterpriseai/aim-experimental"]<br />- ["silogen/aim-base"] - works with "ghcr.io/silogen/aim-*" (registry is not checked in exclusion)<br />Note: Exclusions match against repository names (e.g., "silogen/aim-base"), not full URIs. |  |  |
| `versions` _string array_ | Versions specifies semantic version constraints for this filter.<br />If specified, overrides the global Versions field.<br />Only tags that parse as valid semver are considered (including prereleases like 0.8.1-rc1).<br />Ignored if the Image field includes an explicit tag (e.g., "repo:1.0.0").<br />Examples: ">=1.0.0", "<2.0.0", "~1.2.0" (patch updates), "^1.0.0" (minor updates)<br />Prerelease versions (e.g., 0.8.1-rc1) are supported and follow semver rules:<br />- 0.8.1-rc1 matches ">=0.8.0" (prerelease is part of version 0.8.1)<br />- Use ">=0.8.1-rc1" to match only that prerelease or higher<br />- Leave empty to match all tags (including prereleases and non-semver tags) |  |  |


#### OCIMetadata



OCIMetadata contains standard OCI image metadata extracted from image labels.



_Appears in:_
- [ImageMetadata](#imagemetadata)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `title` _string_ | Title is the human-readable title.<br />Extracted from: org.opencontainers.image.title |  |  |
| `description` _string_ | Description is a brief description.<br />Extracted from: org.opencontainers.image.description |  |  |
| `licenses` _string_ | Licenses is the SPDX license identifier(s).<br />Extracted from: org.opencontainers.image.licenses |  |  |
| `vendor` _string_ | Vendor is the organization that produced the image.<br />Extracted from: org.opencontainers.image.vendor |  |  |
| `authors` _string_ | Authors is contact details of the authors.<br />Extracted from: org.opencontainers.image.authors |  |  |
| `source` _string_ | Source is the URL to the source code repository.<br />Extracted from: org.opencontainers.image.source |  |  |
| `documentation` _string_ | Documentation is the URL to documentation.<br />Extracted from: org.opencontainers.image.documentation |  |  |
| `created` _string_ | Created is the creation timestamp.<br />Extracted from: org.opencontainers.image.created |  |  |
| `revision` _string_ | Revision is the source control revision.<br />Extracted from: org.opencontainers.image.revision |  |  |
| `version` _string_ | Version is the image version.<br />Extracted from: org.opencontainers.image.version |  |  |




#### RecommendedDeployment



RecommendedDeployment describes a recommended deployment configuration for a model.



_Appears in:_
- [ModelMetadata](#modelmetadata)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `gpuModel` _string_ | GPUModel is the GPU model name (e.g., MI300X, MI325X) |  |  |
| `gpuCount` _integer_ | GPUCount is the number of GPUs required |  |  |
| `precision` _string_ | Precision is the recommended precision (e.g., fp8, fp16, bf16) |  |  |
| `metric` _string_ | Metric is the optimization target (e.g., latency, throughput) |  |  |
| `description` _string_ | Description provides additional context about this deployment configuration |  |  |
| `profileId` _string_ | ProfileId is an optional override to select a particular AIM profile by ID |  |  |


#### StorageSpec



StorageSpec defines the persistent storage configuration



_Appears in:_
- [AIMKVCacheSpec](#aimkvcachespec)
- [AIMServiceKVCache](#aimservicekvcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | Size specifies the storage size for the persistent volume.<br />Minimum recommended size is 1Gi for Redis to function properly.<br />If not specified, defaults to 1Gi.<br />WARNING: This field is immutable after creation due to StatefulSet VolumeClaimTemplate limitations. | 1Gi |  |
| `storageClassName` _string_ | StorageClassName specifies the storage class to use for the persistent volume.<br />If not specified, the cluster's default storage class will be used.<br />Ensure your cluster has a default storage class configured or specify one explicitly.<br />WARNING: This field is immutable after creation due to StatefulSet VolumeClaimTemplate limitations. |  |  |
| `accessModes` _[PersistentVolumeAccessMode](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#persistentvolumeaccessmode-v1-core) array_ | AccessModes specifies the access modes for the persistent volume.<br />Defaults to ReadWriteOnce if not specified.<br />WARNING: This field is immutable after creation due to StatefulSet VolumeClaimTemplate limitations. | [ReadWriteOnce] |  |


