# Service Templates

Service Templates define runtime configurations for models and serve as a discovery cache. This document explains the template architecture, discovery mechanism, derivation algorithm, and lifecycle management.

## Overview

Templates fulfill two roles:

1. **Runtime Configuration**: Define optimization goals (latency vs throughput), numeric precision, and GPU requirements
2. **Discovery Cache**: Store model artifact metadata to avoid repeated discovery operations

The discovery cache function is critical. When a template is created, the operator inspects the container to determine which model artifacts must be downloaded. This information is stored in `status.modelSources[]` and reused by services and caching mechanisms.

## Cluster vs Namespace Scope

### AIMClusterServiceTemplate

Cluster-scoped templates are typically installed by administrators as part of model catalog bundles. They arrive through GitOps workflows, Helm installations, or operator bundles.

**Key characteristics**:
- Cannot enable caching directly (caching is namespace-specific)
- Can be cached into namespaces using `AIMTemplateCache` resources
- Discovery runs in the operator namespace (default: `kaiwo-system`)
- Provide baseline runtime profiles maintained by platform teams

### AIMServiceTemplate

Namespace-scoped templates are created by ML engineers and data scientists for custom runtime profiles.

**Key characteristics**:
- Can enable model caching via `spec.caching.enabled`
- Support namespace-specific secrets and authentication
- Discovery runs in the template's namespace
- Allow teams to customize configurations beyond cluster baselines

## Template Specification

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama-3-8b-throughput
  namespace: ml-research
spec:
  aimImageName: meta-llama-3-8b-instruct
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

### Common Fields

| Field | Description |
| ----- | ----------- |
| `aimImageName` | Model identifier referencing an `AIMModel` or `AIMClusterModel`. **Immutable** after creation. |
| `runtimeConfigName` | Runtime configuration for credentials and defaults. Defaults to `default`. |
| `metric` | Optimization goal: `latency` (interactive) or `throughput` (batch processing). |
| `precision` | Numeric precision: `auto`, `fp4`, `fp8`, `fp16`, `fp32`, `bf16`, `int4`, `int8`. |
| `gpuSelector.count` | Number of GPUs per replica. |
| `gpuSelector.model` | GPU type (e.g., `MI300X`, `MI325X`). |
| `resources` | Container resource requirements. These override image defaults. |

### Namespace-Specific Fields

| Field | Description |
| ----- | ----------- |
| `caching.enabled` | Pre-warm model downloads. Creates an `AIMTemplateCache` automatically. |
| `env` | Environment variables for model downloads (typically authentication tokens). |
| `imagePullSecrets` | Secrets for pulling private container images. |

## Discovery as Cache

### Discovery Process

When a template is created or its spec changes:

1. **Job Creation**: The controller creates a Kubernetes Job using the container image referenced by `aimImageName` (resolved via `AIMModel` or `AIMClusterModel`)

2. **Dry-Run Inspection**: The job runs the container in dry-run mode, examining model requirements without downloading large files

3. **Metadata Extraction**: The job outputs:
   - Model source URIs (often Hugging Face Hub references)
   - Expected sizes in bytes
   - Engine arguments and environment variables

4. **Status Update**: Discovered information is written to `status.modelSources[]` and `status.profile`

Discovery completes in seconds. The cached metadata remains available for all services and caches referencing this template.

### Discovery Location

- **Cluster templates**: Discovery runs in the operator namespace (default: `kaiwo-system`)
- **Namespace templates**: Discovery runs in the template's namespace

This allows namespace templates to access namespace-specific secrets during discovery.

### Model Sources

The `status.modelSources[]` array is the primary discovery output:

```yaml
status:
  modelSources:
    - name: meta-llama/Llama-3.1-8B-Instruct
      source: hf://meta-llama/Llama-3.1-8B-Instruct
      sizeBytes: 17179869184
    - name: tokenizer
      source: hf://meta-llama/Llama-3.1-8B-Instruct/tokenizer.json
      sizeBytes: 2097152
```

Services and template caches reference this array to determine what to download and where to find it.

## Template Derivation

Services can specify runtime overrides without creating explicit templates. The controller automatically derives namespace-scoped templates incorporating these overrides.

### Derivation Algorithm

When `AIMService.spec.overrides` is specified:

1. **Base Template Resolution**: The controller resolves the base template using automatic selection or explicit `templateRef`

2. **Hash Computation**: A hash is computed from the override structure (metric, precision, gpuSelector)

3. **Name Generation**: The derived name is `<base-template>-ovr-<hash-suffix>`, truncated to fit Kubernetes 63-character limit

4. **Template Search**: The controller searches for an existing template (namespace or cluster scope) matching the derived spec

5. **Creation**: If no match exists, a new namespace-scoped template is created with labels:
   - `app.kubernetes.io/managed-by: aim`
   - `aim.silogen.ai/derived-template: "true"`

6. **Discovery**: The derived template undergoes discovery like any other template

7. **Sharing**: Multiple services with identical overrides share the same derived template

### Example

Service with overrides:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: chat-service
  namespace: ml-team
spec:
  aimImageName: meta-llama-3-8b
  templateRef: base-template
  overrides:
    metric: throughput
    precision: fp16
```

Derived template created:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: base-template-ovr-8fa3c921
  namespace: ml-team
  labels:
    app.kubernetes.io/managed-by: aim
    aim.silogen.ai/derived-template: "true"
spec:
  aimImageName: meta-llama-3-8b
  metric: throughput     # from override
  precision: fp16        # from override
  # other fields copied from base template
```

## Template Status

### Status Fields

| Field | Type | Description |
| ----- | ---- | ----------- |
| `observedGeneration` | int64 | Most recent generation observed |
| `status` | enum | `Pending`, `Progressing`, `Available`, `Failed` |
| `conditions` | []Condition | Detailed conditions: `Discovered`, `Ready`, `Progressing`, `Failure` |
| `modelSources` | []ModelSource | Discovered model artifacts with URIs and sizes |
| `profile` | JSON | Complete discovery result with engine arguments and metadata |

### Status Lifecycle

- **Pending**: Template created, discovery not yet started
- **Progressing**: Discovery job running
- **Available**: Discovery succeeded, template ready for use
- **Failed**: Discovery encountered errors

Services wait for templates to reach `Available` before deploying.

### Conditions

**Discovered**: Reports discovery status
- `True` when discovery completes successfully
- `False` when discovery fails
- Includes failure reason (job failed, image pull error, etc.)

**Ready**: Reports overall readiness
- `True` when template is available for use
- `False` during discovery or after failures

## Auto-Creation from Model Discovery

When AIM Models have `spec.discovery.enabled: true` and `spec.discovery.autoCreateTemplates: true`, the controller creates templates from the model's recommended deployments.

These auto-created templates:
- Use naming from the recommended deployment metadata
- Include preset metric, precision, and GPU requirements
- Undergo discovery like manually created templates
- Are managed by the model controller

## Caching Integration

### Namespace Template Caching

Enable caching on namespace templates:

```yaml
spec:
  caching:
    enabled: true
```

The controller creates an `AIMTemplateCache` resource that:
- Downloads model artifacts listed in `status.modelSources[]`
- Stores them in a PersistentVolume
- Makes them available to services using this template

### Cluster Template Caching

Cluster templates cannot enable caching directly. To cache a cluster template into a namespace:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMTemplateCache
metadata:
  name: cache-cluster-template
  namespace: ml-team
spec:
  templateRef: cluster-template-name  # references AIMClusterServiceTemplate
```

## Template Selection

When `AIMService.spec.templateRef` is omitted, the controller automatically selects a template:

1. **Enumeration**: Find all templates referencing the service's `aimImageName`
2. **Filtering**: Exclude templates not in `Available` status
3. **GPU Filtering**: Exclude templates requiring GPUs not present in the cluster
4. **Override Matching**: If service specifies overrides, filter by matching metric/precision/GPU selector
5. **Selection**: If exactly one candidate remains, select it

If zero or multiple candidates remain, the service reports a failure condition explaining the issue.

## Examples

### Cluster Template - Latency Optimized

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

### Namespace Template with Caching

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama-3-8b-throughput
  namespace: ml-research
spec:
  aimImageName: meta-llama-3-8b-instruct
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
          name: hf-creds
          key: token
```

### Derived Template from Service Override

Service:

```yaml
spec:
  aimImageName: meta-llama-3-8b
  overrides:
    metric: throughput
    gpuSelector:
      count: 4
      model: MI325X
```

Auto-created derived template (conceptual):

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: <base>-ovr-a1b2c3d4
  namespace: ml-team
  labels:
    aim.silogen.ai/derived-template: "true"
spec:
  aimImageName: meta-llama-3-8b
  metric: throughput
  gpuSelector:
    count: 4
    model: MI325X
```

## Troubleshooting

### Template Stuck in Progressing

Check discovery job status:

```bash
# Cluster template
kubectl -n kaiwo-system get job -l aim.silogen.ai/template=<template-name>

# Namespace template
kubectl -n <namespace> get job -l aim.silogen.ai/template=<template-name>
```

View job logs:

```bash
kubectl -n <namespace> logs job/<job-name>
```

Common issues:
- Image pull failures (missing/invalid imagePullSecrets)
- Container crashes during dry-run
- Runtime config missing

### ModelSources Empty After Discovery

Check the template status conditions:

```bash
kubectl -n <namespace> get aimservicetemplate <name> -o jsonpath='{.status.conditions[?(@.type=="Discovered")]}'
```

The container image may not be a valid AIM container image or may not publish model sources correctly.

### Derived Templates Not Created

Check service status:

```bash
kubectl -n <namespace> get aimservice <name> -o yaml
```

Look for:
- Failure conditions explaining why derivation failed
- `resolvedTemplate` showing which template was selected

Ensure the base template exists and is available.

## Related Documentation

- [Models](images.md) - Understanding the model catalog and discovery
- [Runtime Config Concepts](runtime-config.md) - Resolution algorithm
- [Services Usage](../usage/services.md) - Deploying services with templates
