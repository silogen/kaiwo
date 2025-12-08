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
  modelName: meta-llama-3-8b-instruct
  runtimeConfigName: ml-research
  metric: throughput
  precision: fp8
  gpuSelector:
    count: 2
    model: MI300X
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
| `modelName` | Model identifier referencing an `AIMModel` or `AIMClusterModel` by `metadata.name`. **Immutable** after creation. |
| `runtimeConfigName` | Runtime configuration for storage defaults and discovery settings. Defaults to `default`. |
| `metric` | Optimization goal: `latency` (interactive) or `throughput` (batch processing). **Immutable** after creation. |
| `precision` | Numeric precision: `auto`, `fp4`, `fp8`, `fp16`, `fp32`, `bf16`, `int4`, `int8`. **Immutable** after creation. |
| `gpuSelector.count` | Number of GPUs per replica. **Immutable** after creation. |
| `gpuSelector.model` | GPU type (e.g., `MI300X`, `MI325X`). **Immutable** after creation. |
| `imagePullSecrets` | Secrets for pulling container images during discovery and inference. Must exist in the same namespace (or operator namespace for cluster templates). |
| `serviceAccountName` | Service account for discovery jobs and inference pods. If empty, uses the default service account. |
| `resources` | Container resource requirements. These override model defaults. |
| `modelSources` | Static model sources (optional). When provided, discovery is skipped and these sources are used directly. See [Static Model Sources](#static-model-sources) below. |

### Namespace-Specific Fields

| Field | Description |
| ----- | ----------- |
| `env` | Environment variables for model downloads (typically authentication tokens). |
| `caching` | Caching configuration for namespace-scoped templates. When enabled, models are cached on startup. |

## Discovery as Cache

### Discovery Process

When a template is created or its spec changes:

1. **Job Creation**: The controller creates a Kubernetes Job using the container image referenced by `modelName` (resolved via `AIMModel` or `AIMClusterModel`)

2. **Dry-Run Inspection**: The job runs the container in dry-run mode, examining model requirements without downloading large files

3. **Metadata Extraction**: The job outputs:

    - Model source URIs (often Hugging Face Hub references)
    - Expected sizes in bytes
    - Engine arguments and environment variables

4. **Status Update**: Discovered information is written to `status.modelSources[]` and `status.profile`

Discovery completes in seconds. The cached metadata remains available for all services referencing this template.

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

Services reference this array when determining runtime requirements.

### Static Model Sources

Templates can optionally provide static model sources in `spec.modelSources` instead of relying on discovery. When static sources are provided:

1. **Discovery is skipped**: No discovery job is created
2. **Sources are used directly**: The provided sources are copied to `status.modelSources[]`
3. **Faster startup**: Templates become `Ready` immediately without waiting for discovery
4. **Manual maintenance**: Sources must be updated manually when the model changes

This is useful when:

- Discovery is not available or not needed
- Model sources are already known and stable
- You want to avoid the discovery job overhead
- Working with custom or non-standard container images

**Example with static sources:**

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama-3-8b-static
  namespace: ml-research
spec:
  modelName: meta-llama-3-8b-instruct
  metric: latency
  precision: fp16
  gpuSelector:
    count: 1
    model: MI300X
  modelSources:
    - name: meta-llama/Llama-3.1-8B-Instruct
      sourceURI: hf://meta-llama/Llama-3.1-8B-Instruct
      size: 16Gi
    - name: tokenizer
      sourceURI: hf://meta-llama/Llama-3.1-8B-Instruct/tokenizer.json
      size: 2Mi
```

When `spec.modelSources` is provided, the template moves directly to `Ready` status without running a discovery job.

### Discovery Job Limits

The AIM operator enforces a global limit of **10 concurrent discovery jobs** across the entire cluster. This prevents resource exhaustion when many templates are created simultaneously.

When this limit is reached:

- New templates wait in `Pending` status with reason `AwaitingDiscovery`
- Discovery jobs are queued and run as existing jobs complete
- Services referencing waiting templates remain in `Starting` status

To avoid delays:

- Use static model sources when discovery is not needed
- Stagger template creation when deploying many models at once
- Consider whether cluster-scoped templates can be shared across namespaces

## Template Derivation

Services can specify runtime overrides without creating explicit templates. The controller automatically derives namespace-scoped templates incorporating these overrides.

### Derivation Algorithm

When `AIMService.spec.overrides` is specified:

1. **Base Template Resolution**: The controller resolves the base template using automatic selection or explicit `templateRef`

2. **Hash Computation**: A SHA256 hash is computed from the override structure (metric, precision, gpuSelector) to ensure deterministic naming

3. **Name Generation**: The derived name is `<base-template>-ovr-<hash-suffix>`, truncated to fit Kubernetes 63-character limit

4. **Template Search**: The controller searches for an existing template (namespace or cluster scope) matching the derived spec

5. **Creation**: If no match exists, a new namespace-scoped template is created with:
   - **Labels**:
     - `app.kubernetes.io/managed-by: aim`
     - `aim.silogen.ai/derived-template: "true"`
   - **Ownership**: The service owns the derived template (OwnerReference set)
   - **Cascade Deletion**: Deleting the service automatically deletes the derived template

6. **Discovery**: The derived template undergoes discovery like any other template

7. **Sharing**: Multiple services with identical overrides share the same derived template (the hash ensures they generate the same name)

**Important notes:**
- Derived templates are always created in the service's namespace, even if the base template is cluster-scoped
- The `aim.silogen.ai/derived-template: "true"` label distinguishes auto-created templates from manually created ones
- Deterministic SHA256 naming prevents duplicate templates and enables template reuse across reconciliations
- Services can reference derived templates by name after they're created, but this is not recommended as names may change

### Example

Service with overrides:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: chat-service
  namespace: ml-team
spec:
  model:
    ref: meta-llama-3-8b
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
  modelName: meta-llama-3-8b
  metric: throughput     # from override
  precision: fp16        # from override
  # other fields copied from base template
```

## Template Status

### Status Fields

| Field | Type | Description |
| ----- | ---- | ----------- |
| `observedGeneration` | int64 | Most recent generation observed |
| `status` | enum | `Pending`, `Progressing`, `NotAvailable`, `Ready`, `Degraded`, `Failed` |
| `conditions` | []Condition | Detailed conditions: `Discovered`, `CacheWarm`, `Ready`, `Progressing`, `Failure` |
| `resolvedRuntimeConfig` | object | Metadata about the runtime config that was resolved (name, namespace, scope, UID) |
| `resolvedImage` | object | Metadata about the model image that was resolved (name, namespace, scope, UID) |
| `modelSources` | []ModelSource | Discovered or static model artifacts with URIs and sizes |
| `profile` | JSON | Complete discovery result with engine arguments and metadata |

### Status Lifecycle

- **Pending**: Template created, discovery not yet started
- **Progressing**: Discovery job running or cache warming in progress
- **NotAvailable**: Template cannot run because required GPU resources are not present in the cluster
- **Ready**: Discovery succeeded (or static sources provided), template ready for use
- **Degraded**: Template is partially functional but has issues
- **Failed**: Discovery encountered terminal errors

Services wait for templates to reach `Ready` before deploying.

### Conditions

**Discovered**: Reports discovery status. Reasons:

- `ProfilesDiscovered`: Discovery completed successfully and runtime profiles were extracted
- `AwaitingDiscovery`: Discovery job has been created and is waiting to run
- `DiscoveryFailed`: Discovery job failed (check job logs for details)

**CacheWarm**: Reports caching status (namespace-scoped templates only). Reasons:

- `Warm`: All model sources have been cached successfully
- `WarmRequested`: Caching has been enabled but not yet started
- `Warming`: Cache warming is in progress
- `WarmFailed`: Cache warming failed

**Ready**: Reports overall readiness

- `True` when template is ready for use (discovered and, if requested, cache warmed)
- `False` during discovery, cache warming, or after failures

**Progressing**: Indicates active reconciliation

- `True` when the controller is actively processing discovery or cache operations
- `False` when template has reached a stable state

**Failure**: Reports terminal failures

- `True` when an unrecoverable error has occurred
- Includes detailed reason and message

## Auto-Creation from Model Discovery

When AIM Models have `spec.discovery.enabled: true` and `spec.discovery.autoCreateTemplates: true`, the controller creates templates from the model's recommended deployments.

These auto-created templates:

- Use naming from the recommended deployment metadata
- Include preset metric, precision, and GPU requirements
- Undergo discovery like manually created templates
- Are managed by the model controller

## Template Selection

When `AIMService.spec.templateRef` is omitted, the controller automatically selects a template:

1. **Enumeration**: Find all templates referencing the model (either by `spec.model.ref` or matching the auto-created model from `spec.model.image`)
2. **Filtering**: Exclude templates not in `Ready` status
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
  modelName: meta-llama-3-8b-instruct
  runtimeConfigName: platform-default
  metric: latency
  precision: fp16
  gpuSelector:
    count: 1
    model: MI300X
```

### Namespace Template

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama-3-8b-throughput
  namespace: ml-research
spec:
  modelName: meta-llama-3-8b-instruct
  runtimeConfigName: ml-research
  metric: throughput
  precision: fp8
  gpuSelector:
    count: 2
    model: MI300X
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
  model:
    ref: meta-llama-3-8b
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
  modelName: meta-llama-3-8b
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

- [Models](models.md) - Understanding the model catalog and discovery
- [Runtime Config Concepts](runtime-config.md) - Resolution algorithm
- [Model Caching](caching.md) - Cache lifecycle and deletion behavior
- [Services Usage](../usage/services.md) - Deploying services with templates
