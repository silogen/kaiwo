# Model Sources

AIMClusterModelSource automatically discovers and syncs AI model images from container registries, creating AIMClusterModel resources for matched images.

## Overview

Model sources eliminate the need to manually create model resources for every image version. They continuously sync with container registries, automatically creating models when new images are published.

Key features:

- **Automatic discovery**: Continuously monitors registries for images matching your filters
- **Flexible filtering**: Use wildcards, version constraints, and exclusions
- **Multi-registry support**: Works with Docker Hub, GitHub Container Registry (ghcr.io), and more
- **Periodic sync**: Configurable sync intervals to keep models up to date
- **Private registries**: Supports authentication via imagePullSecrets

## Basic Example

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: silogen-models
spec:
  filters:
    - image: amdenterpriseai/aim-*
  syncInterval: 1h
```

This source discovers all images matching `amdenterpriseai/aim-*` from Docker Hub and creates an AIMClusterModel for each.

## Configuration

### Registry

The `registry` field specifies which container registry to query. Defaults to `docker.io` if not specified.

```yaml
spec:
  registry: ghcr.io  # or docker.io, gcr.io, etc.
```

### Filters

Filters define which images to discover. Each filter specifies a pattern with optional version constraints and exclusions. Multiple filters are combined with OR logic.

#### Repository Patterns

Match repositories using wildcards:

```yaml
spec:
  filters:
    - image: amdenterpriseai/aim-*
```

#### Repository with Specific Tag

Match a specific tag:

```yaml
spec:
  filters:
    - image: silogen/aim-llama:1.0.0
```

#### Full URI

Override the registry for specific filters:

```yaml
spec:
  registry: docker.io
  filters:
    - image: ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1
```

#### Full URI with Wildcard

Override registry and use wildcards:

```yaml
spec:
  registry: ghcr.io
  filters:
    - image: silogen/aim-*
```

### Version Constraints

Use semantic version constraints to filter tags. Supports both global and per-filter version constraints.

#### Global Version Constraints

Apply to all filters:

```yaml
spec:
  registry: ghcr.io
  filters:
    - image: silogen/aim-llama
    - image: silogen/aim-mistral
  versions:
    - ">=1.0.0"
    - "<2.0.0"
```

#### Per-Filter Version Constraints

Override global constraints for specific filters:

```yaml
spec:
  registry: ghcr.io
  versions:
    - ">=1.0.0"  # global default
  filters:
    - image: silogen/aim-llama
      versions:
        - ">=2.0.0"  # overrides global for this filter
    - image: silogen/aim-mistral
      # uses global constraint
```

#### Version Syntax

Constraints use standard semver syntax:

- `>=1.0.0` - Version 1.0.0 or higher
- `<2.0.0` - Below version 2.0.0
- `~1.2.0` - Patch updates only (1.2.x)
- `^1.0.0` - Minor updates allowed (1.x.x)

Prerelease versions (e.g., `0.8.1-rc1`) are supported:

```yaml
versions:
  - ">=0.8.1-rc1"  # includes prereleases
```

Non-semver tags (e.g., `latest`, `dev`) are silently skipped when version constraints are specified.

### Exclusions

Exclude specific repositories from matching:

```yaml
spec:
  filters:
    - image: amdenterpriseai/aim-*
      exclude:
        - amdenterpriseai/aim-base
        - amdenterpriseai/aim-experimental
```

Exclusions match repository names exactly (not including the registry).

### Sync Interval

Control how often the source syncs with the registry:

```yaml
spec:
  syncInterval: 30m  # supports: 15m, 1h, 2h30m, etc.
```

Default is `1h`. Minimum recommended interval is `15m` to avoid rate limiting.

### Private Registries

Authenticate to private registries using imagePullSecrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-secret
  namespace: kaiwo-system  # operator namespace
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_CONFIG
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: private-models
spec:
  registry: ghcr.io
  imagePullSecrets:
    - name: ghcr-secret
  filters:
    - image: myorg/private-model-*
```

Secrets must exist in the operator namespace (typically `kaiwo-system`).

#### GitHub Container Registry (GHCR) Authentication

For GitHub Container Registry, use a GitHub Personal Access Token (PAT) with the minimal required scope:

**Required Scope:**
- `read:packages` - Read access to container packages

**Recommended: Use Fine-Grained Personal Access Tokens**

1. Create a fine-grained PAT at: https://github.com/settings/tokens
2. Set repository access or organization permissions
3. Grant only `read:packages` permission
4. Set expiration date
5. Create the secret:

```bash
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=YOUR_GITHUB_USERNAME \
  --docker-password=YOUR_GITHUB_PAT \
  --namespace=kaiwo-system
```

**Security Best Practices:**
- Use fine-grained PATs instead of classic PATs when possible
- Grant minimal permissions (`read:packages` only)
- Set expiration dates on tokens
- Rotate tokens regularly
- Use separate tokens for different environments (dev/staging/prod)
- Enable encryption at rest for Kubernetes Secrets in production
- Limit Secret access via RBAC to only the operator namespace

**Token Scopes to Avoid:**
- ❌ `repo` - Grants read/write access to repositories (too broad)
- ❌ `write:packages` - Write access not needed for discovery
- ❌ `admin:org` - Organization admin access (unnecessary)
- ❌ `delete:packages` - Delete permission (unnecessary risk)

### Max Models Limit

Control the maximum number of models created to prevent runaway resource creation:

```yaml
spec:
  maxModels: 100  # default: 100, range: 1-10000
  filters:
    - image: org/very-broad-pattern-*
```

When the limit is reached:

- No new models are created, even if more matching images exist
- Existing models are never deleted
- Status shows `modelsLimitReached: true`
- `availableModels` shows total images found vs `discoveredModels` created

**Use Cases:**

- Prevent accidental model explosion from overly broad filters
- Enforce resource quotas in multi-tenant environments
- Limit cluster resource consumption during initial sync

**Example Status:**

```yaml
status:
  status: Ready
  discoveredModels: 100
  availableModels: 250
  modelsLimitReached: true
  conditions:
    - type: MaxModelsLimitReached
      status: "True"
      message: "Model creation limit reached (100 models created). 150 available images not created as models."
```

## Status

The status field tracks sync progress and discovered models:

```bash
kubectl get aimclustermodelsource
```

```
NAME             STATUS   MODELS   LASTSYNC             AGE
silogen-models   Ready    12       2025-01-15T10:30:00  2d
```

### Status Values

- **Pending**: Waiting for initial sync
- **Progressing**: Sync in progress
- **Ready**: All filters succeeded
- **Degraded**: Some filters failed, but others succeeded
- **Failed**: All filters failed

### Detailed Status

```bash
kubectl get aimclustermodelsource silogen-models -o yaml
```

Key status fields:

- `status`: Overall state (Ready, Degraded, Failed, etc.)
- `discoveredModels`: Count of AIMClusterModel resources created
- `availableModels`: Total count of images matching filters in registry
- `modelsLimitReached`: Boolean indicating if maxModels limit was reached
- `lastSyncTime`: Timestamp of last successful sync
- `conditions`: Detailed conditions including Ready, Degraded, and MaxModelsLimitReached

## Examples

### Docker Hub with Wildcards

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: dockerhub-models
spec:
  registry: docker.io
  filters:
    - image: amdenterpriseai/aim-*
      exclude:
        - amdenterpriseai/aim-base
  syncInterval: 2h
```

### GitHub Container Registry with Version Constraints

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: ghcr-stable-models
spec:
  registry: ghcr.io
  filters:
    - image: silogen/aim-llama
    - image: silogen/aim-mistral
  versions:
    - ">=1.0.0"
    - "<2.0.0"
  syncInterval: 1h
```

### Multiple Registries

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: multi-registry-models
spec:
  registry: docker.io  # default
  filters:
    - image: amdenterpriseai/aim-*  # uses docker.io
    - image: ghcr.io/silogen/aim-*  # overrides to ghcr.io
  syncInterval: 1h
```

### Private Registry with Authentication

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: private-registry-creds
  namespace: kaiwo-system
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: BASE64_ENCODED_CONFIG
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: private-models
spec:
  registry: private.registry.io
  imagePullSecrets:
    - name: private-registry-creds
  filters:
    - image: myorg/model-*
      versions:
        - ">=1.0.0"
  syncInterval: 1h
```

### Specific Versions Only

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterModelSource
metadata:
  name: specific-versions
spec:
  registry: ghcr.io
  filters:
    - image: silogen/aim-llama:1.0.0
    - image: silogen/aim-llama:1.1.0
    - image: silogen/aim-mistral:2.0.0
  syncInterval: 6h
```

## Lifecycle

### Created Models

Model sources create AIMClusterModel resources with auto-generated names based on the image URI. These models are owned by the source via an owner reference.

Created models have discovery enabled by default and will automatically create service templates if the image includes recommended deployment metadata.

### Append-Only

Model sources follow an append-only lifecycle during normal operation. Once created, models are never deleted by the source, even if:

- The image is removed from the registry
- The filter is changed or removed

This ensures running services aren't disrupted when registry contents change.

### Ownership and Deletion

Created models have an owner reference to the source. **When you delete the source, Kubernetes will automatically delete all models that were created by it.**

This cascading deletion happens via Kubernetes garbage collection. To prevent accidentally disrupting running services, consider the impact before deleting a model source.

If you need to stop tracking specific models:

1. Update the source filters to exclude those models
2. Delete the unwanted models manually:

```bash
kubectl delete aimclustermodel <model-name>
```

Note: You cannot selectively clean up models while keeping the source unchanged - any models matching the active filters will be recreated on the next sync.

## Troubleshooting

### No Models Discovered

Check the source status:

```bash
kubectl get aimclustermodelsource <name> -o yaml
```

Common causes:

- No images match the filters
- Registry is unreachable
- Authentication failed (check imagePullSecrets)
- Version constraints too restrictive

### Degraded Status

Some filters failed while others succeeded. Check conditions:

```bash
kubectl get aimclustermodelsource <name> -o jsonpath='{.status.conditions}'
```

Look for error messages indicating which filters failed and why.

### Failed Status

All filters failed. Common causes:

- Invalid registry hostname
- Missing or invalid imagePullSecrets
- Network connectivity issues
- Registry catalog API not supported (for wildcard filters)

### Wildcard Filters Not Working

Wildcard filters require registry catalog API support. In addition, support for GitHub Container Registry (ghcr.io) is added via the user of their REST API.

## Related Documentation

- [Models](models.md) - Understanding AIMClusterModel and AIMModel resources
- [Templates](templates.md) - Auto-generated service templates
- [Runtime Config](runtime-config.md) - Authentication and discovery configuration
