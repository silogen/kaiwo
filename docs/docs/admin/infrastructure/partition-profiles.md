# Partition Profile Reference

Partition profiles describe how the AMD GPU operator should configure hardware on a node (for example "CPX" partitioning versus "SPX"). Profiles are now embedded directly inside each `PartitioningPlan` rule, so you no longer need to manage a separate `PartitioningProfile` resource. This page explains the inline profile schema, how it interacts with the AMD DCM ConfigMap, and provides ready-to-use snippets.

## Inline profile schema

Each `PartitioningRule` in a plan includes a `profile` block:

```yaml
rules:
  - description: Partition MI300X workers for inference
    selector:
      matchLabels:
        role: inference
        gpu.amd.com/model: MI300X
    profile:
      dcmProfileName: cpx
      description: "MI300X CPX partition"
      expectedResources:
        amd.com/gpu: "4"
```

The `profile` object has three fields:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `dcmProfileName` | string (required) | Name of the profile defined in the AMD GPU operator's DCM ConfigMap (`config-manager-config` in `kube-amd-gpu`). The controller sets the `dcm.amd.com/gpu-config-profile` node label to this value. |
| `description` | string | Optional free-form description used for documentation. |
| `expectedResources` | map[string]Quantity | Optional verification hints. During the `Verifying` phase the controller ensures `node.status.allocatable` exposes these resource quantities before marking success. |

> **Tip:** If you omit `expectedResources`, the controller still validates that the `dcm.amd.com/gpu-config-profile` label matches `dcmProfileName`, but it skips deeper resource verification.

## Working with DCM profiles

The inline `dcmProfileName` must match a profile defined in the AMD GPU operator's Device Configuration Manager (DCM) ConfigMap. To list available profiles:

```bash
kubectl -n kube-amd-gpu get configmap config-manager-config -o jsonpath='{.data.config\.json}' | jq '."gpu-config-profile" | keys'
```

If you add custom profiles to the ConfigMap, reference them directly via `dcmProfileName`. The controller currently assumes the profile exists and will surface an error only when the DCM operator fails to apply the configuration.

## Example profiles

### CPX partition

```yaml
profile:
  dcmProfileName: cpx
  description: "Expose four logical GPUs per MI300X"
  expectedResources:
    amd.com/gpu: "4"
```

### SPX (unpartitioned)

```yaml
profile:
  dcmProfileName: spx
  description: "Return node to full MI300X capacity"
  expectedResources:
    amd.com/gpu: "8"
```

### Dry-run validation profile

Use dry-run mode to preview the nodes a plan would touch without updating labels:

```yaml
spec:
  dryRun: true
  rules:
    - selector:
        matchExpressions:
          - key: amd.com/gpu
            operator: Exists
      profile:
        dcmProfileName: default
```

Because the plan runs in dry-run mode, the controller marks each `NodePartitioning` as `Skipped` and leaves the node untouched. This is useful for change reviews.

## Updating a plan to a new profile

To roll out a new partition configuration:

1. Update the `profile` block in the relevant rule with the new `dcmProfileName` (and optional `expectedResources`).
2. Apply the plan manifest. The controller recomputes the desired hash, transitions matching nodes back through the state machine, and reapplies the new profile.
3. Monitor the plan's `status.summary` and the corresponding `NodePartitioning` resources to ensure the rollout completes.

Because profiles are now embedded in plans, you can manage them alongside the selectors and rollout policy in a single manifest.
