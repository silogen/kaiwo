# AIM Template Controller Tests

This directory contains Chainsaw tests for the AIM template controllers (AIMClusterServiceTemplate and AIMServiceTemplate).

## Test Coverage

### 01-cluster-template-no-image.yaml
Tests that a cluster template fails with proper status when no AIMClusterImage exists for the modelId.

**Expected behavior:**
- Template status: `Failed`
- Condition: `Failure` with reason `ImageNotFound`
- Condition: `Ready=False` with reason `ImageNotFound`

### 02-cluster-template-with-image.yaml
Tests the complete lifecycle of a cluster template with an available image.

**Expected behavior:**
1. Initially: `Pending` status
2. After job creation: `Progressing` status
3. After discovery completes: `Available` status
4. ModelSources and Profile fields are populated
5. ClusterServingRuntime is created

### 03-namespace-template-priority.yaml
Tests that namespace-scoped templates prioritize namespace-scoped images over cluster-scoped images.

**Expected behavior:**
- Discovery job uses the namespace-scoped image (ghcr.io/amd/vllm:namespace-version)
- Template becomes Available
- ServingRuntime is created in the namespace

### 04-namespace-template-fallback.yaml
Tests that namespace-scoped templates fall back to cluster-scoped images when no namespace-scoped image exists.

**Expected behavior:**
- Discovery job uses the cluster-scoped image (ghcr.io/amd/vllm:latest)
- Template becomes Available

### 05-config-image-pull-secrets.yaml
Tests that image pull secrets from AIMClusterConfig are added to discovery jobs.

**Expected behavior:**
- Discovery job has imagePullSecrets from the default config
- Normal event is emitted with reason `ConfigFound`

### 06-no-config-warning.yaml
Tests that missing AIMClusterConfig emits a warning but doesn't block template creation.

**Expected behavior:**
- Discovery job is created without imagePullSecrets
- Warning event is emitted with reason `ConfigNotFound`
- Template can still proceed to Available

### 07-discovery-results.yaml
Tests that discovery results correctly populate the template status fields.

**Expected behavior:**
- `modelSources` array is populated with correct data
- `profile` object contains all expected fields:
  - model, quantized_model
  - metadata (engine, gpu, precision, gpu_count, metric)
  - engine_args, env_vars
  - models array

## Resource Files

- `resources-cluster-image.yaml`: AIMClusterImage for meta-llama/Llama-3.1-8B-Instruct
- `resources-namespace-image.yaml`: AIMImage (namespace-scoped) with different image tag
- `resources-cluster-config.yaml`: AIMClusterConfig with imagePullSecrets
- `cluster-template.yaml`: AIMClusterServiceTemplate for testing
- `namespace-template.yaml`: AIMServiceTemplate for testing

## Running Tests

Run all AIM tests:
```bash
chainsaw test test/chainsaw/tests/standard/aim/
```

Run a specific test:
```bash
chainsaw test test/chainsaw/tests/standard/aim/01-cluster-template-no-image.yaml
```

## Notes

- The discovery job uses mocked data in the controllers, so it will always succeed with predefined results
- Tests verify the controller behavior, status transitions, and condition updates
- Chainsaw automatically handles namespace creation and cleanup
