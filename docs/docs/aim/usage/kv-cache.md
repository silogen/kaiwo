# Using KV Cache

This guide shows how to configure and use KV cache with your inference services.

## Quick Start

The simplest way to enable KV cache is to let the `AIMService` create one automatically:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    type: redis  # Automatically creates 'kvcache-llama-chat'
```

This creates an `AIMKVCache` resource with default settings (1Gi storage, default storage class, Redis backend).

## Configuration Options

### Custom Storage Size

Specify storage size based on your model and workload:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-70b-instruct:0.7.0
  kvCache:
    type: redis
    storage:
      size: 100Gi  # Larger model needs more cache storage
```

### Custom Storage Class

Use a specific storage class for better performance:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    type: redis
    storage:
      size: 50Gi
      storageClassName: fast-ssd  # Use your high-performance storage class
```

### Custom Access Modes

Specify persistent volume access modes (defaults to ReadWriteOnce):

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    type: redis
    storage:
      size: 50Gi
      accessModes:
        - ReadWriteOnce
```

## Sharing KV Cache

Multiple services can share a single KV cache for better resource utilization.

### Step 1: Create a Standalone KV Cache

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMKVCache
metadata:
  name: shared-cache
  namespace: ml-team
spec:
  kvCacheType: redis
  storage:
    size: 200Gi
    storageClassName: fast-ssd
```

### Step 2: Reference from Multiple Services

```yaml
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    name: shared-cache  # References existing cache
---
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-completion
  namespace: ml-team
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    name: shared-cache  # Same cache, shared across services
```

## Storage Sizing Guide

Choose storage size based on your model and expected usage:

### Small Models (< 7B parameters)

```yaml
kvCache:
  type: redis
  storage:
    size: 10Gi  # Sufficient for most workloads
```

**Use cases**: Chat, simple completion, low-to-medium traffic

### Medium Models (7B - 70B parameters)

```yaml
kvCache:
  type: redis
  storage:
    size: 50Gi  # Balanced for typical usage
```

**Use cases**: Production chat, RAG applications, moderate traffic

### Large Models (> 70B parameters)

```yaml
kvCache:
  type: redis
  storage:
    size: 100Gi  # Start here and scale up as needed
```

**Use cases**: High-volume production, long contexts, batch processing

### Calculating Size

For more precise sizing, use this formula:

```
Storage (GB) ≈ Model_Size (B) × Context_Length (tokens) × Batch_Size × 0.001
```

Example for Llama 70B with 4K context and batch size 8:
```
70 × 4000 × 8 × 0.001 = 2,240 MB ≈ 3 GB (minimum)
```

Add overhead and growth buffer: 3 GB × 20 = 60 GB recommended

## Monitoring and Management

### Check KV Cache Status

```bash
kubectl get aimkvcache -n ml-team
```

Output:
```
NAME              TYPE    STATUS   READY   AGE
kvcache-llama     redis   Ready    1       5m
shared-cache      redis   Ready    1       10m
```

For more details including endpoint information:
```bash
kubectl get aimkvcache -n ml-team -o wide
```

Output:
```
NAME              TYPE    STATUS   READY   ENDPOINT                         AGE
kvcache-llama     redis   Ready    1       redis://kvcache-llama-svc:6379   5m
shared-cache      redis   Ready    1       redis://shared-cache-svc:6379    10m
```

### View Detailed Status

```bash
kubectl describe aimkvcache kvcache-llama -n ml-team
```

This shows additional information including:
- Ready replicas (e.g., "1/1")
- Storage size allocated
- Connection endpoint
- Recent conditions and events
- Last error (if any)

### Check Storage Usage

```bash
kubectl get pvc -n ml-team -l app.kubernetes.io/managed-by=aimkvcache-controller
```

### View Backend Logs

```bash
# Get the StatefulSet name from the AIMKVCache status
kubectl logs -n ml-team kvcache-llama-statefulset-0
```

## Troubleshooting

### KV Cache Stuck in Progressing

Check if the StatefulSet pod is running:

```bash
kubectl get pods -n ml-team -l app.kubernetes.io/name=aimkvcache
```

Check for storage issues:

```bash
kubectl describe pvc -n ml-team
```

Common causes:
- No default storage class configured
- Insufficient storage quota
- Storage class not available

### KV Cache Status Failed

View the conditions for error details:

```bash
kubectl get aimkvcache kvcache-llama -n ml-team -o jsonpath='{.status.conditions}'
```

Check StatefulSet events:

```bash
kubectl describe statefulset -n ml-team kvcache-llama-statefulset
```

### Service Can't Connect to KV Cache

Verify the service endpoint:

```bash
kubectl get svc -n ml-team -l app.kubernetes.io/name=aimkvcache
```

Check AIMService status for KVCache readiness:

```bash
kubectl get aimservice llama-chat -n ml-team -o jsonpath='{.status.conditions[?(@.type=="KVCacheReady")]}'
```

## Best Practices

### 1. Use Shared Caches for Related Services

Share KV cache across services that use the same model or have overlapping prompt patterns:

```yaml
# Good: Multiple chat services using the same model share a cache
kvCache:
  name: llama-8b-shared-cache
```

### 2. Size Conservatively, Then Scale

Start with recommended sizes and monitor actual usage:

```yaml
# Start with 50Gi for a 70B model
storage:
  size: 50Gi
```

Then scale up if needed based on monitoring.

### 3. Use Fast Storage Classes

KV cache performance depends on storage speed:

```yaml
storage:
  size: 100Gi
  storageClassName: premium-ssd  # Use SSD-backed storage
```

### 4. Monitor Storage Capacity

Set up alerts before storage fills up:

```bash
# Check PVC usage regularly
kubectl get pvc -n ml-team
```

### 5. Plan for Failover

Create multiple KV caches for critical workloads:

```yaml
# Primary service
kvCache:
  name: primary-cache

# Failover service (optional)
kvCache:
  name: secondary-cache
```

## Default Behavior

When configuration is omitted, the following defaults apply:

| Field | Default Value | Notes |
|-------|--------------|-------|
| `kvCacheType` | `redis` | Currently only Redis is supported |
| `storage.size` | `1Gi` | Minimum recommended for Redis |
| `storage.storageClassName` | `nil` | Uses cluster default storage class |
| `storage.accessModes` | `[ReadWriteOnce]` | Standard for single-node access |

## See Also

- [KV Cache Concepts](../concepts/kv-cache.md) - Architecture and design
- [Deploying Inference Services](services.md) - AIMService configuration
- [Models](../concepts/models.md) - Model configuration and optimization
