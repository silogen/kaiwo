# AIMKVCache Tests

Tests for the KVCache controller that manages Redis backends for LMCache.

## Tests

**basic/** - Creates a KVCache with defaults and checks it becomes Ready with the correct endpoint.

**custom-image-env/** - Uses custom Redis image and environment variables to verify container configuration.

**custom-resources/** - Sets custom CPU/memory requests and limits, verifies pods use them.

**custom-storage/** - Configures custom storage size and storage class, checks PVC matches.

**default-name/** - When AIMService doesn't specify a KVCache name, it should default to `kvcache-{service-name}`.

**default-resources/** - Without specifying resources, KVCache should use defaults (1 CPU, 1Gi memory).

**service-integration/** - AIMService creates a KVCache, generates LMCache config, and mounts it into InferenceService.

**shared-kvcache/** - Multiple AIMServices reference the same pre-created KVCache instance.

**status-progression/** - Tracks status transitions from Pending → Progressing → Ready as resources come up.

**standalone-update/** - Updates a standalone KVCache with new resources; verifies StatefulSet is updated.

**service-no-update/** - AIMService does NOT update existing KVCache when service spec changes.

**manual-update-service-created/** - Manually updating a service-created KVCache works (controller applies updates).

**preexisting-no-update/** - AIMService does NOT update pre-existing KVCache when referencing with different settings.

## Configuration

**Storage Class**: Configured via `test/chainsaw/values/kvcache.yaml` (default: `openebs-hostpath`).

Override by creating a custom values file:
```yaml
# custom-values.yaml
storageClass: your-storage-class
```

## Running

```bash
# All kvcache tests
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache \
  --values=test/chainsaw/values/kvcache.yaml

# With custom storage class
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache \
  --values=custom-values.yaml

# Single test
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache/basic \
  --values=test/chainsaw/values/kvcache.yaml
```

**Note**: The `--values` flag is required for these tests. YAMLs use `($values.storageClass)` templating.

