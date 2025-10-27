# Runtime Configuration

Runtime configurations provide credentials, storage defaults, and routing settings for your inference services. This guide covers practical configuration tasks.

## Quick Start

Create a namespace-scoped runtime configuration:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  defaultStorageClassName: fast-nvme
  serviceAccountName: aim-runtime
  imagePullSecrets:
    - name: registry-credentials
```

Services in the `ml-team` namespace will automatically use this configuration when `runtimeConfigName` is omitted or set to `default`.

## Common Configurations

### Private Container Registries

Add credentials for pulling private images:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-secret
  namespace: ml-team
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_ENCODED_DOCKER_CONFIG
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  imagePullSecrets:
    - name: ghcr-secret
```

### Model Creation Scope

Control whether auto-created models (from `spec.model.image` in AIMService) are cluster-scoped or namespace-scoped:

```yaml
spec:
  model:
    creationScope: Cluster  # or "Namespace"
    autoDiscovery: true     # enable discovery for auto-created models
```

When a service uses `spec.model.image` directly and no matching AIMModel/AIMClusterModel exists, the controller creates one automatically. The `creationScope` field determines whether it creates an AIMModel (namespace-scoped) or AIMClusterModel (cluster-scoped).

### Storage Class

Set the default storage class:

```yaml
spec:
  defaultStorageClassName: fast-nvme
```

This storage class is used when caching is requested but no storage class is specified.

### Service Account

Specify the service account for discovery jobs and supporting pods:

```yaml
spec:
  serviceAccountName: aim-runtime
```

This account needs permissions to:
- Pull container images (via imagePullSecrets)
- Create and manage pods

### HTTP Routing Defaults

Configure default routing behavior for services in the namespace:

```yaml
spec:
  routing:
    enabled: true
    gatewayRef:
      name: kserve-gateway
      namespace: kgateway-system
```

Services can override these defaults individually.

### Custom HTTP Paths

Set a namespace-wide path template:

```yaml
spec:
  routing:
    enabled: true
    gatewayRef:
      name: kserve-gateway
      namespace: kgateway-system
    pathTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/{.metadata.name}"
```

Templates use JSONPath expressions:
- `{.metadata.namespace}` - service namespace
- `{.metadata.name}` - service name
- `{.metadata.labels['key']}` - label value

Paths are lowercased, URL-encoded, and limited to 200 characters.

## Cluster-Wide Configuration

For cluster-wide defaults (without secrets), use `AIMClusterRuntimeConfig`:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterRuntimeConfig
metadata:
  name: default
spec:
  defaultStorageClassName: standard-ssd
```

**Important**: Namespace configs completely replace cluster configs—there's no field-level merging. If you want cluster defaults to apply to a namespace, you must explicitly include those fields in the namespace config.

## Configuration Precedence

When a service references `runtimeConfigName: my-config`:

1. Controller checks for `AIMRuntimeConfig` named `my-config` in the service's namespace
2. If not found, falls back to `AIMClusterRuntimeConfig` named `my-config`
3. If neither exists, the service enters a degraded state

**Default behavior**: When `runtimeConfigName` is omitted, the controller looks for a config named `default`.

## Referencing from Services

Explicitly reference a config:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    ref: meta-llama-3-8b
  runtimeConfigName: team-config
```

Use the default config (implicit):

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    ref: meta-llama-3-8b
  # runtimeConfigName defaults to 'default'
```

## Multiple Configurations

Create different configs for different use cases:

```yaml
# Production config with strict settings
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: production
  namespace: ml-team
spec:
  defaultStorageClassName: premium-ssd
  serviceAccountName: production-sa
  imagePullSecrets:
    - name: prod-registry
  routing:
    enabled: true
    gatewayRef:
      name: production-gateway
      namespace: gateways
---
# Development config with relaxed settings
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: development
  namespace: ml-team
spec:
  defaultStorageClassName: standard
  serviceAccountName: dev-sa
  routing:
    enabled: false
```

Services select the appropriate config:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  runtimeConfigName: production  # or 'development'
```

## Complete Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: registry-creds
  namespace: ml-research
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_ENCODED_CONFIG
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aim-runtime
  namespace: ml-research
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-research
spec:
  defaultStorageClassName: fast-nvme
  serviceAccountName: aim-runtime
  imagePullSecrets:
    - name: registry-creds
  model:
    creationScope: Cluster
    autoDiscovery: true
  routing:
    enabled: true
    gatewayRef:
      name: kserve-gateway
      namespace: kgateway-system
    pathTemplate: "/ml/{.metadata.namespace}/{.metadata.name}"
```

## Checking Active Configuration

View what config a service is using:

```bash
kubectl -n <namespace> get aimservice <name> -o jsonpath='{.status.resolvedRuntimeConfig}'
```

This shows:
- Which config was selected (namespace or cluster scope)
- The config name
- A hash of the config spec

## Troubleshooting

### Service reports RuntimeConfigMissing

Check if the config exists:

```bash
# Namespace-scoped
kubectl -n <namespace> get aimruntimeconfig <config-name>

# Cluster-scoped
kubectl get aimclusterruntimeconfig <config-name>
```

Create the missing config or fix the service's `runtimeConfigName` reference.

### Discovery jobs failing

Verify the service account has necessary permissions:

```bash
kubectl -n <namespace> get serviceaccount <sa-name>
```

Check that imagePullSecrets exist and are valid:

```bash
kubectl -n <namespace> get secret <secret-name>
```

### Path template errors

Check the service status for path template validation errors:

```bash
kubectl -n <namespace> get aimservice <name> -o jsonpath='{.status.conditions[?(@.type=="RoutingReady")]}'
```

Common issues:
- Referenced labels don't exist on the service
- Path exceeds 200 characters
- Invalid JSONPath syntax

## Related Documentation

- [Services](services.md) - Deploy inference services
- [Runtime Config Concepts](../concepts/runtime-config.md) - Resolution algorithm details
