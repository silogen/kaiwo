# AIMService

`AIMService` is the primary user-facing resource for deploying inference endpoints. It combines a model image, service template, runtime configuration, and optional HTTP routing to produce a KServe `InferenceService` with Gateway API integration.

This document describes the AIMService specification, explains how runtime configurations flow into deployed services, describes the template resolution and derivation behavior, and details the routing configuration options.

## Specification

The following is a full example specification of the `AIMService`. The only required field is `spec.aimImageName`, the others are optional.

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  aimImageName: meta-llama-3-8b
  templateRef: llama-3-8b-latency
  runtimeConfigName: team-config
  replicas: 2
  resources:
    limits:
      cpu: "6"
      memory: 48Gi
    requests:
      cpu: "3"
      memory: 32Gi
  overrides:
    metric: throughput
    precision: fp16
    gpuSelector:
      count: 2
      model: MI300X
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    routeTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/chat"
```

## Fields

!!!warning
    Model caching is not yet active

| Field | Type | Description |
| ----- | ---- | ----------- |
| `aimImageName` | string | Canonical model identifier that maps to an `AIMImage` or `AIMClusterImage` resource. This identifies which model container image to deploy. |
| `templateRef` | string | Name of an `AIMServiceTemplate` or `AIMClusterServiceTemplate` that defines the runtime profile. When omitted, the controller uses the image's `defaultServiceTemplate` field, falling back to the service name if no default is specified. |
| `runtimeConfigName` | string | Name of the `AIMRuntimeConfig` or `AIMClusterRuntimeConfig` to use for registry credentials, storage defaults, and routing configuration. Defaults to `default` when omitted. |
| `replicas` | int32 | Number of inference service replicas to deploy. Defaults to 1. |
| `resources` | ResourceRequirements | Container resource requirements. When specified, these override template and image defaults. See [Resource resolution](#resource-resolution) for the complete precedence order. |
| `overrides` | AIMServiceOverrides | Template parameter overrides for this service. When specified, the controller creates a derived template incorporating these overrides. See [Template derivation](#template-derivation-and-overrides) for details. |
| `env` | []EnvVar | Environment variables for model download authentication (e.g., HuggingFace tokens). Applied when templates are derived from this service. |
| `imagePullSecrets` | []LocalObjectReference | Secrets for pulling private container images. Applied when templates are derived from this service. |
| `cacheModel` | bool | When true, ensures model artifacts are cached before the service starts. Takes effect when the referenced template does not already enable caching. Defaults to false. |
| `routing` | AIMServiceRouting | HTTP routing configuration. When enabled, the controller creates an HTTPRoute for Gateway API traffic management. See [Routing configuration](#routing-configuration) for details. |

### AIMServiceOverrides

The `overrides` field allows customizing template parameters without creating explicit template resources:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `metric` | string | Optimization goal: `latency` for interactive workloads, `throughput` for batch processing. |
| `precision` | string | Numeric precision: `auto`, `fp4`, `fp8`, `fp16`, `fp32`, `bf16`, `int4`, `int8`. Lower precision reduces memory usage and increases throughput. |
| `gpuSelector` | AimGpuSelector | GPU requirements specifying `count` (number of GPUs per replica) and `model` (GPU type such as `MI300X` or `MI325X`). |

### AIMServiceRouting

The `routing` field controls HTTP exposure through Gateway API:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `enabled` | bool | Enables HTTP routing management. When true, the controller creates an HTTPRoute resource. |
| `gatewayRef` | ParentReference | Identifies the Gateway parent for the HTTPRoute. Required when routing is enabled. |
| `annotations` | map[string]string | Annotations to apply to the created HTTPRoute resource. |
| `routeTemplate` | string | HTTP path template rendered using JSONPath expressions. See [Routing templates](#routing-templates) for details. |

## Runtime configuration resolution

Runtime configurations supply credentials, storage defaults, and routing parameters. The resolution process works as follows:

1. The controller examines the `runtimeConfigName` field (defaults to `default` when omitted).
2. It first searches for an `AIMRuntimeConfig` in the service's namespace with that name.
3. If a namespace config is found, it is used exclusively—there is no field-level merging with cluster configs.
4. If no namespace config exists, the controller falls back to an `AIMClusterRuntimeConfig` with the same name.
5. The resolved configuration is recorded in `status.effectiveRuntimeConfig` for audit purposes.

When a service references a config that does not exist at either scope, reconciliation fails and the service enters a `Degraded` state with condition reason `RuntimeConfigMissing` until the config is created.

See [Runtime Configuration](./config.md) for complete details on the resolution model and available configuration options.

## Resource resolution

The controller merges resource requirements from three tiers, with higher tiers taking precedence:

1. **Service-level**: `spec.resources` on the AIMService (highest precedence).
2. **Template-level**: `spec.resources` on the resolved AIMServiceTemplate.
3. **Image-level**: `spec.resources` on the AIMImage or AIMClusterImage (lowest precedence).

After merging, if GPU resource requests or limits are still unset, the controller populates them from the discovery metadata stored in the template's status (`status.profile.metadata.gpu_count`). This ensures the resulting KServe InferenceService always requests the appropriate number of GPU devices unless explicitly overridden.

## Template resolution and derivation

The template resolution process determines which template configuration the service will use. This process handles explicit template references, default template lookup, and automatic template derivation when overrides are specified.

### Resolution process

When a service is created or updated, the controller follows this resolution sequence:

1. **Explicit templateRef**: If `spec.templateRef` is specified, the controller searches for a template with that name (namespace-scoped first, then cluster-scoped). If the template is not found, the service enters a `Degraded` state.

2. **Default template lookup**: If `templateRef` is omitted, the controller examines the referenced AIMImage (namespace-scoped first, then cluster-scoped) and uses its `defaultServiceTemplate` field. If no default template is configured, the service enters a `Degraded` state with an appropriate error message.

3. **Override handling**: If `spec.overrides` is specified, the controller modifies the template name by appending a hash suffix and creates a derived template. See [Template derivation](#template-derivation-and-overrides) below.

The controller enforces explicit configuration: services must either reference an existing template via `spec.templateRef` or rely on a configured `defaultServiceTemplate` on the image. There is no automatic template creation unless `spec.overrides` is specified. This prevents unexpected behavior and ensures clear configuration management.

The resolved template reference, including whether it is namespace-scoped or cluster-scoped, is recorded in `status.resolvedTemplate`.

### Template derivation and overrides

When `spec.overrides` is specified, the controller automatically creates a derived namespace-scoped template that incorporates the override values. This allows services to customize runtime parameters without manually creating template resources.

The derivation process works as follows:

1. The controller resolves the base template name using the resolution process described above.

2. It computes a hash of the `overrides` structure and appends a suffix like `-ovr-2926054f` to the base template name. This ensures each unique override combination gets its own template while allowing multiple services with identical overrides to share a derived template.

3. The controller searches for an existing template that matches the derived spec. If a match is found (either namespace-scoped or cluster-scoped), the service uses that template.

4. If no match is found, the controller creates a new namespace-scoped template with the derived name. The template is labeled with `app.kubernetes.io/managed-by: aim` and `aim.silogen.ai/derived-template: "true"`.

5. The derived template undergoes discovery like any other template. Once discovery completes and the template becomes available, the service proceeds with deployment.

**Example**: A service with `templateRef: base-template` and `overrides: {metric: throughput}` might produce a derived template named `base-template-ovr-8fa3c921`. Multiple services with identical overrides will share this derived template.

**Note**: Template derivation ensures the final name does not exceed Kubernetes name length limits (63 characters) by truncating the base name if necessary.

## Routing configuration

When `spec.routing.enabled` is true, the controller creates an HTTPRoute resource that forwards traffic through the specified Gateway. The HTTP path prefix is determined by evaluating the route template.

### Routing templates

Route templates use JSONPath expressions wrapped in `{...}` and are rendered against the entire AIMService object. The controller evaluates templates in this precedence order:

1. `spec.routing.routeTemplate` on the service (highest precedence).
2. `spec.routing.routeTemplate` from the resolved runtime config.
3. Default: `/<namespace>/<service-uid>`.

During rendering, the controller:

- Evaluates each JSONPath expression (e.g., `{.metadata.namespace}`, `{.metadata.labels['team']}`).
- Lowercases and enforces RFC 1123 conventions each path segment.
- Trims duplicate slashes and the trailing slash.
- Validates that the final path is ≤ 200 characters.

If template evaluation fails (invalid JSONPath syntax, missing label/annotation, multi-value result, or path exceeds 200 characters), the service enters a `Degraded` state with condition reason `RouteTemplateInvalid`. The controller creates/updates the InferenceService but skips HTTPRoute creation until the template issue is resolved.

### Routing template examples

Valid template expressions:

```yaml
# Namespace-based path
routeTemplate: "/{.metadata.namespace}/{.metadata.name}"

# Label-based path (label must exist)
routeTemplate: "/team/{.metadata.labels['team']}/{.metadata.name}"

# Static path with namespace
routeTemplate: "/inference/{.metadata.namespace}/llm"
```

Invalid template expressions:

```yaml
# Field doesn't exist - will degrade service
routeTemplate: "/{.spec.model}"

# Missing label - will degrade service if label absent
routeTemplate: "/{.metadata.labels['nonexistent']}"
```

The resolved HTTP path is published in `status.routing.path` for reference. To inspect the generated HTTPRoute, use:

```bash
kubectl -n <namespace> get httproute <service-name> -o yaml
```

## Status

The `status` field reflects reconciliation progress and provides observability into the service lifecycle:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `status` | enum | High-level lifecycle state: `Pending`, `Starting`, `Running`, `Failed`, `Degraded`. |
| `observedGeneration` | int64 | Most recent generation observed by the controller. |
| `conditions` | []Condition | Detailed conditions including `Resolved`, `RuntimeReady`, `RoutingReady`, `CacheReady`, `Ready`, `Progressing`, `Failure`. |
| `resolvedRuntimeConfig` | AIMResolvedRuntimeConfig | Reference to the runtime config used (namespace or cluster scope) and a hash of its spec. |
| `resolvedImage` | AIMResolvedReference | Reference to the AIMImage or AIMClusterImage resolved for this service. |
| `resolvedTemplate` | AIMServiceResolvedTemplate | Reference to the template used, including its name, namespace (if applicable), scope, and UID. Shows derived template names when overrides are applied. |
| `routing` | AIMServiceRoutingStatus | Contains the resolved HTTP `path` when routing is enabled and successfully configured. |

### Status conditions

The controller maintains these condition types:

- **`Resolved`**: True when the image, template, and runtime config have been successfully resolved.
- **`CacheReady`**: True when required model caches are present or cache warming has completed.
- **`RuntimeReady`**: True when the KServe InferenceService is ready to serve traffic.
- **`RoutingReady`**: True when routing is enabled and the HTTPRoute has been successfully created and accepted by the Gateway.
- **`Ready`**: True when all other conditions are satisfied and the service is fully operational.
- **`Progressing`**: True while the controller is actively working toward readiness.
- **`Failure`**: True when a terminal or recoverable failure has occurred.

### Example status - runtime config missing

```yaml
status:
  status: Degraded
  conditions:
  - type: Failure
    status: "True"
    reason: RuntimeConfigMissing
    message: AIMRuntimeConfig "team-config" not found in namespace "ml-team"
  - type: RuntimeReady
    status: "False"
    reason: RuntimeConfigMissing
    message: Cannot configure runtime without AIMRuntimeConfig
```

### Example status - route template invalid

```yaml
status:
  status: Degraded
  conditions:
  - type: Failure
    status: "True"
    reason: RouteTemplateInvalid
    message: 'failed to evaluate route template "{.metadata.labels[''team'']}": label "team" not found'
  - type: RuntimeReady
    status: "True"
    reason: RuntimeReady
    message: InferenceService is ready
  - type: RoutingReady
    status: "False"
    reason: RouteTemplateInvalid
    message: HTTPRoute creation skipped due to invalid route template
```

### Example status - template not found

```yaml
status:
  status: Degraded
  conditions:
  - type: Failure
    status: "True"
    reason: TemplateNotFound
    message: 'Template "llama-latency" not found. Create the template or verify the template name.'
  - type: Resolved
    status: "False"
    reason: TemplateNotFound
    message: 'Template "llama-latency" not found. Create the template or verify the template name.'
  - type: RuntimeReady
    status: "False"
    reason: TemplateNotFound
    message: Referenced template does not exist
  - type: Progressing
    status: "False"
    reason: TemplateNotFound
    message: Cannot proceed without template
```

### Example status - no default template configured

```yaml
status:
  status: Degraded
  conditions:
  - type: Failure
    status: "True"
    reason: TemplateNotFound
    message: 'No template reference specified and no default template found on the image. Provide spec.templateRef or configure the image''s defaultServiceTemplate field.'
  - type: Resolved
    status: "False"
    reason: TemplateNotFound
    message: 'No template reference specified and no default template found on the image. Provide spec.templateRef or configure the image''s defaultServiceTemplate field.'
  - type: RuntimeReady
    status: "False"
    reason: TemplateNotFound
    message: Referenced template does not exist
```

## Events and debugging

The controller emits Kubernetes events on the AIMService object to provide visibility into reconciliation activities:

| Event Type | Reason | Description |
| ---------- | ------ | ----------- |
| Normal | RuntimeConfigResolved | Runtime config successfully resolved and applied. |
| Warning | DefaultRuntimeConfigNotFound | The implicit `default` runtime config was not found (non-fatal). |
| Warning | RuntimeConfigMissing | An explicitly referenced runtime config does not exist (fatal). |
| Warning | RouteTemplateInvalid | Route template evaluation failed. Includes the specific error. |
| Normal | TemplateResolved | Template successfully resolved or created. |
| Normal | InferenceServiceCreated | KServe InferenceService created. |
| Normal | InferenceServiceUpdated | KServe InferenceService updated. |

### Debugging commands

Inspect the service status:

```bash
kubectl -n <namespace> get aimservice <name> -o yaml
kubectl -n <namespace> describe aimservice <name>
```

View the InferenceService:

```bash
kubectl -n <namespace> get inferenceservice <name> -o yaml
```

View the HTTPRoute (when routing is enabled):

```bash
kubectl -n <namespace> get httproute <name>-route -o yaml
```

Check controller logs:

```bash
kubectl -n kaiwo-system logs -l app=aim-controller -f
```

## Related documentation

- [Runtime Configuration](./config.md) - Details on AIMRuntimeConfig resolution and configuration options
- [Images and Templates](./images-and-templates.md) - Understanding AIMImage and AIMServiceTemplate resources
- [Template Caching](./caching.md) - Model artifact caching and pre-warming (if available)
