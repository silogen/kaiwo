# GPU Partitioning

GPU partitioning enables fine-grained control over GPU resource allocation in AMD GPU clusters. The partitioning system dynamically applies partition configurations to nodes, automatically draining workloads, reconfiguring GPUs, and verifying the new configuration before returning nodes to service.

This document describes the partitioning resources, explains the orchestration model, details the lifecycle state machine, and provides operational guidance for managing GPU partitions at scale.

## Overview

The GPU partitioning system consists of three Custom Resource Definitions (CRDs):

- **`PartitioningProfile`** defines a reusable GPU partition configuration that specifies how GPUs should be divided.
- **`PartitioningPlan`** orchestrates the application of profiles across a set of nodes using label selectors and rollout policies.
- **`NodePartitioning`** represents a per-node work item that executes the state machine for applying a partition profile to a single node.

The controller automatically creates `NodePartitioning` resources when a plan matches nodes, then executes a multi-phase state machine that drains the node, applies the partition configuration, waits for the AMD GPU operator to reconfigure, verifies the result, and returns the node to service.

## PartitioningProfile

`PartitioningProfile` defines a reusable GPU partition configuration. Profiles are cluster-scoped and can be referenced by multiple plans.

### Specification

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-4-partition
spec:
  displayName: "MI300X 4-Way Partition"
  profileName: mi300x-4way
  targetSelector:
    matchLabels:
      gpu.amd.com/model: MI300X
  expectedResources:
    amd.com/gpu: "4"
```

### Fields

| Field | Type | Description |
| ----- | ---- | ----------- |
| `displayName` | string | Human-readable description displayed in tools and dashboards. |
| `profileName` | string | Name of the DCM profile to apply. This must match a profile defined in the AMD GPU operator's DCM ConfigMap (`config-manager-config` in the `kube-amd-gpu` namespace). |
| `targetSelector` | LabelSelector | Optional node selector that validates nodes before applying this profile. When specified, the controller ensures nodes match these labels. Use this as a guardrail to prevent applying MI300X profiles to MI325X nodes. |
| `expectedResources` | map[string]Quantity | Resources expected in `node.status.allocatable` after successful partitioning. The verification phase checks these values before marking the operation as complete. |

### DCM profile integration

The `profileName` field references a partition configuration in the AMD GPU operator's DCM (Device Configuration Manager) ConfigMap. The controller sets the `dcm.amd.com/gpu-config-profile` label on nodes to trigger the operator's reconfiguration logic.

To list available DCM profiles:

```bash
kubectl -n kube-amd-gpu get configmap config-manager-config -o yaml
```

The DCM ConfigMap contains a `config.json` field with profile definitions. Ensure your `profileName` matches one of the defined profiles. The controller does not validate profile existence at profile creation time—validation occurs when the profile is applied to a node.

### Example profiles

Single GPU per partition:
```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-single
spec:
  displayName: "MI300X Single GPU"
  profileName: mi300x-1gpu
  expectedResources:
    amd.com/gpu: "8"
```

Two GPUs per partition:
```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-dual
spec:
  displayName: "MI300X Dual GPU"
  profileName: mi300x-2gpu
  expectedResources:
    amd.com/gpu: "4"
```

## PartitioningPlan

`PartitioningPlan` orchestrates the application of partition profiles across multiple nodes. Plans use label selectors to identify target nodes and rollout policies to control how many nodes change simultaneously.

### Specification

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningPlan
metadata:
  name: partition-inference-nodes
spec:
  paused: false
  rollout:
    maxParallel: 2
    maxUnavailable: 1
    excludeControlPlane: true  # Default: protects control plane nodes
  rules:
  - name: mi300x-inference
    selector:
      matchLabels:
        workload-type: inference
        gpu.amd.com/model: MI300X
    profileRef:
      name: mi300x-4-partition
  - name: mi325x-inference
    selector:
      matchLabels:
        workload-type: inference
        gpu.amd.com/model: MI325X
    profileRef:
      name: mi325x-8-partition
```

### Fields

| Field | Type | Description |
| ----- | ---- | ----------- |
| `paused` | bool | When true, the controller stops creating new `NodePartitioning` resources and pauses reconciliation. Existing node operations continue to completion. Use this to halt a rollout without deleting the plan. |
| `dryRun` | bool | When true, the controller creates `NodePartitioning` resources but marks them as skipped. Use this to preview which nodes would be affected before committing to the operation. |
| `rollout` | RolloutPolicy | Controls orchestration behavior. See [Rollout policies](#rollout-policies) for details. |
| `rules` | []PartitioningRule | Ordered list of rules mapping nodes to profiles. The controller evaluates rules in order and applies the first matching rule to each node. |

### PartitioningRule

Each rule consists of a node selector and a profile reference:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | Optional human-readable identifier for the rule. |
| `selector` | LabelSelector | Kubernetes label selector identifying target nodes. |
| `profileRef.name` | string | Name of the `PartitioningProfile` to apply. |

### Rollout policies

The `rollout` field controls how many nodes undergo partitioning simultaneously:

| Field | Default | Description |
| ----- | ------- | ----------- |
| `maxParallel` | 1 | Maximum number of nodes to process concurrently. Higher values speed up cluster-wide rollouts but increase the blast radius of configuration errors. |
| `maxUnavailable` | 1 | Maximum number of nodes allowed to be unavailable at once. A node is considered unavailable while in `Draining`, `Applying`, `WaitingOperator`, `Verifying`, or `Failed` phases. This prevents the plan from draining too many nodes simultaneously. |
| `excludeControlPlane` | true | Protects control plane nodes from being partitioned. When true (default), nodes with the `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` label are automatically excluded from all rules. Set to false to allow partitioning control plane nodes. |

**Note**: The controller enforces both limits. If either `maxParallel` or `maxUnavailable` is reached, no additional nodes begin partitioning until existing operations complete or fail.

### Plan phases

Plans report an overall phase in `status.phase`:

| Phase | Description |
| ----- | ----------- |
| `Pending` | Plan created but no nodes have started partitioning yet. |
| `Progressing` | At least one node is actively being partitioned (Draining, Applying, WaitingOperator, or Verifying). |
| `Completed` | All matching nodes have successfully reached the `Succeeded` phase. |
| `Degraded` | At least one node has entered the `Failed` phase or a resource reference (profile) is invalid. |
| `Paused` | The plan's `spec.paused` field is set to true. |

### Status summary

The plan aggregates per-node status counts in `status.summary`:

```yaml
status:
  phase: Progressing
  summary:
    matchingNodes: 10
    totalNodes: 10
    pending: 2
    applying: 2
    verifying: 1
    succeeded: 4
    failed: 1
  nodeStatuses:
  - nodeName: gpu-node-01
    phase: Succeeded
    desiredHash: sha256:a1b2c3...
    currentHash: sha256:a1b2c3...
  - nodeName: gpu-node-02
    phase: Applying
    desiredHash: sha256:a1b2c3...
    currentHash: ""
```

| Field | Description |
| ----- | ----------- |
| `matchingNodes` | Total number of nodes selected by any rule in the plan. |
| `totalNodes` | Number of `NodePartitioning` resources owned by this plan. |
| `pending` | Nodes in `Pending` phase. |
| `applying` | Nodes in `Draining`, `Applying`, or `WaitingOperator` phases. |
| `verifying` | Nodes in `Verifying` phase. |
| `succeeded` | Nodes in `Succeeded` phase. |
| `failed` | Nodes in `Failed` phase. |
| `skipped` | Nodes skipped due to dry-run mode. |

The `nodeStatuses` array provides a lightweight cache for dashboards. The source of truth is in the individual `NodePartitioning` resources.

## NodePartitioning

`NodePartitioning` represents a per-node work item created by the `PartitioningPlan` controller. Each resource executes a state machine that drains the node, applies the partition configuration, waits for the GPU operator to reconfigure, verifies the result, and returns the node to service.

Users typically do not create `NodePartitioning` resources manually—they are managed by plans. However, understanding the state machine is essential for troubleshooting and monitoring.

### Specification

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: NodePartitioning
metadata:
  name: partition-inference-nodes-gpu-node-01
spec:
  planRef:
    name: partition-inference-nodes
    uid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
  nodeName: gpu-node-01
  desiredHash: sha256:a1b2c3d4e5f6...
  profileRef:
    name: mi300x-4-partition
```

### State machine

The controller executes a multi-phase state machine for each node. Each phase represents a distinct step in the partitioning lifecycle:

```
     ┌─────────┐
     │         │
     │ Pending │
     │         │
     └────┬────┘
          │
          v
     ┌─────────┐
     │         │
     │ Draining│──┐
     │         │  │
     └────┬────┘  │
          │       │
          v       │
     ┌─────────┐  │
     │         │  │
     │ Applying│  │
     │         │  │
     └────┬────┘  │
          │       │ Error
          v       │
  ┌───────────────┐│
  │               ││
  │WaitingOperator││
  │               ││
  └───────┬───────┘│
          │        │
          v        │
     ┌─────────┐   │
     │         │   │
     │Verifying│   │
     │         │   │
     └────┬────┘   │
          │        │
          v        v
     ┌─────────┐┌────────┐
     │         ││        │
     │Succeeded││ Failed │
     │         ││        │
     └─────────┘└────────┘
```

### Phases

| Phase | Description |
| ----- | ----------- |
| `Pending` | Initial state after creation. The controller transitions to `Draining` on the next reconcile. |
| `Draining` | Node is being cordoned, tainted, and drained. The controller cordons the node (`spec.unschedulable=true`), applies the `amd-dcm=up:NoExecute` taint, and evicts all pods that do not tolerate the taint. Static pods and DaemonSets with appropriate tolerations remain running. |
| `Applying` | The controller ensures the partition profile is present in the DCM ConfigMap and applies the `dcm.amd.com/gpu-config-profile` label to the node. This triggers the AMD GPU operator to reconfigure GPUs. |
| `WaitingOperator` | Waiting for the AMD GPU device plugin to restart and become ready on the node. The controller monitors device plugin pods in the `kube-amd-gpu` namespace. |
| `Verifying` | Verifying that the partition configuration is correct. The controller checks that the `dcm.amd.com/gpu-config-profile` label matches the profile and that `node.status.allocatable` contains the expected GPU resources. |
| `Succeeded` | Partition configuration applied and verified successfully. The controller untaints and uncordons the node, allowing workloads to schedule. |
| `Failed` | An error occurred during the state machine execution. The node remains cordoned and tainted. Manual intervention is required. |

### Non-blocking reconciliation

The controller is designed to perform fast, non-blocking checks:

- Each reconcile performs **one state transition only**, then returns to allow status updates.
- All operations are **idempotent**—the controller checks conditions before taking action and skips work that is already complete.
- **No polling or blocking waits**—the controller relies on Kubernetes watch events to trigger reconciliation when node state changes (DCM label changes, allocatable resource changes, pod readiness).

This design ensures the controller remains responsive and does not hold locks or block API server operations.

### Drain behavior

During the `Draining` phase, the controller:

1. **Cordons the node** by setting `spec.unschedulable=true`, preventing new pods from scheduling.
2. **Applies a taint** (`amd-dcm=up:NoExecute`), causing pods without a matching toleration to be evicted.
3. **Evicts pods** that do not tolerate the taint. Pods with the toleration (such as monitoring DaemonSets) remain running.

The following pod types are **not evicted**:

- Static pods (mirror pods created by kubelet)
- Pods that tolerate the `amd-dcm` taint with effect `NoExecute`

The drain operation is idempotent and fast. The controller returns immediately after initiating evictions—it does not wait for all pods to terminate. Kubernetes handles termination asynchronously.

### Verification checks

During the `Verifying` phase, the controller performs the following checks:

1. **DCM label verification**: Ensures `node.labels["dcm.amd.com/gpu-config-profile"]` matches the expected profile name.
2. **Allocatable resource verification**: Checks that `node.status.allocatable` contains `amd.com/gpu` resources. If the profile specifies `expectedResources`, the controller validates the exact quantity.

If either check fails, the controller sets the `Verified` condition to `False` and requeues. The verification phase does not time out—it waits indefinitely for the operator to complete reconfiguration. This prevents premature failure when operator reconfiguration takes longer than expected.

### Conditions

Each `NodePartitioning` resource maintains several status conditions:

| Condition | Meaning |
| --------- | ------- |
| `NodeCordoned` | True when the node is successfully cordoned. |
| `NodeTainted` | True when the `amd-dcm` taint has been applied. |
| `DrainCompleted` | True when all non-tolerated pods have been evicted. |
| `ProfileApplied` | True when the DCM profile label has been set on the node. |
| `OperatorReady` | True when the AMD GPU device plugin is running and ready. |
| `Verified` | True when the partition configuration has been verified. |

Inspect conditions to understand exactly which step of the process succeeded or failed:

```bash
kubectl get nodepartitioning <name> -o yaml
```

### Retry and failure handling

The controller does not automatically retry failed operations. If a node enters the `Failed` phase, manual investigation is required:

1. Inspect the `NodePartitioning` status conditions to identify which step failed.
2. Check the node's current state (`kubectl describe node <name>`).
3. Verify the AMD GPU operator logs for errors.
4. Once the issue is resolved, delete the `NodePartitioning` resource. The plan controller will recreate it and retry the operation.

## Control plane protection

The `excludeControlPlane` field provides automatic protection against accidentally partitioning control plane nodes. When enabled (the default), the controller automatically filters out nodes with control plane labels before applying any rules.

**Default behavior**: Control plane nodes are protected even if you omit the `rollout` section entirely. The controller defaults to `excludeControlPlane: true` for safety.

### How it works

The controller checks for the following standard Kubernetes labels:
- `node-role.kubernetes.io/control-plane` (modern clusters)
- `node-role.kubernetes.io/master` (older clusters)

When `excludeControlPlane: true`, nodes with either label are excluded from all plan rules, regardless of whether the rule's selector would otherwise match. This happens transparently—the controller logs excluded nodes at debug level but does not create `NodePartitioning` resources for them.

### When to disable

In most production environments, you should keep this protection enabled. However, you may need to disable it in specific scenarios:

- **Development clusters**: Single-node clusters where the control plane also runs workloads
- **Edge deployments**: Resource-constrained environments where control plane nodes have GPUs
- **Testing**: Validating partition configurations on control plane hardware

To disable protection:

```yaml
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningPlan
metadata:
  name: dev-cluster-partition
spec:
  rollout:
    excludeControlPlane: false  # Allows partitioning control plane nodes
  rules:
  - selector:
      matchLabels:
        gpu.present: "true"
    profileRef:
      name: dev-profile
```

**Warning**: Partitioning control plane nodes can disrupt cluster operations. Ensure control plane components tolerate the `amd-dcm` taint or do not require GPU access before proceeding.

### Verifying control plane labels

Check which nodes have control plane labels:

```bash
# Modern label
kubectl get nodes -l node-role.kubernetes.io/control-plane

# Legacy label
kubectl get nodes -l node-role.kubernetes.io/master
```

If your control plane nodes lack these labels, add them for protection:

```bash
kubectl label node <node-name> node-role.kubernetes.io/control-plane=
```

## Operational workflows

### Applying a new partition profile

Create a profile and a plan:

```bash
# Create the profile
kubectl apply -f - <<EOF
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningProfile
metadata:
  name: mi300x-quad
spec:
  displayName: "MI300X 4-Way Partition"
  profileName: mi300x-4gpu
  expectedResources:
    amd.com/gpu: "4"
EOF

# Create a plan targeting specific nodes
kubectl apply -f - <<EOF
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningPlan
metadata:
  name: partition-inference-pool
spec:
  rollout:
    maxParallel: 2
    maxUnavailable: 1
  rules:
  - selector:
      matchLabels:
        node-pool: inference
    profileRef:
      name: mi300x-quad
EOF
```

Monitor progress:

```bash
# Watch plan status
kubectl get partitioningplan partition-inference-pool -w

# List per-node status
kubectl get nodepartitioning -l plan=partition-inference-pool

# Check detailed status
kubectl describe partitioningplan partition-inference-pool
```

### Previewing changes with dry-run

Before applying a plan to production nodes, preview which nodes will be affected:

```bash
kubectl apply -f - <<EOF
apiVersion: infrastructure.silogen.ai/v1alpha1
kind: PartitioningPlan
metadata:
  name: preview-partition
spec:
  dryRun: true
  rules:
  - selector:
      matchLabels:
        node-pool: inference
    profileRef:
      name: mi300x-quad
EOF

# Review which nodes matched
kubectl get partitioningplan preview-partition -o yaml
```

Examine `status.nodeStatuses` to see which nodes the plan selected. Once satisfied, set `dryRun: false` to execute the rollout.

### Pausing a rollout

To halt an in-progress rollout without deleting the plan:

```bash
kubectl patch partitioningplan partition-inference-pool \
  --type merge -p '{"spec":{"paused":true}}'
```

Nodes currently undergoing partitioning continue to completion, but no new nodes begin the process. Resume by setting `paused: false`.

### Changing partition profiles

To apply a different profile to the same nodes, update the plan's `profileRef`:

```bash
kubectl patch partitioningplan partition-inference-pool \
  --type merge -p '{"spec":{"rules":[{"profileRef":{"name":"mi300x-dual"}}]}}'
```

The controller computes a new `desiredHash` for each `NodePartitioning` resource. Nodes that have already succeeded with the old profile transition back to `Pending` and re-execute the state machine with the new profile.

### Troubleshooting failed nodes

If a node enters the `Failed` phase:

1. **Check the NodePartitioning status**:
   ```bash
   kubectl describe nodepartitioning <name>
   ```

   Look at the `Conditions` section to identify which step failed.

2. **Inspect the node**:
   ```bash
   kubectl describe node <node-name>
   ```

   Verify labels, taints, and allocatable resources.

3. **Check AMD GPU operator logs**:
   ```bash
   kubectl -n kube-amd-gpu logs -l app=amd-gpu-operator -f
   ```

4. **Verify DCM ConfigMap**:
   ```bash
   kubectl -n kube-amd-gpu get configmap config-manager-config -o yaml
   ```

   Ensure the `profileName` exists in the ConfigMap.

5. **Retry the operation**:
   Once the issue is resolved, delete the `NodePartitioning` resource:
   ```bash
   kubectl delete nodepartitioning <name>
   ```

   The plan controller recreates the resource and retries the operation.

### Manually uncordoning a failed node

If you need to return a failed node to service without completing partitioning:

```bash
# Remove the taint
kubectl taint node <node-name> amd-dcm=up:NoExecute-

# Uncordon the node
kubectl uncordon <node-name>

# Delete the NodePartitioning resource to prevent the controller from re-draining
kubectl delete nodepartitioning <name>
```

**Warning**: This bypasses the verification phase. Ensure the node's GPU configuration is correct before scheduling workloads.

## Status and observability

### Plan status

View plan status:

```bash
kubectl get partitioningplan
kubectl describe partitioningplan <name>
```

The plan's `status.summary` provides aggregated counts by phase. The `status.nodeStatuses` array lists per-node status for quick inspection.

### Per-node status

List all node partitioning operations:

```bash
kubectl get nodepartitioning
```

Filter by plan:

```bash
kubectl get nodepartitioning -l plan=<plan-name>
```

View detailed status:

```bash
kubectl describe nodepartitioning <name>
```

### Events

The controller emits Kubernetes events for key lifecycle transitions:

| Event Type | Reason | Description |
| ---------- | ------ | ----------- |
| Normal | DrainStarted | Node drain initiated. |
| Normal | DrainCompleted | Node successfully drained. |
| Normal | ProfileApplied | DCM profile label set on node. |
| Normal | VerificationSucceeded | Partition configuration verified. |
| Normal | NodeSucceeded | Partitioning completed successfully. |
| Warning | NodeNotFound | Target node does not exist. |
| Warning | ProfileNotFound | Referenced profile does not exist. |
| Warning | StateMachineFailed | State machine execution encountered an error. |

View events:

```bash
kubectl get events --field-selector involvedObject.kind=NodePartitioning
kubectl describe nodepartitioning <name>
```

### Controller logs

The partitioning controllers run in the operator's namespace (typically `kaiwo-system`):

```bash
kubectl -n kaiwo-system logs -l app=partitioning-controller -f
```

Logs include structured fields for filtering:

- `controller`: `node-partitioning` or `partitioning-plan`
- `node`: Node name (for node-partitioning events)
- `phase`: Current phase

## Best practices

### Control plane protection

- **Control planes are protected by default**: Even if you omit the `rollout` section entirely, control plane nodes are automatically excluded. You must explicitly set `rollout.excludeControlPlane: false` to disable this protection.
- **Label control plane nodes explicitly**: Ensure all control plane nodes have the `node-role.kubernetes.io/control-plane` label for proper protection.
- **Separate GPU workloads from control plane**: In production clusters, run GPU workloads on dedicated worker nodes rather than collocating them with control plane components.

### Profile design

- **Use explicit `expectedResources`**: Specify the exact GPU count you expect after partitioning. This ensures verification detects misconfiguration.
- **Apply `targetSelector` guardrails**: Prevent accidental application of MI300X profiles to MI325X nodes by setting label selectors.
- **Test profiles in non-production first**: Create a test plan with `dryRun: true` or `maxParallel: 1` to validate profiles before rolling out cluster-wide.

### Plan orchestration

- **Start with conservative rollout policies**: Use `maxParallel: 1` and `maxUnavailable: 1` until you validate the profile works correctly.
- **Increase parallelism gradually**: Once confident, increase `maxParallel` to speed up large rollouts.
- **Use node labels for targeting**: Label nodes by purpose (e.g., `workload-type: inference`) rather than physical attributes. This allows updating partition profiles without changing node labels.

### Drain tolerance

- **Add tolerations to monitoring DaemonSets**: Ensure observability tools continue running during partitioning by adding:
  ```yaml
  tolerations:
  - key: amd-dcm
    operator: Exists
    effect: NoExecute
  ```

### Change management

- **Preview with dry-run**: Always test a new plan with `dryRun: true` before executing.
- **Pause during incidents**: Use `paused: true` to halt rollouts if issues arise.
- **Monitor node availability**: Track `maxUnavailable` to ensure you do not drain too many nodes simultaneously.

## Limitations

- **Manual intervention required for failures**: The controller does not automatically retry failed nodes. Operators must investigate and delete the `NodePartitioning` resource to retry.
- **No automatic rollback**: If partitioning succeeds but the new configuration causes problems, manually create a new plan with the previous profile.
- **Single profile per node**: A node can only have one active partition profile. Applying a new profile replaces the previous configuration.
- **Cluster-scoped resources**: Plans and profiles are cluster-scoped. RBAC policies must grant appropriate permissions to administrators.

## Related documentation

- [AMD GPU Operator Documentation](https://github.com/AMD/gpu-operator) - Details on DCM profile configuration
- [Kubernetes Taints and Tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) - Background on drain behavior
