# AIM Models

AIM Model resources form a catalog that maps logical model identifiers to specific container images. This document explains the model resource types, discovery mechanism, and lifecycle.

## Overview

Model resources serve two purposes:

1. **Registry**: Translate abstract model references into concrete container images
2. **Version control**: Update which container serves a model without changing service configurations

## Cluster vs Namespace Scope

### AIMClusterModel

Cluster-scoped models are typically installed by administrators through GitOps workflows or Helm charts. They represent curated model catalogs maintained by platform teams or model publishers.

Cluster models provide a consistent baseline across all namespaces. Any namespace can reference a cluster model unless it defines a namespace-scoped model with the same name, which takes precedence.

**Discovery for cluster models** runs in the operator namespace (default: `kaiwo-system`). Auto-generated templates are created as cluster-scoped resources.

### AIMModel

Namespace-scoped models allow teams to:

- Define team-specific model variants
- Override cluster-level definitions for testing
- Control model access at the namespace level

When both cluster and namespace models exist with the same `metadata.name`, the namespace resource takes precedence within that namespace.

**Discovery for namespace models** runs in the model's namespace. Auto-generated templates are created as namespace-scoped resources.

## Model Specification

An AIM Model uses `metadata.name` as the canonical model identifier:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModel
metadata:
  name: meta-llama-3-8b-instruct
spec:
  image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  discovery:
    enabled: true
    autoCreateTemplates: true
  resources:
    limits:
      cpu: "8"
      memory: 64Gi
    requests:
      cpu: "4"
      memory: 32Gi
```

### Fields

| Field | Purpose                                                                                                                                                           |
| ----- |-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `image` | Container image URI implementing this model. The operator inspects this image during discovery.                                                                   |
| `discovery` | Controls metadata extraction and automatic template generation. Discovery is attempted automatically.                                                             |
| `discovery.autoCreateTemplates` | When true (default), creates ServiceTemplates from recommended deployments published by the image.                                                                |
| `defaultServiceTemplate` | Default template name to use when services reference this model without specifying a template. Optional.                                                          |
| `imagePullSecrets` | Secrets for pulling the container image during discovery and inference. Must exist in the same namespace as the model (or operator namespace for cluster models). |
| `serviceAccountName` | Service account to use for discovery jobs and metadata extraction. If empty, uses the default service account.                                                    |
| `resources` | Default resource requirements. These serve as baseline values that templates and services can override.                                                           |

## Discovery Mechanism

Discovery is an automatic process that extracts metadata from container images and creates templates.

### Discovery Process

When discovery is enabled:

1. **Job Creation**: The controller creates a Kubernetes Job in the appropriate namespace (operator namespace for cluster images, image namespace for namespace images)

2. **Image Inspection**: The job pulls the container image using credentials from the referenced runtime config and extracts:

    - Required model artifact sources (Hugging Face Hub references, etc.)
    - GPU requirements and recommended deployment configurations
    - Runtime metadata from container labels

3. **Metadata Storage**: Extracted metadata is written to `status.discoveredMetadata`

4. **Template Generation**: If `autoCreateTemplates: true`, the controller examines the image's recommended deployments and creates corresponding ServiceTemplate resources

### Required Labels

AIM discovery expects container images to include specific labels. Official AIM container images include:

- `org.amd.silogen.model.canonicalName`: Model identifier
- `org.amd.silogen.model.deployments`: JSON array of recommended deployment configurations

Missing required labels causes the `MetadataExtracted` condition to flip to `False` and marks the model resource `Failed`.

## Lifecycle and Status

### Status Field

The `status` field tracks discovery progress:

| Field | Description |
| ----- | ----------- |
| `status` | Enum: `Pending`, `Progressing`, `Ready`, `Degraded`, `Failed` |
| `conditions` | Detailed conditions including `RuntimeResolved` and `MetadataExtracted` |
| `resolvedRuntimeConfig` | Metadata about the runtime config that was resolved (name, namespace, scope, UID) |
| `imageMetadata` | Extracted metadata from the container image including model and OCI metadata |

### Status Values

- **Pending**: Initial state, waiting for reconciliation
- **Progressing**: Discovery job running or templates being created
- **Ready**: Discovery succeeded and all auto-generated templates are healthy
- **Degraded**: Discovery succeeded but some templates have issues
- **Failed**: Discovery failed or required labels missing

### Conditions

**RuntimeResolved**: Reports whether runtime config resolution succeeded. Reasons:

- `RuntimeResolved`: Runtime configuration was successfully resolved
- `RuntimeConfigMissing`: The explicitly referenced runtime config was not found
- `DefaultRuntimeConfigMissing`: The implicit default runtime config was not found (warning, allows reconciliation to continue)

**MetadataExtracted**: Reports whether image inspection succeeded. Reasons:

- `MetadataExtracted`: Discovery completed successfully
- `MetadataExtractionFailed`: Discovery job failed or required labels missing from image

### Toggling Discovery

You can enable discovery after image creation:

```bash
kubectl edit aimclustermodel meta-llama-3-8b-instruct
# Set spec.discovery.enabled: true
```

The controller runs extraction on the next reconciliation and updates status accordingly.

Disabling discovery after templates exist leaves templates in place. The `TemplatesAutoGenerated` condition remains `True`.

## Resource Resolution

When services reference a model, the controller merges resources from multiple sources:

1. Service-level: `AIMService.spec.resources` (highest precedence)
2. Template-level: `AIMServiceTemplate.spec.resources`
3. Model-level: `AIMModel.spec.resources` (baseline)

If GPU quantities remain unset after merging, the controller copies them from discovery metadata recorded on the template (`status.profile.metadata.gpu_count`).

## Model Lookup

For namespace-scoped lookups (from templates or services in a namespace):

1. Check for `AIMModel` in the same namespace
2. Fall back to `AIMClusterModel` with the same name

This allows namespace models to override cluster baselines.

## Examples

### Cluster Model with Discovery

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModel
metadata:
  name: meta-llama-3-8b-instruct
spec:
  image: ghcr.io/example/llama-3.1-8b-instruct:v1.2.0
  runtimeConfigName: platform-default
  discovery:
    enabled: true
    autoCreateTemplates: true
  resources:
    limits:
      cpu: "8"
      memory: 64Gi
      nvidia.com/gpu: "1"
    requests:
      cpu: "4"
      memory: 32Gi
      nvidia.com/gpu: "1"
```

### Namespace Model Without Discovery

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMModel
metadata:
  name: meta-llama-3-8b-dev
  namespace: ml-team
spec:
  image: ghcr.io/ml-team/llama-dev:latest
  runtimeConfigName: ml-team
  defaultServiceTemplate: custom-template-name
  discovery:
    enabled: false  # skip discovery and auto-templates
  resources:
    limits:
      cpu: "6"
      memory: 48Gi
```

### Enabling Discovery for Private Container Images

```yaml
# Secret in namespace
apiVersion: v1
kind: Secret
metadata:
  name: private-registry
  namespace: ml-team
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_CONFIG
---
# Runtime config in namespace
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  serviceAccountName: aim-runtime
  imagePullSecrets:
    - name: private-registry
---
# Model with discovery
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMModel
metadata:
  name: proprietary-model
  namespace: ml-team
spec:
  image: private.registry/models/proprietary:v1
  runtimeConfigName: default  # uses config above
  discovery:
    enabled: true
```

## Troubleshooting

### Discovery Job Fails

Check the job logs:

```bash
# For cluster images
kubectl -n kaiwo-system logs -l job-name=<image-name>-discovery

# For namespace images
kubectl -n <namespace> logs -l job-name=<image-name>-discovery
```

Common causes:

- Missing or invalid imagePullSecrets
- Image doesn't exist or tag is invalid
- Required labels missing from image

### Templates Not Auto-Created

Check the model status:

```bash
kubectl get aimclustermodel <name> -o yaml
# or
kubectl -n <namespace> get aimmodel <name> -o yaml
```

Look for:

- `discovery.enabled: false` - discovery is disabled
- `discovery.autoCreateTemplates: false` - auto-creation disabled
- `TemplatesAutoGenerated` condition with reason `NoRecommendedTemplates`

### MetadataExtracted Condition False

The container image is missing required labels or the discovery job failed. Check:

```bash
kubectl get aimclustermodel <name> -o jsonpath='{.status.conditions[?(@.type=="MetadataExtracted")]}'
```

Inspect the container image labels:

```bash
docker pull <image>
docker inspect <image> --format='{{json .Config.Labels}}'
```

## Auto-Creation from Services

When a service uses `spec.model.image` directly (instead of `spec.model.ref`), AIM automatically creates a model resource if one doesn't already exist with that image URI.

### Creation Scope

The runtime config's `spec.model.creationScope` field controls whether the auto-created model is cluster-scoped or namespace-scoped:

```yaml
# In runtime config
spec:
  model:
    creationScope: Cluster  # creates AIMClusterModel
    # OR
    creationScope: Namespace  # creates AIMModel in service's namespace
```

### Discovery for Auto-Created Models

The runtime config's `spec.model.autoDiscovery` field controls whether auto-created models run discovery:

```yaml
spec:
  model:
    autoDiscovery: true  # auto-created models run discovery and create templates
```

### Example

Service using direct image reference:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: my-service
  namespace: ml-team
spec:
  model:
    image: ghcr.io/example/my-model:v1.0.0
  runtimeConfigName: default
```

If the runtime config has `creationScope: Cluster` and `autoDiscovery: true`, AIM creates:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModel
metadata:
  name: auto-<hash-of-image>
spec:
  image: ghcr.io/example/my-model:v1.0.0
  discovery:
    enabled: true
    autoCreateTemplates: true
```

## Related Documentation

- [Templates](templates.md) - Understanding ServiceTemplates and discovery
- [Runtime Config Concepts](runtime-config.md) - Resolution details including model creation
- [Services Usage](../usage/services.md) - Deploying services

## Note on Terminology

AIM Model resources (`AIMModel` and `AIMClusterModel`) define the mapping between model identifiers and container images. While we sometimes refer to the "model catalog" conceptually, the Kubernetes resources are always `AIMModel` and `AIMClusterModel`.
