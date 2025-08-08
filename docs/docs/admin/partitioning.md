# GPU Partitioning

Kaiwo supports dynamically partitioning AMD data center GPUs.

## Prerequisites

### AMD GPU Operator

The AMD GPU operator v1.3.0 must be deployed.

### Device Config Manager (DCM)

The DCM feature must be [enabled in the operator](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/dcm/device-config-manager.html#configuring-the-device-config-manager) by specifying the following configuration in the DeviceConfig custom resource:

```yaml
configManager:
  # To enable/disable the metrics exporter, enable to partition
  enable: True
  # This specifies the name of the config map which holds the partitioning profiles
  config: 
    name: "config-manager-config"
```

### Config Manager Config Map

The partitioning profiles are stored in a config map named `config-manager-config` in the `kube-amd-gpu` namespace. You can either create this yourself, or let the Kaiwo operator create the default profiles for you.

If you create one yourself, you must include the profiles `spx` and `cpx`, but you can freely modify their contents. Note however that the result may be different than expected, if the contents do not specify correct partitioning profiles. If you do not include these profiles but request them in the partitioning profile field, the partitioning process will fail.

The default that Kaiwo uses is:

```json
{
  "gpu-config-profiles": {
    "cpx": {
      "skippedGPUs": {
        "ids": []
      },
      "profiles": [
        {
          "computePartition": "CPX",
          "memoryPartition": "NPS4",
          "numGPUsAssigned": 8
        }
      ]
    },
    "spx": {
      "skippedGPUs": {
        "ids": []
      },
      "profiles": [
        {
          "computePartition": "SPX",
          "memoryPartition": "NPS1",
          "numGPUsAssigned": 8
        }
      ]
    }
  }
}
```

## Enabling Partitioning

There are two primary ways to enable partitioning on a node. In either case, you can monitor the partitioining status and progress by inspecting the proxy KaiwoNode custom resource.

### Via Node Labels

You can directly label a node that you want to partition by adding the labels:

- **`kaiwo.silogen.ai/node.amd.partitioning.enabled=true`** to enable the partitioning feature

- **`kaiwo.silogen.ai/node.amd.partitioning.profile=spx|cpx`** to set the particular partitioning profile that you want

This will update the proxy KaiwoNode, and if the currently applied profile is different than the requested one, trigger the partitioning process.

### Via KaiwoNode Spec

You can also directly edit the proxy KaiwoNode spec to enable partitioning:

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoNode
metadata:
  name: my-node-name
spec:
  partitioning:
    enabled: true
    profile: cpx
```

!!!warning
    Note that the node label takes priority, and the node label is used to overwrite any value in the KaiwoNode partitioning spec.

## Partitioning Considerations

There are several important considerations when enabling dynamic partitioning.

### Node Tainting

In order to ensure no pods are using the GPUs during the partitioning process, the node is tainted before partitioning starts. The Kaiwo operator automatically adds tolerations to all Deployments and DaemonSets in the `kube-system` namespace, however if you have any critical resources that must remain running, ensure that they have the following tolerations set:

```yaml
tolerations:
  - effect: NoExecute
    key: amd-dcm
    operator: Equal
    value: up
  - effect: NoExecute
    key: kaiwo-partitioning
    operator: ExistsExists
```

### Partitioning Control Plane Nodes

Due to the tainting process, care should be taken if control plane nodes are partitioned. Ensure that all resources that need to stay running have the appropriate tolerations set.


