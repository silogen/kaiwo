# AMD Inference Microservices (AIM) — v1alpha1 Guide

AIM provides a consistent way to deploy optimized LLM inference services on AMD GPUs using KServe and a vLLM-based runtime. This document explains the resource model and the recommended workflows to **install a model catalog**, **template** the runtime, **cache** model artifacts, and **deploy** services.

> **Source of truth:** All naming and behavior in this guide follow the provided v1alpha1 CRDs.

---

## Roles & scopes

| Role                               | Responsibilities                                                                                                                                                                              |
| ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Cluster administrator**          | Publishes models as cluster-scoped catalog entries (`AIMImage`), may define cluster-wide templates (`AIMClusterServiceTemplate`), enables storage and KServe.                                 |
| **Tenant administrator**           | Prepares each namespace using `AIMNamespaceConfig` (routing and cache storage defaults).                                                                                                      |
| **Application user / ML engineer** | Creates namespace templates (`AIMServiceTemplate`), optionally pre-warms caches (`AIMTemplateCache`), deploys services (`AIMService`), and may create `ModelCache` directly for fine control. |
| **Operator (controller)**          | Reconciles AIM resources: discovers model sources for templates, warms caches (`TemplateCache` → `ModelCache` → PVC), creates KServe artifacts and routes, and updates status/conditions.     |

**Scope quick reference**

| Kind                        | Scope     | Purpose                                                                                                                          |
| --------------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `AIMImage`                  | Cluster   | Catalog entry: **modelId → container image**, with a **defaultServiceTemplate** name (advisory).                                 |
| `AIMClusterServiceTemplate` | Cluster   | Cluster-wide runtime profile for one model (no caching field).                                                                   |
| `AIMServiceTemplate`        | Namespace | Namespace runtime profile for one model; can enable caching via `spec.caching.enabled`.                                          |
| `AIMTemplateCache`          | Namespace | Pre-warms caches for a named template (resolves ns first, then cluster).                                                         |
| `ModelCache`                | Namespace | Ensures a PVC exists and the **model sourceUri** is downloaded. Uniqueness by `spec.sourceUri` (immutable).                      |
| `AIMNamespaceConfig`        | Namespace | Routing and default cache storage configuration for the namespace.                                                               |
| `AIMService`                | Namespace | A running inference service bound to `spec.aimModelId` + **`spec.templateRef`** (required), with optional per-service overrides. |

---

## Quickstart: install a catalog and run services

This section starts with the **simplest paths first**, then adds variations.

### 1) Install a predefined catalog (images + cluster templates)

In most environments you will consume a curated set of **AIMImage** objects (model IDs and images) and **AIMClusterServiceTemplate** objects from an **external repo** packaged with Kustomize.

```bash
# Example: install a predefined catalog bundle (models + cluster templates)
kubectl apply -k https://example.org/aim-packs/catalogs/llama3
```

> These bundles typically include:
>
> * `AIMImage` entries with `spec.modelId`, `spec.image`, and a `spec.defaultServiceTemplate` name.
> * Matching `AIMClusterServiceTemplate` objects for common runtime profiles (e.g., latency-optimized, throughput-optimized).

### 2) Deploy services — common patterns

> **Important (CRD correctness):** In v1alpha1, `AIMService.spec.templateRef` is **required**. The “default template” concept comes from `AIMImage.spec.defaultServiceTemplate`; in practice, packs set `templateRef` to that default for you so you only edit the **model ID** or add **overrides**.

#### (a) Service with just the model ID (uses the pack’s default template)

Use a service manifest provided by the pack; it already fills `templateRef` with the image’s default template. You typically only change `spec.aimModelId`.

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama3-svc
  namespace: team-a
spec:
  aimModelId: meta/llama-3-8b:1.1+20240915
  templateRef: llama3-8b-latency-gpu1   # pre-wired by the pack to match the model’s default
  cacheModel: true                      # on-demand caching (default true)
  replicas: 1
  configRef: default
```

#### (b) Service that explicitly selects a namespace template

Create or reuse a **namespace** template, then reference it:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama3-8b-fast
  namespace: team-a
spec:
  model: meta/llama-3-8b:1.1+20240915
  metric: latency
  precision: fp8
  gpuSelector:
    count: 1
    model: MI300X
  caching:
    enabled: false
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama3-fast-svc
  namespace: team-a
spec:
  aimModelId: meta/llama-3-8b:1.1+20240915
  templateRef: llama3-8b-fast
  cacheModel: true
  replicas: 2
  configRef: default
```

#### (c) Service with **overrides** and **no custom template**

You do **not** need to author a namespace template to tweak a few knobs. Keep `templateRef` pointing to a base (default) template and use `spec.overrides`. **Precedence** at runtime:

```
service overrides  >  template values
```

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama3-override-svc
  namespace: team-a
spec:
  aimModelId: meta/llama-3-8b:1.1+20240915
  templateRef: llama3-8b-latency-gpu1   # base template (from catalog pack)
  cacheModel: true
  overrides:
    precision: bf16
    gpuSelector:
      count: 2
      model: MI300X
  replicas: 2
```

#### (d) Service that **selects a template** and also **overrides** it

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama3-fast-bf16
  namespace: team-a
spec:
  aimModelId: meta/llama-3-8b:1.1+20240915
  templateRef: llama3-8b-fast            # ns template
  cacheModel: true
  overrides:
    precision: bf16                      # wins over template.precision
  replicas: 1
```

---

## Caching: on-demand and pre-warming

AIM supports two complementary paths.

### On-demand caching (most users)

If `AIMService.spec.cacheModel: true` (default), the operator:

1. Resolves the template referenced by `templateRef`.
2. Ensures an **AIMTemplateCache** exists in the service’s namespace for that template.
3. For each model in **`template.status.modelSources[]`**:

    * Ensures a **ModelCache** object exists (unique by `spec.sourceUri`).
    * Ensures the PVC exists and downloads the model content (reused across services for the same source).

You do **not** need to create `AIMTemplateCache` or `ModelCache` manually for this path.

### Pre-warm a cache (for fast startup or CI/CD pipelines)

You can warm caches before deploying or scaling services:

* Enable caching at the **template** level:

  ```yaml
  apiVersion: aim.silogen.ai/v1alpha1
  kind: AIMServiceTemplate
  metadata:
    name: llama3-8b-fast
    namespace: team-a
  spec:
    model: meta/llama-3-8b:1.1+20240915
    metric: latency
    precision: fp8
    gpuSelector:
      count: 1
      model: MI300X
    caching:
      enabled: true
  ```

  The operator creates the required `AIMTemplateCache`, which fans out to `ModelCache` + PVC.

* Or create a **TemplateCache** directly (resolves an ns template first; falls back to a cluster template of the same name):

  ```yaml
  apiVersion: aim.silogen.ai/v1alpha1
  kind: AIMTemplateCache
  metadata:
    name: warm-llama3-8b-fast
    namespace: team-a
  spec:
    templateRef: llama3-8b-fast
  ```

> **ModelCache** details: `spec.sourceUri` is immutable and the uniqueness key; the same source is reused across templates/services within the namespace. The PVC name appears in `status.persistentVolumeClaim`.

---

## Discovery & status (what the operator populates)

* On **template create/update** (both ns and cluster), the operator runs a **dry-run** AIM container to determine required models:

    * **Namespace template**: job runs in the same namespace.
    * **Cluster template**: job runs in the operator’s system namespace.
* Discovered models are written to **`status.modelSources[]`** on the template as `{ sourceUri, size }`.

**Key status fields**

* **Templates**: `status.status` (`Pending | Progressing | Available | Degraded | Failed`) + conditions (`Discovered`, `CacheWarm`, `Ready`, `Progressing`, `Failure`).
* **TemplateCache**: `status.status` + conditions (`Resolved`, `CacheWarm`, `Ready`, `Progressing`, `Failure`) and `status.resolvedTemplateKind`.
* **ModelCache**: `status.status` + conditions (`StorageReady`, `Progressing`, `Ready`, `Failure`), and `status.persistentVolumeClaim`.
* **Service**: `status.status` (`Pending | Starting | Running | Failed | Degraded`) + conditions (`Resolved`, `CacheReady`, `RuntimeReady`, `RoutingReady`, `Ready`, `Progressing`, `Failure`).

---

## Cluster- vs namespace-scoped templates

* **AIMClusterServiceTemplate (cluster)**
  Maintained by cluster admins for consistency. It does **not** carry a caching field. Discovery runs in the operator’s system namespace. Useful as a **reference profile** or baseline.

* **AIMServiceTemplate (namespace)**
  Owned by tenants/users. Can enable caching (`spec.caching.enabled`). Discovery runs in the same namespace. Recommended for day-to-day service deployment because `AIMService.spec.templateRef` must name a **namespace** template in v1alpha1.

* **Cache warm resolution**
  `AIMTemplateCache.spec.templateRef` resolves to a namespace template first; if none exists, it falls back to a cluster template with the same name.

---

## Defining your own models and templates

### Define a model catalog entry (cluster admin)

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMImage
metadata:
  name: meta-llama-3-8b
spec:
  modelId: meta/llama-3-8b:1.1+20240915
  image: registry.example.com/aim/llama3-8b:1.1
  defaultServiceTemplate: llama3-8b-latency-gpu1
```

(Optional) provide a cluster template:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterServiceTemplate
metadata:
  name: llama3-8b-latency-gpu1
spec:
  model: meta/llama-3-8b:1.1+20240915
  metric: latency
  precision: fp16
  gpuSelector:
    count: 1
    model: MI300X
```

### Author a namespace template (user)

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama3-8b-mi325x-throughput
  namespace: team-a
spec:
  model: meta/llama-3-8b:1.1+20240915
  metric: throughput
  precision: bf16
  gpuSelector:
    count: 2
    model: MI325X
  caching:
    enabled: false
```

Then deploy a service with that template (or use overrides as shown earlier).

---

## Namespace configuration (routing & storage defaults)

`AIMNamespaceConfig` configures **routing** and a default **cache storage class** for a namespace. (This guide intentionally omits secret/credential definitions in the namespace config.)

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMNamespaceConfig
metadata:
  name: default
  namespace: team-a
spec:
  routing:
    gateway:
      name: public-gw
      namespace: infra-gw
    autoCreateRoute: true
  cacheStorageClassName: fast-rwx
```

If routing is enabled, the operator creates one HTTPRoute per `AIMService` and attaches it to the configured Gateway. Paths typically include the namespace and an internal workload identifier.

---

## Monitoring & troubleshooting

```bash
# Inspect discovery results on a template
kubectl -n team-a get aimservicetemplate llama3-8b-fast -o yaml | yq '.status'

# See which PVC backs a ModelCache
kubectl -n team-a get modelcache llama3-weights -o jsonpath='{.status.persistentVolumeClaim}{"\n"}'

# List TemplateCaches and resolved kinds
kubectl -n team-a get aimtemplatecache
```

**Common issues**

* **Template not found (service)** → `AIMService` condition `Resolved=False`, reason `TemplateNotFound`.
* **Discovery failures (template)** → template condition `Failure` with reason `DiscoveryFailed`.
* **Storage problems (ModelCache)** → `StorageReady=False` with reasons like `PVCPending`, `StorageClassMissing`, `InsufficientCapacity`.
* **Download failures (ModelCache)** → condition `Failure`, reason `DownloadFailed`.
* **Routing failures (service)** → reasons `RouteFailed`, `ConfiguringRoute`.

---

## Summary

* Install a **catalog pack** (`AIMImage` + cluster templates) via `kubectl apply -k …`.
* Deploy services using one of the four common patterns:

    1. **Just the model ID** (pack pre-fills `templateRef` to the default).
    2. **Explicit namespace template**.
    3. **Overrides without a custom template** (still reference a base template).
    4. **Template + overrides** (overrides win).
* Let `cacheModel: true` handle **on-demand caching**, or pre-warm via `AIMServiceTemplate.spec.caching.enabled` or an explicit `AIMTemplateCache`.
* Use namespace templates for day-to-day deployment; cluster templates serve as shared baselines.
* Track progress via status/conditions on Template, TemplateCache, ModelCache, and Service.
