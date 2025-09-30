# API Reference

## Packages
- [aim.silogen.ai/v1alpha1](#aimsilogenaiv1alpha1)


## aim.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the AIM v1alpha1 API group.

### Resource Types
- [AIMClusterModel](#aimclustermodel)
- [AIMClusterModelList](#aimclustermodellist)
- [AIMNamespaceConfig](#aimnamespaceconfig)
- [AIMNamespaceConfigList](#aimnamespaceconfiglist)
- [AIMService](#aimservice)
- [AIMServiceList](#aimservicelist)
- [AIMServiceTemplate](#aimservicetemplate)
- [AIMServiceTemplateList](#aimservicetemplatelist)
- [ModelCache](#modelcache)
- [ModelCacheList](#modelcachelist)



#### AIMClusterModel



AIMClusterModel is the Schema for cluster-scoped AIM model catalog entries.



_Appears in:_
- [AIMClusterModelList](#aimclustermodellist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterModel` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMClusterModelSpec](#aimclustermodelspec)_ |  |  |  |
| `status` _[AIMClusterModelStatus](#aimclustermodelstatus)_ |  |  |  |


#### AIMClusterModelList



AIMClusterModelList contains a list of AIMClusterModel.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMClusterModelList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMClusterModel](#aimclustermodel) array_ |  |  |  |


#### AIMClusterModelSpec



AIMClusterModelSpec defines the desired state of AIMClusterModel.



_Appears in:_
- [AIMClusterModel](#aimclustermodel)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the canonical model name (includes version/revision).<br />Example: `meta/llama-3-8b:1.1+20240915`. |  | MinLength: 1 <br /> |
| `image` _string_ | Image is the container image URI for this AIM model.<br />This image is inspected by the operator to select runtime profiles used by templates. |  | MinLength: 1 <br /> |


#### AIMClusterModelStatus



AIMClusterModelStatus defines the observed state of AIMClusterModel.



_Appears in:_
- [AIMClusterModel](#aimclustermodel)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the model's state |  |  |


#### AIMMetric

_Underlying type:_ _string_

AIMMetric enumerates the targeted service characteristic

_Validation:_
- Enum: [latency throughput]

_Appears in:_
- [AIMServiceTemplateSpec](#aimservicetemplatespec)

| Field | Description |
| --- | --- |
| `latency` |  |
| `throughput` |  |


#### AIMNamespaceConfig



AIMNamespaceConfig configures credentials and routing for a namespace.
It is namespaced and typically created by a tenant administrator.



_Appears in:_
- [AIMNamespaceConfigList](#aimnamespaceconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMNamespaceConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AIMNamespaceConfigSpec](#aimnamespaceconfigspec)_ |  |  |  |


#### AIMNamespaceConfigList



AIMNamespaceConfigList contains a list of AIMNamespaceConfig.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aim.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `AIMNamespaceConfigList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AIMNamespaceConfig](#aimnamespaceconfig) array_ |  |  |  |


#### AIMNamespaceConfigSpec



AIMNamespaceConfigSpec defines the desired configuration for an AIM-enabled namespace.



_Appears in:_
- [AIMNamespaceConfig](#aimnamespaceconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `credentials` _[AIMNamespaceCredentials](#aimnamespacecredentials)_ | Credentials provide secret references used during model access and caching. |  |  |
| `images` _[AIMNamespaceImageConfig](#aimnamespaceimageconfig)_ | Images configures image pull behavior for AIM images within this namespace. |  |  |
| `routing` _[AIMNamespaceRoutingConfig](#aimnamespaceroutingconfig)_ | Routing controls automatic HTTPRoute creation for AIM services in this namespace.<br />When enabled (default), the operator creates one HTTPRoute per service using<br />path-based routing with the pattern `/<namespace>/<workload_id>/` and attaches<br />it to the referenced Gateway listener. |  |  |
| `cacheStorageClassName` _string_ | CacheStorageClassName is the name of the storage class to use for cached models |  |  |


#### AIMNamespaceCredentials



AIMNamespaceCredentials gathers optional credentials for model discovery and caching.
One or more providers can be configured.



_Appears in:_
- [AIMNamespaceConfigSpec](#aimnamespaceconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `huggingFaceToken` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#secretkeyselector-v1-core)_ | HuggingFaceToken is the token used to access Hugging Face repositories. |  |  |
| `s3` _[AIMS3Credential](#aims3credential) array_ | S3 contains one or more S3 credential configurations. |  |  |


#### AIMNamespaceImageConfig



AIMNamespaceImageConfig defines image pull configuration for AIM workloads in the namespace.

Configure one or more image pull secrets to access private registries
hosting AIM images referenced by AIMClusterModel entries.

Example:

```yaml
imagePullSecrets:
  - my-regcred
  - another-secret

```



_Appears in:_
- [AIMNamespaceConfigSpec](#aimnamespaceconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imagePullSecrets` _string array_ | ImagePullSecrets is the list of Secret names to use for pulling AIM images. |  |  |


#### AIMNamespaceRoutingConfig



AIMNamespaceRoutingConfig controls automatic HTTPRoute provisioning in the namespace.



_Appears in:_
- [AIMNamespaceConfigSpec](#aimnamespaceconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `gateway` _[ParentReference](#parentreference)_ | Gateway references the Gateway listener to use for exposure and routing<br />(mirrors HTTPRoute.parentRefs[*]). |  |  |
| `autoCreateRoute` _boolean_ | AutoCreateRoute enables automatic HTTPRoute creation for AIM services. | true |  |


#### AIMPrecision

_Underlying type:_ _string_

AIMPrecision enumerates supported numeric precisions

_Validation:_
- Enum: [bf16 fp16 fp8 int8]

_Appears in:_
- [AIMServiceTemplateSpec](#aimservicetemplatespec)

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


#### AIMS3Credential



AIMS3Credential configures S3 credential references for a specific endpoint.
Provide an endpoint and secret references for authentication.



_Appears in:_
- [AIMNamespaceCredentials](#aimnamespacecredentials)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpointUrl` _string_ | EndpointURL (S3-compatible). If both Region and EndpointURL are set, EndpointURL wins. |  |  |
| `region` _string_ | Region (AWS S3). If both Region and EndpointURL are set, EndpointURL wins. |  |  |
| `accessKeyId` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#secretkeyselector-v1-core)_ |  |  |  |
| `secretAccessKey` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#secretkeyselector-v1-core)_ |  |  |  |


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


#### AIMServiceSpec



AIMServiceSpec defines the desired state of AIMService.

Binds a canonical model to an AIMServiceTemplate and configures replicas and
caching behavior. The template governs the runtime selection knobs; only the
number of replicas is overrideable here.



_Appears in:_
- [AIMService](#aimservice)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `model` _string_ | Model is the canonical model name (including version/revision) to deploy.<br />Expected to match the `spec.name` of an AIMClusterModel. Example:<br />`meta/llama-3-8b:1.1+20240915`. |  | MinLength: 1 <br /> |
| `templateRef` _string_ | TemplateRef is the name of the AIMServiceTemplate (same namespace) to use.<br />The template selects the runtime profile and GPU parameters. |  | MinLength: 1 <br /> |
| `cacheModel` _boolean_ | CacheModel requests that model sources be cached when starting the service<br />if the template itself does not warm the cache. Defaults to `true`.<br />When `warmCache: false` on the template, this setting ensures caching is<br />performed before the service becomes ready. | true |  |
| `replicas` _integer_ | Replicas overrides the number of replicas for this service.<br />Other runtime settings remain governed by the template. | 1 |  |
| `configRef` _string_ | ConfigRef selects the AIMNamespaceConfig (by name) to use for this service.<br />The default value is "default". The referenced AIMNamespaceConfig must<br />reside in the same namespace as the service. | default |  |


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



AIMServiceTemplateSpec defines the desired state of AIMServiceTemplate.

A namespaced and versioned template that selects a runtime profile
for a given AIM model (by canonical name). Templates are intentionally
narrow: they describe runtime selection knobs for the AIM container and do
not redefine the full Kubernetes deployment shape.



_Appears in:_
- [AIMServiceTemplate](#aimservicetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `model` _string_ | Model is the canonical model name (exact string match), including version/revision.<br />Matches `spec.name` of an AIMClusterModel. Immutable.<br />Example: `meta/llama-3-8b:1.1+20240915` |  | MinLength: 1 <br /> |
| `metric` _[AIMMetric](#aimmetric)_ | Metric selects the optimization goal. Immutable.<br />- `latency`: prioritize low end‑to‑end latency<br />- `throughput`: prioritize sustained requests/second |  | Enum: [latency throughput] <br /> |
| `precision` _[AIMPrecision](#aimprecision)_ | Precision selects the numeric precision used by the runtime. Immutable. | auto | Enum: [auto fp4 fp8 fp16 fp32 bf16 int4 int8] <br /> |
| `gpusPerReplica` _integer_ | GpusPerReplica is the total number of GPUs for a single model replica. Immutable. |  | Minimum: 1 <br /> |
| `gpuModel` _string_ | GpuModel is the physical GPU card model targeted by this template. Immutable. |  | MinLength: 1 <br /> |
| `tensorParallelism` _integer_ | TensorParallelism is the tensor parallel degree expected by the runtime. Immutable. |  | Minimum: 1 <br /> |
| `warmCache` _boolean_ | WarmCache requests immediate model cache warming in this namespace after profile discovery.<br />Defaults to `false`.<br />When left `false`, services can still request caching via `AIMService.spec.cacheModel: true`. | false |  |


#### AIMServiceTemplateStatus



AIMServiceTemplateStatus defines the observed state of AIMServiceTemplate.



_Appears in:_
- [AIMServiceTemplate](#aimservicetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest observations of template state. |  |  |
| `status` _[AIMTemplateStatusEnum](#aimtemplatestatusenum)_ | Status represents the current high‑level status of the template lifecycle.<br />Values: `Pending`, `Progressing`, `Available`, `Failed`. | Pending | Enum: [Pending Progressing Available Failed] <br /> |
| `modelSources` _string array_ | ModelSources list the models that this template requires to run. These are the models that will be<br />cached, if this template is cached. |  |  |


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
| `Failed` | AIMTemplateStatusFailed denotes a terminal failure for discovery or warm operations.<br /> |


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
| `sourceUri` _string_ | SourceURI is the source of the model to be downloaded. This is the only<br />identifier |  | MinLength: 1 <br /> |
| `storageClassName` _string_ | StorageClassName specifies the storage class for the cache volume |  |  |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api)_ | Size specifies the size of the cache volume |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#envvar-v1-core) array_ | Env lists the environment variables required to download the model into the cache |  |  |


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


