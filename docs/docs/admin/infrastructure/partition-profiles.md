# Partition Profile Reference

This document provides detailed guidance on creating and managing GPU partition profiles for AMD GPU clusters. It includes profile examples for common configurations, explains how to integrate with the AMD GPU operator's DCM system, and provides troubleshooting guidance.

## Profile anatomy

A `PartitioningProfile` defines a reusable GPU partition configuration that can be applied to multiple nodes through `PartitioningPlan` resources.

### Minimal profile

The simplest profile references a DCM configuration:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-single
spec:
  displayName: "Single GPU per partition"
  profileName: mi300x-1gpu
```

This profile:
- References the `mi300x-1gpu` configuration in the DCM ConfigMap
- Provides a human-readable display name for tools and dashboards
- Does not validate target nodes or expected resources

### Complete profile with validation

A production profile includes guardrails and verification:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-quad
spec:
  displayName: "MI300X 4-Way Partition (Inference)"
  profileName: mi300x-4gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "4"
```

This profile:
- Validates that only MI300X nodes can use this profile via `targetSelector`
- Verifies that `node.status.allocatable["amd.com/gpu"]` reports exactly 4 GPUs after partitioning
- Prevents operator errors by failing early if applied to incompatible nodes

## DCM integration

### Understanding DCM

The AMD GPU operator uses the Device Configuration Manager (DCM) to apply partition configurations to GPUs. DCM reads partition profiles from a ConfigMap and applies them when nodes are labeled with `dcm.amd.com/gpu-config-profile`.

The partitioning controller automates this process:
1. Ensures the profile exists in the DCM ConfigMap (future enhancement)
2. Sets the `dcm.amd.com/gpu-config-profile` label on the node
3. Waits for the AMD GPU device plugin to restart
4. Verifies the new allocatable resources

### Inspecting DCM profiles

View the DCM ConfigMap:

```bash
kubectl -n kube-amd-gpu get configmap config-manager-config -o yaml
```

The ConfigMap contains a `config.json` field with profile definitions:

```json
{
  "gpu-config-profile": {
    "mi300x-1gpu": {
      "partitions": 8,
      "compute-units-per-partition": 228
    },
    "mi300x-2gpu": {
      "partitions": 4,
      "compute-units-per-partition": 456
    },
    "mi300x-4gpu": {
      "partitions": 2,
      "compute-units-per-partition": 912
    }
  }
}
```

Your `PartitioningProfile.spec.profileName` must match a key in the `gpu-config-profile` object.

### Creating DCM profiles

DCM profiles are typically managed by the AMD GPU operator installation. Consult the [AMD GPU Operator documentation](https://github.com/AMD/gpu-operator) for guidance on creating custom partition configurations.

If you need to add a custom profile:

1. Edit the ConfigMap:
   ```bash
   kubectl -n kube-amd-gpu edit configmap config-manager-config
   ```

2. Add your profile to the `gpu-config-profile` section in `config.json`.

3. Create a corresponding `PartitioningProfile` resource.

**Note**: Manual ConfigMap edits may be overwritten by operator upgrades. Consider using Helm value overrides or Kustomize patches for production environments.

## Profile examples

### MI300X profiles

#### Single GPU per partition (8 partitions)

Maximum partitioning for workloads requiring small GPU allocations:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-max-partition
spec:
  displayName: "MI300X 8-Way Partition"
  profileName: mi300x-1gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "8"
```

Use cases:
- Small inference workloads (< 16GB VRAM)
- Development environments
- High-density serving

#### Dual GPU per partition (4 partitions)

Balanced configuration for medium workloads:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-dual
spec:
  displayName: "MI300X Dual GPU"
  profileName: mi300x-2gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "4"
```

Use cases:
- Medium language models (13B-30B parameters)
- Multi-GPU inference
- Training small to medium models

#### Quad GPU per partition (2 partitions)

Large allocations for demanding workloads:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-quad
spec:
  displayName: "MI300X Quad GPU"
  profileName: mi300x-4gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "2"
```

Use cases:
- Large language models (70B+ parameters)
- Multi-GPU training
- High-throughput batch inference

#### Full node (no partitioning)

All GPUs allocated as a single resource:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-full
spec:
  displayName: "MI300X Full Node"
  profileName: mi300x-8gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "1"
```

Use cases:
- Extremely large models requiring full node VRAM
- Multi-node distributed training
- Jobs that need maximum memory bandwidth

### MI325X profiles

Similar patterns apply to MI325X nodes. Adjust the `targetSelector` and `expectedResources` to match MI325X characteristics:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi325x-quad
spec:
  displayName: "MI325X Quad GPU"
  profileName: mi325x-4gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI325X
  expectedResources:
    amd.com/gpu: "2"
```

### Mixed GPU environments

When managing clusters with multiple GPU models, create separate profiles for each model and use `targetSelector` to enforce compatibility:

```yaml
---
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: inference-partition-mi300x
spec:
  displayName: "Inference Partition (MI300X)"
  profileName: mi300x-4gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "2"
---
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: inference-partition-mi325x
spec:
  displayName: "Inference Partition (MI325X)"
  profileName: mi325x-4gpu
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI325X
  expectedResources:
    amd.com/gpu: "2"
```

Reference both profiles in your plan:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningPlan
metadata:
  name: partition-inference-pool
spec:
  rules:
  - selector:
      matchLabels:
        workload-type: inference
        gpu.amd.com/model: MI300X
    profileRef:
      name: inference-partition-mi300x
  - selector:
      matchLabels:
        workload-type: inference
        gpu.amd.com/model: MI325X
    profileRef:
      name: inference-partition-mi325x
```

## Validation and guardrails

### Target selector

The `targetSelector` field prevents accidental misconfiguration by validating that nodes match specific labels before applying the profile. This is especially important in mixed-GPU environments.

#### GPU model validation

Ensure profiles are only applied to compatible GPU models:

```yaml
spec:
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
```

#### Workload type validation

Combine with workload labels to prevent development profiles from being applied to production nodes:

```yaml
spec:
  targetSelector:
    matchLabels:
      environment: production
      gpu.amd.com/model: MI300X
```

#### Multiple label requirements

Use `matchExpressions` for complex validation:

```yaml
spec:
  targetSelector:
    matchExpressions:
    - key: gpu.amd.com/model
      operator: In
      values:
      - MI300X
      - MI300A
    - key: environment
      operator: NotIn
      values:
      - deprecated
```

### Expected resources

The `expectedResources` field defines what should appear in `node.status.allocatable` after successful partitioning. The verification phase compares actual allocatable resources against this map.

#### Basic GPU count validation

Verify the exact number of GPU resources:

```yaml
spec:
  expectedResources:
    amd.com/gpu: "4"
```

#### Multiple resource validation

Validate additional resources if your partition configuration affects memory or other allocations:

```yaml
spec:
  expectedResources:
    amd.com/gpu: "4"
    amd.com/gpu-memory: "128Gi"
```

**Note**: The controller performs exact quantity matching. If `expectedResources` specifies `"4"`, the verification fails if the node reports `"3"` or `"5"`.

### Validation behavior

When the controller applies a profile to a node:

1. **Target validation** (during planning): If `targetSelector` is specified, the controller checks node labels. Nodes that do not match are skipped with a `ProfileMismatch` condition.

2. **Resource verification** (during verification phase): If `expectedResources` is specified, the controller compares `node.status.allocatable` against the expected values. Mismatches cause the verification phase to fail.

If validation fails:
- The `NodePartitioning` resource transitions to `Failed`
- A condition describes the validation error
- The node remains cordoned and tainted
- Manual intervention is required

## Lifecycle management

### Creating profiles

Profiles can be created before or after plans reference them. The plan controller tolerates missing profiles and reports a `Degraded` state until the profile appears.

Create a profile:

```bash
kubectl apply -f profile.yaml
```

Verify creation:

```bash
kubectl get partitioningprofile
kubectl describe partitioningprofile <name>
```

### Updating profiles

Updating a profile does **not** automatically trigger re-partitioning of nodes already using that profile. Profile changes only affect new nodes or nodes that have not yet completed partitioning.

To apply an updated profile to existing nodes:

1. Update the profile.
2. Delete affected `NodePartitioning` resources (or the entire plan and recreate it).
3. The plan controller recreates `NodePartitioning` resources with the new profile configuration.

**Important**: Treat profiles as immutable once referenced by a plan. To change configurations, create a new profile with a different name and update the plan to reference it.

### Deleting profiles

Deleting a profile while a plan references it causes the plan to enter a `Degraded` state. Existing `NodePartitioning` resources continue executing with the cached profile data, but new nodes cannot begin partitioning.

To safely delete a profile:

1. Ensure no plans reference the profile:
   ```bash
   kubectl get partitioningplan -o yaml | grep -A2 profileRef
   ```

2. Delete the profile:
   ```bash
   kubectl delete partitioningprofile <name>
   ```

## Troubleshooting

### Profile not found

**Symptom**: Plan reports `Degraded` phase with condition `ProfileNotFound`.

**Cause**: The plan references a profile that does not exist.

**Resolution**:
1. Verify the profile name in the plan matches the profile resource:
   ```bash
   kubectl get partitioningprofile
   ```

2. Create the missing profile or fix the reference in the plan.

### DCM profile missing

**Symptom**: Node partitioning fails during the `Applying` phase with a message about missing DCM configuration.

**Cause**: The `profileName` does not exist in the DCM ConfigMap.

**Resolution**:
1. Verify the DCM ConfigMap contains the profile:
   ```bash
   kubectl -n kube-amd-gpu get configmap config-manager-config -o jsonpath='{.data.config\.json}' | jq '.["gpu-config-profile"]'
   ```

2. Add the missing profile to the ConfigMap or correct the `profileName` in the `PartitioningProfile`.

### Target selector mismatch

**Symptom**: Nodes are not selected by the plan, or `NodePartitioning` resources are created but immediately fail.

**Cause**: Node labels do not match the profile's `targetSelector`.

**Resolution**:
1. Check node labels:
   ```bash
   kubectl get node <name> --show-labels
   ```

2. Either add the required labels to the node or remove the `targetSelector` from the profile.

### Expected resources mismatch

**Symptom**: Node partitioning reaches the `Verifying` phase but never succeeds. Condition `Verified=False` with reason `GPUResourcesNotFound` or `AllocatableMismatch`.

**Cause**: The node's `status.allocatable` does not match the profile's `expectedResources`.

**Resolution**:
1. Check the node's allocatable resources:
   ```bash
   kubectl get node <name> -o jsonpath='{.status.allocatable}' | jq
   ```

2. Compare against the profile's `expectedResources`. If the DCM configuration is correct but quantities differ, update the profile's `expectedResources` to match reality.

3. If allocatable resources are incorrect, investigate the AMD GPU operator:
   ```bash
   kubectl -n kube-amd-gpu logs -l app=amd-gpu-device-plugin
   ```

### Device plugin not ready

**Symptom**: Node partitioning stuck in `WaitingOperator` phase with condition `OperatorReady=False`.

**Cause**: The AMD GPU device plugin is not running or not ready on the node.

**Resolution**:
1. Check device plugin pods:
   ```bash
   kubectl -n kube-amd-gpu get pods -o wide | grep <node-name>
   ```

2. View device plugin logs:
   ```bash
   kubectl -n kube-amd-gpu logs <device-plugin-pod-name>
   ```

3. Ensure the device plugin DaemonSet tolerates the `amd-dcm` taint:
   ```bash
   kubectl -n kube-amd-gpu get daemonset amd-gpu-device-plugin -o jsonpath='{.spec.template.spec.tolerations}'
   ```

   The DaemonSet should include:
   ```yaml
   tolerations:
   - key: amd-dcm
     operator: Exists
     effect: NoExecute
   ```

## Best practices

### Naming conventions

Use descriptive names that include:
- GPU model (e.g., `mi300x`, `mi325x`)
- Partition count or GPU allocation (e.g., `quad`, `dual`, `single`)
- Optional: workload type (e.g., `inference`, `training`)

Examples:
- `mi300x-quad-inference`
- `mi325x-dual-training`
- `mi300x-max-partition-dev`

### Version management

Treat profiles as versioned artifacts:

```yaml
metadata:
  name: mi300x-quad-v2
  labels:
    version: "v2"
spec:
  displayName: "MI300X Quad GPU (v2)"
  # ...
```

When updating configurations, create a new profile (`v3`) and migrate plans rather than editing the existing profile in place.

### Documentation

Add annotations to profiles describing their purpose:

```yaml
metadata:
  name: mi300x-quad-inference
  annotations:
    description: "4-way partition optimized for LLM inference workloads"
    recommended-use: "Models up to 70B parameters"
    owner-team: "ml-infrastructure"
```

### Testing

Before deploying a new profile to production:

1. **Validate on a test node**: Create a plan targeting a single test node with the new profile.

2. **Verify allocatable resources**: Ensure the node reports the expected GPU count after partitioning.

3. **Run workload validation**: Schedule a test workload to confirm GPU allocation works correctly.

4. **Rollout gradually**: Use `maxParallel: 1` for the initial production rollout, then increase once confident.

## Related documentation

- [GPU Partitioning](./gpu-partitioning.md) - Main partitioning documentation
- [AMD GPU Operator](https://github.com/AMD/gpu-operator) - DCM configuration reference
