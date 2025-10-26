# Runtime Configuration Architecture

Runtime configurations provide credentials, storage defaults, and routing parameters. This document explains the resolution algorithm, inheritance model, and status tracking.

## Resolution Model

The AIM operator resolves runtime settings from two Custom Resource Definitions:

- **`AIMClusterRuntimeConfig`**: Cluster-wide defaults that apply across namespaces
- **`AIMRuntimeConfig`**: Namespace-scoped configuration including authentication secrets

### Resolution Algorithm

When a workload references `runtimeConfigName: my-config`:

1. The controller first looks for `AIMRuntimeConfig` named `my-config` in the workload's namespace
2. If found, the namespace config is used **exclusively**
3. If not found, the controller falls back to `AIMClusterRuntimeConfig` named `my-config`
4. The resolved configuration is published in the consumer's `status.effectiveRuntimeConfig`

When `runtimeConfigName` is omitted, the controller resolves a config named `default`.

### No Field-Level Merging

**Critical**: When both namespace and cluster configs exist with the same name, the namespace config **completely replaces** the cluster config. There is no field-level merging or inheritance.

This ensures predictable behavior: what you write is what you get. If you want cluster defaults to apply to a namespace, you must explicitly include those fields in the namespace config.

### Example Resolution

Given:

```yaml
# Cluster config
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterRuntimeConfig
metadata:
  name: default
spec:
  defaultStorageClassName: standard-ssd
  routing:
    enabled: true
---
# Namespace config
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  imagePullSecrets:
    - name: private-registry
```

A service in `ml-team` namespace referencing `runtimeConfigName: default` gets:
- `imagePullSecrets: [private-registry]` (from namespace config)
- **No** `defaultStorageClassName` (namespace config doesn't include it)
- **No** `routing.enabled` (namespace config doesn't include it)

The cluster config fields are **not merged** into the namespace config.

## Effective Runtime Config Tracking

The resolved configuration is published in `status.effectiveRuntimeConfig` with:
- Reference to the source object (namespace or cluster scope)
- Hash of the spec for change detection

### Namespace Config Status

```yaml
status:
  effectiveRuntimeConfig:
    namespaceRef:
      kind: AIMRuntimeConfig
      namespace: ml-team
      name: default
      scope: Namespace
    hash: 792403ad…
```

### Cluster Config Status

```yaml
status:
  effectiveRuntimeConfig:
    clusterRef:
      kind: AIMClusterRuntimeConfig
      name: default
      scope: Cluster
    hash: 5f8a9b2c…
```

Only one ref (namespace or cluster) is present, never both.

## Resources Supporting Runtime Config

The following AIM resources accept `runtimeConfigName`:

- `AIMModel` / `AIMClusterModel`
- `AIMServiceTemplate` / `AIMClusterServiceTemplate`
- `AIMService`
- `AIMTemplateCache`

Each resource independently resolves its runtime config and publishes the result in status.

## Configuration Scoping

### Cluster Runtime Configuration

`AIMClusterRuntimeConfig` captures non-secret defaults shared across namespaces:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterRuntimeConfig
metadata:
  name: default
spec:
  defaultStorageClassName: fast-nvme
```

**Use cases**:
- Platform-wide storage class defaults
- Shared routing configurations for clusters without multi-tenancy

**Limitations**:
- Cannot contain secrets (no `imagePullSecrets` or `serviceAccountName`)
- Cannot enforce namespace-specific policies

### Namespace Runtime Configuration

`AIMRuntimeConfig` provides namespace-specific configuration including authentication:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  defaultStorageClassName: team-ssd
  serviceAccountName: aim-runtime
  imagePullSecrets:
    - name: ghcr-team-secret
  routing:
    enabled: true
    gatewayRef:
      name: kserve-gateway
      namespace: kgateway-system
    pathTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}"
```

**Use cases**:
- Team-specific registry credentials
- Namespace-level routing policies
- Custom storage classes per team
- Service account configuration for discovery jobs

## Field Reference

### Common Fields

| Field | Scope | Description |
| ----- | ----- | ----------- |
| `defaultStorageClassName` | Both | Storage class for model caches when workloads don't specify one |

### Namespace-Only Fields

| Field | Description |
| ----- | ----------- |
| `serviceAccountName` | ServiceAccount used by discovery jobs and supporting pods |
| `imagePullSecrets` | Registry credentials added to pod specs |
| `routing.enabled` | Enable/disable HTTP routing for services using this config |
| `routing.gatewayRef` | Gateway parent for HTTPRoute resources |
| `routing.pathTemplate` | HTTP path template for services. See [Routing Templates](#routing-templates) |

## Routing Templates

Runtime configs can supply a reusable HTTP route template via `spec.routing.pathTemplate`. The template is rendered against the `AIMService` object using JSONPath expressions.

### Template Syntax

```yaml
spec:
  routing:
    pathTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/{.spec.aimImageName}/"
```

### Rendering Process

During reconciliation:

1. **Evaluation**: Each placeholder (e.g., `{.metadata.namespace}`) is evaluated with JSONPath
2. **Validation**: Missing fields, invalid expressions, or multi-value results fail the render
3. **Normalization**: Each path segment is:
   - Lowercased
   - RFC 3986 encoded
   - De-duplicated (multiple slashes collapsed)
4. **Length Check**: Final path must be ≤ 200 characters
5. **Trailing Slash**: Removed

### Rendering Failures

A rendered path that:
- Exceeds 200 characters
- Contains invalid JSONPath
- References missing labels/fields

...degrades the `AIMService` with reason `PathTemplateInvalid` and skips HTTPRoute creation. The ServingRuntime remains intact.

### Precedence

Services evaluate path templates in this order:

1. `AIMService.spec.routing.pathTemplate` (highest precedence)
2. Runtime config's `spec.routing.pathTemplate`
3. Default: `/<namespace>/<service-uid>`

This allows:
- **Runtime configs**: Set namespace-wide path conventions
- **Services**: Override with specific paths when needed

### Example

Runtime config with path template:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    pathTemplate: "/ml/{.metadata.namespace}/{.metadata.labels['project']}"
```

Service using template:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
  labels:
    project: conversational-ai
spec:
  aimImageName: meta-llama-3-8b
  # routing.pathTemplate omitted - uses runtime config template
```

Rendered path: `/ml/ml-team/conversational-ai`

Service with override:

```yaml
spec:
  aimImageName: meta-llama-3-8b
  routing:
    pathTemplate: "/custom/{.metadata.name}"
```

Rendered path: `/custom/llama-chat` (runtime config template ignored)

## Error and Warning Behavior

### Missing Explicit Config

When a workload explicitly references a non-existent config:

```yaml
spec:
  runtimeConfigName: non-existent
```

Result:
- Reconciliation fails
- Workload reports `ConfigResolved=False` (or equivalent failure condition)
- Reconciliation retries until the config appears

### Missing Default Config

When the implicit `default` config doesn't exist:

Result:
- Controller emits event: `DefaultRuntimeConfigNotFound`
- Reconciliation continues without overrides
- Workloads relying on private registries fail later unless a namespace config supplies credentials

This allows workloads without special requirements to proceed even when no default config exists.

### Authentication Expectations

Discovery jobs and cluster-scoped operations run in the operator namespace and use the ServiceAccount/imagePullSecrets from the resolved runtime config.

For **cluster images** and **cluster templates**:
- Resolution happens in the operator namespace
- The `default` runtime config must exist in the operator namespace
- Secrets must exist in the operator namespace

For **namespace images** and **namespace templates**:
- Resolution happens in the resource's namespace
- Secrets must exist in that namespace

## Operator Namespace

The AIM controllers determine the operator namespace from the `AIM_OPERATOR_NAMESPACE` environment variable (default: `kaiwo-system`).

Cluster-scoped workflows such as:
- Cluster template discovery
- Cluster image inspection
- Auto-generated cluster templates

...run auxiliary pods in this namespace and resolve namespaced runtime configs there.

## Examples

### Cluster Config for Storage Defaults

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterRuntimeConfig
metadata:
  name: default
spec:
  defaultStorageClassName: fast-nvme
```

### Namespace Config with Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-team-secret
  namespace: ml-team
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_DOCKER_CONFIG
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  serviceAccountName: aim-runtime
  imagePullSecrets:
    - name: ghcr-team-secret
  defaultStorageClassName: team-ssd
```

### Multiple Named Configs

```yaml
# Production config
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: production
  namespace: ml-team
spec:
  serviceAccountName: production-sa
  imagePullSecrets:
    - name: prod-registry
  defaultStorageClassName: premium-ssd
  routing:
    enabled: true
    gatewayRef:
      name: production-gateway
      namespace: gateways
    pathTemplate: "/prod/{.metadata.namespace}/{.metadata.name}"
---
# Development config
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: development
  namespace: ml-team
spec:
  serviceAccountName: dev-sa
  defaultStorageClassName: standard
  routing:
    enabled: false
```

Services select configs explicitly:

```yaml
spec:
  aimImageName: meta-llama-3-8b
  runtimeConfigName: production  # or 'development'
```

### Operator Namespace Config for Cluster Resources

```yaml
# In operator namespace (e.g., kaiwo-system)
apiVersion: v1
kind: Secret
metadata:
  name: global-registry
  namespace: kaiwo-system
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_CONFIG
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: kaiwo-system
spec:
  serviceAccountName: aim-runtime
  imagePullSecrets:
    - name: global-registry
```

This config is used for cluster-scoped image discovery and template operations.

## Hash-Based Change Detection

The `hash` field in `status.effectiveRuntimeConfig` is a hash of the runtime config's spec. Controllers use this to detect configuration changes:

1. Controller reads workload's `status.effectiveRuntimeConfig.hash`
2. Re-resolves runtime config and computes new hash
3. If hashes differ, the config changed and workload needs reconciliation

This enables efficient change detection without deep spec comparisons.

## Related Documentation

- [Images](models.md) - How images use runtime configs for discovery
- [Templates](templates.md) - Template discovery and runtime config resolution
- [Services Usage](../usage/services.md) - Practical service configuration
- [Runtime Config Usage](../usage/runtime-config.md) - Configuration guide
