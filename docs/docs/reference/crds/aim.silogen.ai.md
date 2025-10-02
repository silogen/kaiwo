# API Reference

## Packages
- [aim.silogen.ai/v1alpha1](#aimsilogenaiv1alpha1)


## aim.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the AIM v1alpha1 API group.

### Resource Types
- [AIMClusterConfig](#aimclusterconfig)
- [AIMClusterServiceTemplate](#aimclusterservicetemplate)
- [AIMClusterServiceTemplateList](#aimclusterservicetemplatelist)
- [AIMConfigList](#aimconfiglist)
- [AIMImage](#aimimage)
- [AIMImageList](#aimimagelist)
- [AIMService](#aimservice)
- [AIMServiceList](#aimservicelist)
- [AIMServiceTemplate](#aimservicetemplate)
- [AIMServiceTemplateList](#aimservicetemplatelist)
- [AIMTemplateCache](#aimtemplatecache)
- [AIMTemplateCacheList](#aimtemplatecachelist)
- [ModelCache](#modelcache)
- [ModelCacheList](#modelcachelist)





#### AIMClusterConfig



AIMClusterConfig configures credentials and routing at a cluster level.



_Appears in:_
- [AIMConfigList](#aimconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMConfigSpec](#aimconfigspec)_ |  |  |  |


#### AIMClusterModelSpec



AIMClusterModelSpec defines the desired state of AIMImage.



_Appears in:_
- [AIMImage](#aimimage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `modelId` _string_ | ModelID is the ID name (includes version/revision).<br />Example: `meta/llama-3-8b:1.1+20240915`. |  | MinLength: 1 <br /> |
| `image` _string_ | Image is the container image URI for this AIM model.<br />This image is inspected by the operator to select runtime profiles used by templates. |  | MinLength: 1 <br /> |
| `defaultServiceTemplate` _string_ | DefaultServiceTemplate is the name of the default service template to use, if an<br />AIMService is created without specifying a template name. |  |  |


#### AIMClusterModelStatus



AIMClusterModelStatus defines the observed state of AIMImage.



_Appears in:_
- [AIMImage](#aimimage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the model's state |  |  |


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
| `modelId` _string_ | ModelID is the AIM model ID. Matches `spec.name` of an AIMImage. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AimGpuSelector](#aimgpuselector)_ | AimGpuSelector contains the strategy to choose the resources to give each replica |  |  |


#### AIMConfigList



AIMConfigList contains a list of AIMClusterConfig.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMClusterConfig](#aimclusterconfig) array_ |  |  |  |


#### AIMConfigSpec



AIMConfigSpec defines the desired configuration for an AIM-enabled namespace.



_Appears in:_
- [AIMClusterConfig](#aimclusterconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `routing` _[AIMRoutingConfig](#aimroutingconfig)_ | Routing controls automatic HTTPRoute creation for AIM services in this namespace.<br />When enabled (default), the operator creates one HTTPRoute per service using<br />path-based routing with the pattern `/<namespace>/<workload_id>/` and attaches<br />it to the referenced Gateway listener. |  |  |
| `cacheStorageClassName` _string_ | CacheStorageClassName is the name of the storage class to use for cached models |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets are a list of secrets that will be merged with any existing ones when referencing the AIM containers |  |  |


#### AIMImage



AIMImage is the Schema for cluster-scoped AIM model catalog entries.



_Appears in:_
- [AIMImageList](#aimimagelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMImage` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMClusterModelSpec](#aimclustermodelspec)_ |  |  |  |
| `status` _[AIMClusterModelStatus](#aimclustermodelstatus)_ |  |  |  |


#### AIMImageList



AIMImageList contains a list of AIMImage.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMImageList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMImage](#aimimage) array_ |  |  |  |


#### AIMMetric

_Underlying type:_ _string_

AIMMetric enumerates the targeted service characteristic

_Validation:_
- Enum: [latency throughput]

_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMRuntimeParameters](#aimruntimeparameters)
- [AIMServiceOverrides](#aimserviceoverrides)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)

| Field | Description |
| --- | --- |
| `latency` |  |
| `throughput` |  |


#### AIMModelSource







_Appears in:_
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceUri` _string_ | SourceURI is the source where the model should be downloaded from |  |  |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | Size is the amount of storage that the source expects |  |  |


#### AIMPrecision

_Underlying type:_ _string_

AIMPrecision enumerates supported numeric precisions

_Validation:_
- Enum: [bf16 fp16 fp8 int8]

_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
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


#### AIMRoutingConfig



AIMRoutingConfig controls automatic HTTPRoute provisioning in the namespace.



_Appears in:_
- [AIMConfigSpec](#aimconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `gateway` _[ParentReference](#parentreference)_ | Gateway references the Gateway listener to use for exposure and routing<br />(mirrors HTTPRoute.parentRefs[*]). |  |  |
| `autoCreateRoute` _boolean_ | AutoCreateRoute enables automatic HTTPRoute creation for AIM services. | true |  |


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
| `gpuSelector` _[AimGpuSelector](#aimgpuselector)_ | AimGpuSelector contains the strategy to choose the resources to give each replica |  |  |


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
| `gpuSelector` _[AimGpuSelector](#aimgpuselector)_ | AimGpuSelector contains the strategy to choose the resources to give each replica |  |  |


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
| `aimModelId` _string_ | AIMModelID is the canonical model name (including version/revision) to deploy.<br />Expected to match the `spec.name` of an AIMImage. Example:<br />`meta/llama-3-8b:1.1+20240915`. |  | MinLength: 1 <br /> |
| `templateRef` _string_ | TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to use.<br />The template selects the runtime profile and GPU parameters. |  |  |
| `cacheModel` _boolean_ | CacheModel requests that model sources be cached when starting the service<br />if the template itself does not warm the cache.<br />When `warmCache: false` on the template, this setting ensures caching is<br />performed before the service becomes ready. | false |  |
| `replicas` _integer_ | Replicas overrides the number of replicas for this service.<br />Other runtime settings remain governed by the template unless overridden. | 1 |  |
| `configRef` _string_ | ConfigRef selects the cluster-scoped AIMClusterConfig (by name) to use for this service. | default |  |
| `overrides` _[AIMServiceOverrides](#aimserviceoverrides)_ | Overrides allows overriding specific template parameters for this service.<br />When specified, these values take precedence over the template values. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env specifies environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |


#### AIMServiceStatus



AIMServiceStatus defines the observed state of AIMService.



_Appears in:_
- [AIMService](#aimservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of template state. |  |  |
| `status` _[AIMServiceStatusEnum](#aimservicestatusenum)_ | Status represents the current high‑level status of the service lifecycle.<br />Values: `Pending`, `Starting`, `Running`, `Failed`, `Degraded`. | Pending | Enum: [Pending Starting Running Failed Degraded] <br /> |


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
| `modelId` _string_ | ModelID is the AIM model ID. Matches `spec.name` of an AIMImage. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AimGpuSelector](#aimgpuselector)_ | AimGpuSelector contains the strategy to choose the resources to give each replica |  |  |
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
| `modelId` _string_ | ModelID is the AIM model ID. Matches `spec.name` of an AIMImage. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. |  | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpuSelector` _[AimGpuSelector](#aimgpuselector)_ | AimGpuSelector contains the strategy to choose the resources to give each replica |  |  |


#### AIMServiceTemplateStatus



AIMServiceTemplateStatus defines the observed state of AIMServiceTemplate.



_Appears in:_
- [AIMClusterServiceTemplate](#aimclusterservicetemplate)
- [AIMServiceTemplate](#aimservicetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of template state. |  |  |
| `status` _[AIMTemplateStatusEnum](#aimtemplatestatusenum)_ | Status represents the current high‑level status of the template lifecycle.<br />Values: `Pending`, `Progressing`, `Available`, `Failed`. | Pending | Enum: [Pending Progressing Available Failed] <br /> |
| `modelSources` _[AIMModelSource](#aimmodelsource) array_ | ModelSources list the models that this template requires to run. These are the models that will be<br />cached, if this template is cached. |  |  |


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


#### AIMTemplateCacheStatus



AIMTemplateCacheStatus defines the observed state of AIMTemplateCache



_Appears in:_
- [AIMTemplateCache](#aimtemplatecache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of the template cache state. |  |  |
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
- Enum: [Pending Progressing Available Failed]

_Appears in:_
- [AIMServiceTemplateStatus](#aimservicetemplatestatus)

| Field | Description |
| --- | --- |
| `Pending` | AIMTemplateStatusPending denotes that the template has been created and discovery has not yet started.<br /> |
| `Progressing` | AIMTemplateStatusProgressing denotes that discovery and/or cache warm is in progress.<br /> |
| `Available` | AIMTemplateStatusAvailable denotes that discovery succeeded and, if requested, caches are warmed.<br /> |
| `Degraded` | AIMTemplateStatusDegraded denotes that the template is non-functional for some reason, for example that the cluster doesn't have the resources specified.<br /> |
| `Failed` | AIMTemplateStatusFailed denotes a terminal failure for discovery or warm operations.<br /> |


#### AimGpuSelector







_Appears in:_
- [AIMClusterServiceTemplateSpec](#aimclusterservicetemplatespec)
- [AIMRuntimeParameters](#aimruntimeparameters)
- [AIMServiceOverrides](#aimserviceoverrides)
- [AIMServiceTemplateSpec](#aimservicetemplatespec)
- [AIMServiceTemplateSpecCommon](#aimservicetemplatespeccommon)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `count` _integer_ | Count is the number of the GPU resources requested per replica |  | Minimum: 1 <br /> |
| `model` _string_ | Model is the model name of the GPU that is supported by this template |  | MinLength: 1 <br /> |


#### ModelCache



ModelCache is the Schema for the modelcaches API



_Appears in:_
- [ModelCacheList](#modelcachelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `ModelCache` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ModelCacheSpec](#modelcachespec)_ |  |  |  |
| `status` _[ModelCacheStatus](#modelcachestatus)_ |  |  |  |


#### ModelCacheList



ModelCacheList contains a list of ModelCache





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `ModelCacheList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[ModelCache](#modelcache) array_ |  |  |  |


#### ModelCacheSpec



ModelCacheSpec defines the desired state of ModelCache



_Appears in:_
- [ModelCache](#modelcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceUri` _string_ | SourceURI is the source of the model to be downloaded. This is the only<br />identifier |  | MinLength: 1 <br />Pattern: `^(hf\|s3)://[^ \t\r\n]+$` <br /> |
| `storageClassName` _string_ | StorageClassName specifies the storage class for the cache volume |  |  |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | Size specifies the size of the cache volume |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env lists the environment variables to use for authentication when downloading models.<br />These variables are used for authentication with model registries (e.g., HuggingFace tokens). |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core) array_ | ImagePullSecrets references secrets for pulling AIM container images. |  |  |


#### ModelCacheStatus



ModelCacheStatus defines the observed state of ModelCache



_Appears in:_
- [ModelCache](#modelcache)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the model cache's state |  |  |
| `status` _[ModelCacheStatusEnum](#modelcachestatusenum)_ | Status represents the current status of the model cache | Pending | Enum: [Pending Progressing Available Failed] <br /> |
| `lastUsed` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | LastUsed represents the last time a model was deployed that used this cache |  |  |
| `persistentVolumeClaim` _string_ | PersistentVolumeClaim represents the name of the created PVC |  |  |


#### ModelCacheStatusEnum

_Underlying type:_ _string_



_Validation:_
- Enum: [Pending Progressing Available Failed]

_Appears in:_
- [ModelCacheStatus](#modelcachestatus)

| Field | Description |
| --- | --- |
| `Pending` | ModelCacheStatusPending denotes that the model cache has not been created yet<br /> |
| `Progressing` | ModelCacheStatusProgressing denotes that the model cache is currently being filled<br /> |
| `Available` | ModelCacheStatusAvailable denotes that a model cache is filled and ready to be used<br /> |
| `Failed` | ModelCacheStatusFailed denotes that the model cache has failed. A more detailed reason will be available in the conditions.<br /> |


