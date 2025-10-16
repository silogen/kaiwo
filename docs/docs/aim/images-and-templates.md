# AIM Images and Service Templates

This document explains AIM Images and Service Templates, the foundational resources that define what container images to use for models and how to configure their runtime behavior. Understanding these resources is essential for both administrators who manage cluster-wide catalogs and ML engineers who deploy inference services.

Images serve as a catalog that maps model identifiers to container images, providing version control for model deployments. Templates define runtime configurations and act as a discovery cache. Once created, the operator inspects the container to determine which model artifacts are required, storing this information for reuse by services and caching mechanisms. Together, these resources enable consistent, reproducible deployments across your cluster.

## AIM Images

AIM Images form a catalog that maps logical model identifiers to specific container images. This mapping serves two purposes: it acts as a registry that translates abstract model references into concrete container images, and it provides version control by allowing you to update which container image serves a particular model without changing service configurations.

### Cluster vs Namespace Scope

AIM provides two image resource types to accommodate different organizational needs and deployment patterns.

**AIMClusterImage** resources are cluster-scoped and typically installed by cluster administrators. These resources usually come from external sources through GitOps workflows or Helm installations, representing curated model catalogs maintained by platform teams or model publishers. Cluster images provide a consistent baseline across all namespaces, ensuring that teams share common model definitions unless explicitly overridden.

**AIMImage** resources are namespace-scoped and allow individual teams to define namespace-specific model variants or override cluster-level definitions. These are less commonly used but valuable when a team needs to test a different version of a model image or provide team-specific model access. When both cluster and namespace images exist for the same model ID, the namespace-scoped resource takes precedence within that namespace.

### Anatomy of an Image

An AIM Image uses its `metadata.name` as the canonical model identifier. The spec contains:

- **`image`**: the container image URI that implements this model. The operator inspects the image during discovery to determine runtime requirements and model sources.
- **`defaultServiceTemplate`**: an optional hint pointing to the template that should be used when services do not specify one.
- **`runtimeConfigName`**: the runtime configuration that supplies registry credentials or storage defaults. When omitted, the operator resolves the `default` runtime config.

### Examples

Here is a cluster-scoped image that makes a Llama 3.1 8B model available across the entire cluster:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterImage
metadata:
  name: meta-llama-3-8b-instruct
spec:
  image: ghcr.io/example/llama-3.1-8b-instruct:v1.2.0
  defaultServiceTemplate: llama-3-8b-latency
  runtimeConfigName: platform-default
  resources:
    limits:
      cpu: "8"
      memory: 64Gi
    requests:
      cpu: "4"
      memory: 32Gi
```

Namespace-scoped images follow the same structure but include a namespace and allow teams to override cluster defaults:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMImage
metadata:
  name: meta-llama-3-8b-instruct-dev
  namespace: ml-team
spec:
  image: ghcr.io/ml-team/llama-3.1-8b-instruct:dev-latest
  defaultServiceTemplate: llama-3-8b-experimental
  runtimeConfigName: ml-team
  resources:
    limits:
      cpu: "8"
      memory: 48Gi
    requests:
      cpu: "4"
      memory: 24Gi
```

### Behind the Scenes

When templates or services reference a model ID, the operator looks up the corresponding image from the catalog. For namespace-scoped lookups, the operator checks for an `AIMImage` in the same namespace first, then falls back to cluster-scoped `AIMClusterImage` resources. The resolved image contributes both the container reference and its default `resources`. During reconciliation the controller merges resources in this order:

1. `AIMService.spec.resources` (service-level override).
2. `AIMServiceTemplate.spec.resources` (template default).
3. `AIMImage.spec.resources` (catalog default).

If GPU quantities are still unset after the merge, the controller copies them from discovery metadata recorded on the template.

## Service Templates

Service Templates define runtime configurations for models, specifying parameters like optimization goals, numeric precision, and GPU requirements. Beyond configuration, templates serve a critical function as a discovery cache: when a template is created, the operator runs a discovery process that determines which model artifacts must be downloaded, storing this information in the template's status. Services and caching mechanisms reuse this discovered information, avoiding repeated discovery operations.

### The Template as Cache

The discovery cache function is central to how AIM manages model artifacts. When you create a template, the operator launches a short-lived discovery job using the container image referenced by the template's model ID. This job runs in dry-run mode to inspect the container's requirements without actually downloading large model files. The discovered model sources—including download URIs and sizes—are written to the template's `status.modelSources[]` field.

This cached discovery information is then reused by multiple downstream consumers. When services deploy using the template, they reference the discovered model sources for caching decisions. When template caches are created to pre-warm model downloads, they use the same model source list. This approach ensures consistency and prevents redundant discovery operations across multiple services using the same template configuration.

### Cluster vs Namespace - When to Use

Understanding when to use cluster-scoped versus namespace-scoped templates is essential for organizing your deployment workflow.

**AIMClusterServiceTemplate** resources are cluster-scoped and typically come from external sources installed by administrators. These templates arrive through GitOps workflows, Helm installations, or operator bundles, providing baseline runtime profiles that work across the cluster. Cluster templates serve as reference configurations maintained by platform teams or model publishers. They cannot enable caching themselves, as caching is a namespace-level concern. However, it is possible to cache a cluster-level template into a specific namespace by using a TemplateCache object. Discovery for cluster templates runs in the operator's system namespace.

**AIMServiceTemplate** resources are namespace-scoped and owned by ML engineers and data scientists. These templates allow teams to create custom runtime profiles tailored to their specific use cases. Namespace templates can enable model caching through the `caching.enabled` field, allowing teams to pre-warm model downloads for faster service startup. Discovery for namespace templates runs in the same namespace as the template. Teams typically create namespace templates when they need configurations different from cluster baselines or when they want to enable caching without explicitly creating TemplateCache resources.

The decision between cluster and namespace templates often follows this pattern: cluster administrators install cluster templates as part of model catalog bundles, providing well-tested baseline configurations. Users then either reference these cluster templates directly from services or create namespace templates when they need customization or caching capabilities.

### Anatomy of a Template

Templates contain both common fields shared between cluster and namespace scopes, and namespace-specific fields that control caching and authentication.

The `aimImageName` field identifies which image catalog entry this template configures. This field is immutable after creation to maintain consistency with discovered model sources.

The `metric` field selects the optimization goal for the runtime. Setting this to `latency` optimizes for low end-to-end latency in interactive scenarios, while `throughput` optimizes for sustained requests per second in batch processing workloads. This choice influences how the runtime allocates resources and schedules work.

The `precision` field determines the numeric precision used during inference. Options include `fp16`, `fp8`, `bf16`, `int8`, and `auto`. Lower precision values like `fp8` reduce memory usage and increase throughput at the cost of potential accuracy loss. The `auto` setting lets the runtime select appropriate precision based on hardware capabilities and model requirements.

The `gpuSelector` object specifies GPU requirements through two fields: `count` indicates how many GPUs each replica needs, and `model` names the specific GPU type required, such as `MI300X` or `MI325X`. This selector ensures services deploy only on nodes with appropriate hardware.

Namespace-scoped templates include additional fields for caching and authentication. The `caching` object controls whether the operator pre-warms model downloads for this template. Setting `caching.enabled` to `true` causes the operator to create an `AIMTemplateCache` resource that downloads model artifacts referenced in `status.modelSources[]` before services start. The `env` field provides environment variables for model downloads, typically used for authentication tokens. The `imagePullSecrets` field references Kubernetes secrets needed to pull private container images.

### Discovery Process

When a template is created or updated, the operator initiates a discovery process to determine model requirements. The operator creates a Kubernetes Job that runs the container image in dry-run mode, examining the runtime's model requirements without downloading full model files. For cluster templates, this job runs in the operator's system namespace. For namespace templates, the job runs in the template's namespace, allowing it to access namespace-specific secrets and configurations.

The discovery job completes quickly, typically within seconds. Upon successful completion, the operator parses the job output and extracts model source information including download URIs and expected sizes. This information is written to the template's `status.modelSources[]` field, where it remains available for services and caching mechanisms to reference. If discovery fails, the template's status reflects the failure through appropriate conditions, and services depending on this template will not proceed until discovery succeeds.

### Auto-Creation from Service Overrides

Users creating AIM Services may specify runtime overrides without explicitly creating a template. When this occurs, the operator automatically creates a namespace-scoped template incorporating the specified overrides. This auto-created template goes through the same discovery process and caching as manually created templates.

For example, if a service specifies `metric: throughput` and `gpuSelector.count: 2` as overrides while referencing a base template with different values, the operator creates a new namespace template with these override values applied. The service then references this auto-created template. This behavior allows users to customize runtime behavior without manually managing template resources, while still benefiting from discovery caching and consistent configuration.

### Examples

A cluster-scoped template provides a baseline latency-optimized configuration:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterServiceTemplate
metadata:
  name: llama-3-8b-latency
spec:
  aimImageName: meta-llama-3-8b-instruct
  runtimeConfigName: platform-default
  metric: latency
  precision: fp16
  gpuSelector:
    count: 1
    model: MI300X
```

A namespace-scoped template with caching enabled allows teams to pre-warm model downloads:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama-3-8b-throughput
  namespace: ml-research
spec:
  aimImageName: meta-llama-3-8b-instruct-dev
  runtimeConfigName: ml-research
  metric: throughput
  precision: fp8
  gpuSelector:
    count: 2
    model: MI300X
  caching:
    enabled: true
  env:
    - name: HF_TOKEN
      valueFrom:
        secretKeyRef:
          name: huggingface-creds
          key: token
  imagePullSecrets:
    - name: registry-credentials
```

### Status

Template status tracks the discovery lifecycle and provides information about discovered model sources:

| Field | Type | Description |
|-------|------|-------------|
| `observedGeneration` | int64 | Most recent generation observed by the controller |
| `status` | enum | High-level lifecycle state: `Pending`, `Progressing`, `Available`, or `Failed` |
| `conditions` | []Condition | Detailed conditions including `Discovered`, `Ready`, `Progressing`, and `Failure` |
| `modelSources` | []ModelSource | Discovered model artifacts with source URIs and sizes, populated after successful discovery |
| `profile` | JSON | Full discovery result containing engine arguments, environment variables, and metadata |

The `status` field provides a quick overview of template readiness. Templates start in `Pending` state when created, transition to `Progressing` while discovery runs, reach `Available` once discovery succeeds, or move to `Failed` if discovery encounters errors. Services should wait for templates to reach `Available` status before deploying.

The `modelSources` array is the primary output of the discovery cache. Each entry includes the model name, download source URI (often a Hugging Face Hub reference), and expected size in bytes. Services and caching mechanisms reference this array to determine what to download and where to find it.

## Service routing templates

When `spec.routing.enabled` is true on an `AIMService`, the operator creates an HTTPRoute that forwards traffic through Gateway API. The HTTP path prefix is computed in three steps:

1. Use `spec.routing.routeTemplate` on the service if present.
2. Otherwise fall back to the resolved runtime config’s `spec.routing.routeTemplate`.
3. If neither is set, default to `/<namespace>/<service-uid>`.

Route templates accept JSONPath fragments wrapped in `{…}` and are rendered against the full `AIMService` object. Every segment is lowercased, RFC 3986 encoded, and de-duplicated; the trailing slash is trimmed and the final string must stay under 200 characters. Invalid expressions, missing data, or a path that exceeds the limit degrade the service with reason `RouteTemplateInvalid`, and the controller skips HTTPRoute creation while leaving the ServingRuntime intact.

Example override on the service:

```yaml
spec:
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    routeTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/{.spec.model}/"
```

Namespace administrators can define the same `routeTemplate` field inside runtime configs to establish tenant-wide defaults, while individual services can provide more specific overrides when necessary. See [AIMService routing](./service.md#routing-templates) for the full behaviour.
