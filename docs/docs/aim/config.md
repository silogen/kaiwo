# AIM Configuration

AIMClusterConfig provides cluster-wide configuration for AIM resources, controlling authentication, caching behavior, and routing policies. This resource acts as a shared configuration that templates, images, services, and caches can reference by name, allowing administrators to centralize settings like image pull secrets and storage classes.

## Overview

AIMClusterConfig is a cluster-scoped resource that defines operational settings for AIM deployments. Rather than repeating configuration across multiple resources, you create one or more AIMClusterConfig objects and reference them by name from other AIM types. This approach provides consistency and simplifies management, especially when working with private container registries or configuring model caching.

The most common pattern uses a single config named `default`, which AIM resources automatically reference when no explicit `configName` is specified. For more complex deployments, you can create multiple configs for different environments or teams, then explicitly reference them from your resources.

## Configuration Fields

### ImagePullSecrets

The `imagePullSecrets` field references Kubernetes secrets needed to pull private AIM container images. When templates run discovery jobs or services deploy inference containers, they use these secrets to authenticate with container registries.

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: default
spec:
  imagePullSecrets:
  - name: ghcr-pull-secret
  - name: docker-hub-secret
```

Discovery jobs and inference pods automatically receive these image pull secrets, allowing them to pull images from private registries without requiring per-resource configuration.

### Caching Configuration

The `caching` object controls how AIM manages model artifacts

```yaml
spec:
  caching:
    storageClassName: fast-ssd
```

The `storageClassName` field specifies which storage class to use for model caches. This affects both explicit `AIMTemplateCache` resources and automatic caching triggered by namespace templates with `caching.enabled: true`.

### Routing Configuration

The `routing` object controls how AIM services are exposed through the cluster's ingress infrastructure:

```yaml
spec:
  routing:
    autoCreateRoute: true
    gateway:
      name: kaiwo-gateway
      namespace: kaiwo-system
```

When `autoCreateRoute` is enabled (the default), the operator automatically creates HTTPRoute resources for each AIMService, making them accessible through the specified Gateway. The routes use path-based routing with the pattern `/<namespace>/<service-name>/`, allowing multiple services to share a single gateway.

The `gateway` reference identifies which Gateway API Gateway resource to attach routes to. This field mirrors the `parentRefs` structure used by HTTPRoute resources in the Gateway API.

## Using configName

AIM resources reference configurations through a `configName` field. This field appears in:

- **AIMImage** and **AIMClusterImage**: References config for image pull secrets
- **AIMServiceTemplate** and **AIMClusterServiceTemplate**: References config for discovery jobs
- **AIMTemplateCache**: References config for cache storage and authentication
- **AIMService**: References config for runtime deployments and routing

When you don't specify a `configName`, resources default to referencing a config named `default`. This means a single AIMClusterConfig named `default` can provide configuration for all AIM resources in your cluster.

### Default Config Pattern

The simplest configuration approach uses a single default config:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: default
spec:
  imagePullSecrets:
  - name: registry-secret
  caching:
    storageClassName: standard
```

With this config in place, all AIM resources automatically inherit these settings:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: my-template
  namespace: ml-team
spec:
  aimImageName: llama-3-8b
  # configName: default (implicit)
  metric: latency
  precision: fp16
  gpuSelector:
    count: 1
    model: MI300X
```

The template references the default config automatically, using its image pull secrets for the discovery job.

### Explicit Config References

For deployments requiring different configurations across environments or teams, create multiple configs and reference them explicitly:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: production
spec:
  imagePullSecrets:
  - name: prod-registry-secret
  caching:
    storageClassName: premium-ssd
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: prod-template
  namespace: ml-prod
spec:
  aimImageName: llama-3-8b
  configName: production  # Explicitly reference production config
  metric: throughput
  precision: fp16
  gpuSelector:
    count: 2
    model: MI300X
```

This template uses the `production` config instead of `default`, inheriting its premium storage class and production registry credentials.

## Configuration Resolution and Error Handling

When AIM resources reference configs, the operator handles missing configurations based on whether the reference is explicit or implicit:

### Missing Default Config

If no config named `default` exists and a resource doesn't specify a `configName`, the operator emits a warning event but continues reconciliation:

```
Warning  DefaultConfigNotFound  AIMClusterConfig "default" not found, proceeding with defaults
```

Discovery jobs run without image pull secrets, which works fine for public images but fails for private registries. Services deploy without routing configuration unless specified at the service level.

### Missing Explicit Config

If a resource explicitly specifies a `configName` that doesn't exist, reconciliation fails with a clear error:

```
Status: Failed
Conditions:
  Type: Failure
  Status: True
  Reason: ObservationFailed
  Message: failed to get AIMClusterConfig "production": not found
```

This strict behavior catches configuration errors early, preventing resources from running with incorrect settings. The operator automatically retries when the missing config appears.

## Common Patterns

### Single Default Config

Most deployments use a single default config providing baseline settings for all resources:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: default
spec:
  imagePullSecrets:
  - name: ghcr-secret
  caching:
    storageClassName: standard
    cacheAimImageBase: true
  routing:
    autoCreateRoute: true
    gateway:
      name: kaiwo-gateway
      namespace: kaiwo-system
```

This pattern works well for teams with consistent requirements and simplifies resource definitions by eliminating per-resource configuration.

### Environment-Specific Configs

Larger deployments often create separate configs for different environments:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: development
spec:
  imagePullSecrets:
  - name: dev-registry
  caching:
    storageClassName: standard
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: production
spec:
  imagePullSecrets:
  - name: prod-registry
  caching:
    storageClassName: premium-ssd
  routing:
    gateway:
      name: prod-gateway
      namespace: ingress-prod
```

Resources in development namespaces reference the `development` config, while production resources reference `production`, ensuring appropriate isolation and performance characteristics.

### Team-Specific Configs

Organizations with multiple teams can create team-specific configs:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: ml-research
spec:
  imagePullSecrets:
  - name: research-registry
  caching:
    storageClassName: fast-ssd
    cacheAimImageBase: true
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterConfig
metadata:
  name: ml-production
spec:
  imagePullSecrets:
  - name: prod-registry
  caching:
    storageClassName: standard
```

This allows different teams to maintain separate registry credentials and storage preferences while sharing the same cluster infrastructure.

## Troubleshooting

### Diagnosing Missing Configs

When a resource fails to find its config, check the resource's events and status:

```bash
kubectl describe aimservicetemplate my-template -n ml-team
```

Look for warning or error events:

```
Events:
  Type     Reason              Message
  ----     ------              -------
  Warning  DefaultConfigNotFound  Default AIMClusterConfig not found, proceeding with defaults
```

Or status conditions showing failures:

```
Status:
  Status: Failed
  Conditions:
    Type: Failure
    Status: True
    Message: failed to get AIMClusterConfig "production": not found
```

### Verifying Config Existence

List available configs:

```bash
kubectl get aimclusterconfig
```

Check if the default config exists:

```bash
kubectl get aimclusterconfig default
```

### Fixing Config References

If a resource references a non-existent config, either create the config or update the resource to use an existing one:

```bash
# Create the missing config
kubectl apply -f production-config.yaml

# Or update the resource to use a different config
kubectl edit aimservicetemplate my-template -n ml-team
```

The operator automatically retries reconciliation when the config becomes available.

## Future: Namespace Configs

An upcoming feature will introduce namespace-scoped `AIMNamespaceConfig` resources. These will allow namespaces to override cluster-level settings for resources within that namespace. The resolution order will check for namespace configs first, then fall back to cluster configs, providing fine-grained control while maintaining cluster-wide defaults.
