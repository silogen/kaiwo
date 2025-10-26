# API Reference

## Packages
- [aim.silogen.ai/v1alpha1](#aimsilogenaiv1alpha1)


## aim.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the AIM v1alpha1 API group.

### Resource Types
- [AIMClusterModel](#aimclustermodel)
- [AIMClusterModelList](#aimclustermodellist)
- [AIMClusterRuntimeConfig](#aimclusterruntimeconfig)
- [AIMClusterRuntimeConfigList](#aimclusterruntimeconfiglist)
- [AIMClusterServiceTemplate](#aimclusterservicetemplate)
- [AIMClusterServiceTemplateList](#aimclusterservicetemplatelist)
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
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |


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
| `aimImageName` _string_ | AIMModelName is the AIM image name. Matches `metadata.name` of an AIMModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template. | default |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default container resource requirements applied to services derived from this template.<br />Service-specific values override the template defaults. |  |  |




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
| `modelDownloadImage` _string_ | ModelDownloadImage is the image used to download the model | kserve/storage-initializer:v0.16.0-rc0 |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |


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

| Field | Description |
| --- | --- |
| `Pending` | AIMModelCacheStatusPending denotes that the model cache has not been created yet<br /> |
| `Progressing` | AIMModelCacheStatusProgressing denotes that the model cache is currently being filled<br /> |
| `Available` | AIMModelCacheStatusAvailable denotes that a model cache is filled and ready to be used<br /> |
| `Failed` | AIMModelCacheStatusFailed denotes that the model cache has failed. A more detailed reason will be available in the conditions.<br /> |


#### AIMModelDiscoverySpec



AIMModelDiscoverySpec configures metadata discovery and template generation for an image.



_Appears in:_
- [AIMModelSpec](#aimmodelspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled toggles metadata discovery for this image. Disabled by default. |  |  |
| `autoCreateTemplates` _boolean_ | AutoCreateTemplates controls whether recommended deployments from discovery<br />automatically create ServiceTemplates. Enabled by default when discovery runs. |  |  |


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
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)

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
| `image` _string_ | Image is the container image URI for this AIM model.<br />This image is inspected by the operator to select runtime profiles used by templates. |  | MinLength: 1 <br /> |
| `defaultServiceTemplate` _string_ | DefaultServiceTemplate is the default template to use for this image, if the user does not provide any |  |  |
| `discovery` _[AIMModelDiscoverySpec](#aimmodeldiscoveryspec)_ | Discovery controls metadata extraction and automatic template creation for this image. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this image. | default |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default resource requirements for services using this image.<br />Template- or service-level values override these defaults. |  |  |


#### AIMModelStatus



AIMModelStatus defines the observed state of AIMModel.



_Appears in:_
- [AIMClusterModel](#aimclustermodel)
- [AIMModel](#aimmodel)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller |  |  |
| `status` _[AIMModelStatusEnum](#aimmodelstatusenum)_ | Status represents the overall status of the image based on its templates | Pending | Enum: [Pending Progressing Ready Degraded Failed] <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the model's state |  |  |
| `resolvedRuntimeConfig` _[AIMResolvedRuntimeConfig](#aimresolvedruntimeconfig)_ | ResolvedRuntimeConfig captures metadata about the runtime config that was resolved. |  |  |
| `imageMetadata` _[ImageMetadata](#imagemetadata)_ | ImageMetadata is the metadata extracted from an AIM image |  |  |


#### AIMModelStatusEnum

_Underlying type:_ _string_

AIMModelStatusEnum represents the overall status of an AIMModel.

_Validation:_
- Enum: [Pending Progressing Ready Degraded Failed]

_Appears in:_
- [AIMModelStatus](#aimmodelstatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMModelStatusPending indicates the image has been created but template generation has not started.<br /> |
| `Progressing` | AIMModelStatusProgressing indicates one or more templates are still being discovered.<br /> |
| `Ready` | AIMModelStatusReady indicates all templates are available and ready.<br /> |
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

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `engine` _string_ | Engine identifies the inference engine used for this profile (e.g., "vllm", "tgi"). |  |  |
| `gpu` _string_ | GPU specifies the GPU model this profile is optimized for (e.g., "MI300X", "MI325X"). |  |  |
| `gpu_count` _integer_ | GPUCount indicates how many GPUs are required per replica for this profile. |  |  |
| `metric` _[AIMMetric](#aimmetric)_ | Metric indicates the optimization goal for this profile ("latency" or "throughput"). |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision specifies the numeric precision used in this profile (e.g., "fp16", "fp8"). |  | Enum: [bf16 fp16 fp8 int8] <br /> |


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

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultStorageClassName` _string_ | DefaultStorageClassName specifies the storage class to use for model caches and PVCs<br />when the consuming resource (AIMModelCache, AIMTemplateCache, AIMServiceTemplate) does not<br />specify a storage class. If this field is empty, the cluster's default storage class is used. |  |  |
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |


#### AIMRuntimeConfigCredentials



AIMRuntimeConfigCredentials captures namespace-scoped authentication configuration.
These fields are only available in namespace-scoped AIMRuntimeConfig resources.



_Appears in:_
- [AIMRuntimeConfigSpec](#aimruntimeconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for workloads<br />created by the operator, including:<br />- Discovery jobs that inspect container images<br />- Cache warmer jobs that download model artifacts<br />- Any other operator-managed pods in this namespace<br />If empty, the default service account for the namespace is used. |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets lists secrets containing credentials for pulling private container images.<br />These secrets are used when:<br />- Pulling AIM model container images for discovery<br />- Pulling runtime images for inference services<br />- Pulling utility images for cache warming<br />The secrets are merged with any cluster-level defaults configured on the operator.<br />Each secret must exist in the same namespace as this runtime config. |  |  |


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
| `routing` _[AIMRuntimeRoutingConfig](#aimruntimeroutingconfig)_ | Routing controls HTTP routing defaults applied to AIM resources.<br />When set, these defaults are used for AIMService resources that enable routing<br />but do not specify their own routing configuration. |  |  |
| `serviceAccountName` _string_ | ServiceAccountName specifies the Kubernetes service account to use for workloads<br />created by the operator, including:<br />- Discovery jobs that inspect container images<br />- Cache warmer jobs that download model artifacts<br />- Any other operator-managed pods in this namespace<br />If empty, the default service account for the namespace is used. |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets lists secrets containing credentials for pulling private container images.<br />These secrets are used when:<br />- Pulling AIM model container images for discovery<br />- Pulling runtime images for inference services<br />- Pulling utility images for cache warming<br />The secrets are merged with any cluster-level defaults configured on the operator.<br />Each secret must exist in the same namespace as this runtime config. |  |  |


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

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether HTTP routing is managed for inference services using this config.<br />When true, the operator creates HTTPRoute resources for services that reference this config.<br />When false or unset, routing must be explicitly enabled on each service.<br />This provides a namespace or cluster-wide default that individual services can override. |  |  |
| `gatewayRef` _[ParentReference](#parentreference)_ | GatewayRef specifies the Gateway API Gateway resource that should receive HTTPRoutes.<br />This identifies the parent gateway for routing traffic to inference services.<br />The gateway can be in any namespace (cross-namespace references are supported).<br />If routing is enabled but GatewayRef is not specified, service reconciliation will fail<br />with a validation error. |  |  |
| `pathTemplate` _string_ | PathTemplate defines the HTTP path template for routes, evaluated using JSONPath expressions.<br />The template is rendered against the AIMService object to generate unique paths.<br />Example templates:<br />- `/\{.metadata.namespace\}/\{.metadata.name\}` - namespace and service name<br />- `/\{.metadata.namespace\}/\{.metadata.labels['team']\}/inference` - with label<br />- `/models/\{.spec.aimImageName\}` - based on model name<br />The template must:<br />- Use valid JSONPath expressions wrapped in \{...\}<br />- Reference fields that exist on the service<br />- Produce a path ≤ 200 characters after rendering<br />- Result in valid URL path segments (lowercase, RFC 1123 compliant)<br />If evaluation fails, the service enters Degraded state with PathTemplateInvalid reason.<br />Individual services can override this template via spec.routing.pathTemplate. |  |  |


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


#### AIMServiceList



AIMServiceList contains a list of AIMService.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMServiceList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMService](#aimservice) array_ |  |  |  |


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


#### AIMServiceRouting



AIMServiceRouting configures optional HTTP routing for the service.



_Appears in:_
- [AIMServiceSpec](#aimservicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled toggles HTTP routing management. | false |  |
| `gatewayRef` _[ParentReference](#parentreference)_ | GatewayRef identifies the Gateway parent that should receive the HTTPRoute.<br />When omitted while routing is enabled, reconciliation will report a failure. |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the HTTPRoute resource. |  |  |
| `pathTemplate` _string_ | PathTemplate overrides the HTTP path template used for routing.<br />The value is rendered against the AIMService object using JSONPath expressions. |  |  |


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
| `aimImageName` _string_ | AIMModelName is the canonical model name (including version/revision) to deploy.<br />Expected to match the `spec.metadata.name` of an AIMModel. Example:<br />`meta-llama-3-8b-1-1-20240915`. |  | MinLength: 1 <br /> |
| `templateRef` _string_ | TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to use.<br />The template selects the runtime profile and GPU parameters. |  |  |
| `cacheModel` _boolean_ | CacheModel requests that model sources be cached when starting the service<br />if the template itself does not warm the cache.<br />When `warmCache: false` on the template, this setting ensures caching is<br />performed before the service becomes ready. | false |  |
| `replicas` _integer_ | Replicas overrides the number of replicas for this service.<br />Other runtime settings remain governed by the template unless overridden. | 1 |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this service. | default |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources overrides the container resource requirements for this service.<br />When specified, these values take precedence over the template and image defaults. |  |  |
| `overrides` _[AIMServiceOverrides](#aimserviceoverrides)_ | Overrides allows overriding specific template parameters for this service.<br />When specified, these values take precedence over the template values. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |
| `routing` _[AIMServiceRouting](#aimservicerouting)_ | Routing enables HTTP routing through Gateway API for this service. |  |  |


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
| `aimImageName` _string_ | AIMModelName is the AIM image name. Matches `metadata.name` of an AIMModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template. | default |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default container resource requirements applied to services derived from this template.<br />Service-specific values override the template defaults. |  |  |
| `caching` _[AIMTemplateCachingConfig](#aimtemplatecachingconfig)_ | Caching configures model caching behavior for this namespace-scoped template.<br />When enabled, models will be cached using the specified environment variables<br />during download. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |


#### AIMServiceTemplateSpecCommon



AIMServiceTemplateSpecCommon contains the shared fields for both cluster-scoped
and namespace-scoped service templates.



_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `aimImageName` _string_ | AIMModelName is the AIM image name. Matches `metadata.name` of an AIMModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AIMGpuSelector](#aimgpuselector)_ | GpuSelector specifies GPU requirements for each replica.<br />Defines the GPU count and model type required for deployment.<br />This field is immutable after creation. |  |  |
| `runtimeConfigName` _string_ | RuntimeConfigName references the AIM runtime configuration (by name) to use for this template. | default |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core)_ | Resources defines the default container resource requirements applied to services derived from this template.<br />Service-specific values override the template defaults. |  |  |


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
| `resolvedImage` _[AIMResolvedReference](#aimresolvedreference)_ | ResolvedImage captures metadata about the image that was resolved. |  |  |
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
| `storageClassName` _string_ | StorageClassName is the name for the storage class to use for this cache |  |  |
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


