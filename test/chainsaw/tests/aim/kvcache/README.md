# AIMKVCache Tests

Tests for the KVCache controller that manages Redis backends for LMCache.

## Tests

### **standalone/**
Comprehensive standalone AIMKVCache lifecycle test covering:
- Basic creation with defaults (default resources: requests=1 CPU/1Gi memory, limits=1 CPU/1Gi memory)
- Custom image and environment variables
- Custom resource requests/limits
- Custom storage size and storage class
- Status progression (Pending → Progressing → Ready)
- Updates to standalone KVCache (verifies StatefulSet updates)

This single comprehensive test covers all critical KVCache controller functionality in ~2 minutes.

## Configuration

**Storage Class**: Configured via `test/chainsaw/values/kvcache.yaml` (default: `openebs-hostpath`).

Override by creating a custom values file:
```yaml
# custom-values.yaml
storageClass: your-storage-class
```

## Running

```bash
# Run the comprehensive kvcache test
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache \
  --values=test/chainsaw/values/kvcache.yaml

# With custom storage class
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache \
  --values=custom-values.yaml

# Or run directly
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache/standalone \
  --values=test/chainsaw/values/kvcache.yaml
```

**Note**: The `--values` flag is required. YAMLs use `($values.storageClass)` templating.

