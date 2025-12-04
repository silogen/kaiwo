# Model Caching

AIM provides a hierarchical caching system that allows model artifacts to be pre-downloaded and shared across services in the same namespace. This document explains the caching architecture, resource lifecycle, and deletion behavior.

## Overview

Model caching in AIM uses three resource types:

1. **AIMModelCache**: Stores downloaded model artifacts on a PVC
2. **AIMTemplateCache**: Groups model caches for a specific template (owned by `AIMServiceTemplate`)
3. **AIMService**: Can trigger template cache creation via `spec.cacheModel: true`

## Caching Hierarchy

### Ownership Structure

```
AIMServiceTemplate
    └── AIMTemplateCache (owned by template)
            └── AIMModelCache(s) (created by template cache)
                    └── PVC(s) + Download Job(s) (owned by model cache)
```

### Creation Flow

When an `AIMService` has `spec.cacheModel: true`, the service controller creates an `AIMTemplateCache`, if one doesn't already exist, for the resolved template. However, the cache is **owned by the template**, not the service. This allows:

- Multiple services to share the same template cache
- Cache preservation when a service is deleted (if the AIMTemplateCache becomes Available)
- Proper cleanup when the template itself is deleted

The `AIMTemplateCache` creates an `AIMModelCache` for each needed model. The `AIMModelCache` handles the model download.

## Cache Status Values

Each cache resource tracks its status:

| Status | Description |
| ------ | ----------- |
| `Pending` | Cache created, waiting for processing |
| `Progressing` | Download or provisioning in progress |
| `Available` | Cache is ready and can be used |
| `Failed` | Cache creation failed (download error, storage issue, etc.) |

Note that a `Failed` AIMModelCache will retry the download periodically, causing the Status to change at the same time.

## Deletion Behavior

AIM implements a cache cleanup process that preserves useful caches while cleaning up non-functioning ones.

### Cache handling when AIMService is deleted

When an `AIMService` is deleted:

**Template caches** that were created by this service are evaluated:
- **Available caches** → **Preserved** (can be reused by future services)
- **Non-available caches** (Pending/Progressing/Failed) → **Deleted**

This design allows cache reuse: if you delete a service and recreate it later, the existing Available cache will be used immediately without re-downloading.

**Note**: Since template caches are owned by templates (not services), an Available cache persists as long as its owning template exists.

### Cache handling when AIMServiceTemplate is deletion

When an `AIMServiceTemplate` is deleted:

1. **Template caches** owned by this template are garbage-collected automatically
2. This cleans up non-Available model caches

### AIMTemplateCache Deletion

When an `AIMTemplateCache` is deleted:

1. **Model caches** created by this template cache are evaluated:
   - **Available caches** → **Preserved** (can be reused by other template caches)
   - **Non-available caches** (Pending/Progressing/Failed) → **Deleted**
2. The template cache itself is removed

Available Model caches are preserved because they can be shared across template caches for the same model sources, and they can be reused by any `AIMTemplateCache` created later.

Note that if an AIMService has caching enabled, a new AIMTemplateCache will be immediately created by the AIMService.

### AIMModelCache Deletion

When an `AIMModelCache` is deleted:

1. The **PVC** containing downloaded model files is garbage-collected
2. Any running **download Job** is garbage-collected

NOTE: Any AIMService running with this Model will keep the PVC mounted 

## Cache Reuse

### Automatic Reuse

Services automatically detect and use existing caches:

1. Service resolves its template
2. Controller looks for `AIMTemplateCache` matching the template
3. If an Available cache exists, the service mounts its PVCs directly
4. No re-download is needed

### Cross-Service Sharing

Multiple services can share the same cached models:

- Services using the same template reference the same `AIMTemplateCache`
- Model caches are identified by `sourceURI`, enabling reuse across templates

## Manual Cache Management

* To manually make sure a model is available create an AIMModelCache for that model.
* To make sure all models that belong to a AIMServiceTemplate or AIMClusterServiceTemplate is available, create an AIMTemplateCache in the namespace.
* Manual cleanup is necessary for all `Available` AIMModelCaches.

## Related Documentation

- [Templates](templates.md) - Understanding ServiceTemplates and discovery
- [Services](../usage/services.md) - Deploying services with caching

