# Helm Installation Guide

This guide provides detailed instructions for installing Kaiwo using Helm charts.

## Prerequisites

- Kubernetes cluster (v1.22+ recommended)
- Helm 3.8+ installed
- kubectl configured with cluster-admin privileges
- [Dependencies installed](installation.md#step-1-install-kaiwo-and-its-dependencies) (cert-manager, GPU operator, Kueue, etc.)

## Quick Start

### Install from OCI Registry (Recommended)

```bash
# Install latest version
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator

# Install specific version
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator --version <version>
```

## Configuration Options

### Basic Configuration

```bash
# Override image and resources
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator \
  --set image.tag=v0.1.7 \
  --set replicas=2 \
  --set resources.limits.memory=8Gi
```

### Custom Namespace

```bash
# Install in custom namespace
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator \
  --set namespace=my-kaiwo-system
```

### With Global Configurations

Create a values file to provide operator and queue configurations:

```yaml
# production-values.yaml
image:
  tag: "v0.1.7"

# Enable global operator configuration
kaiwoConfig:
  enabled: true
  spec:
    # Ray settings
    ray:
      defaultRayImage: "ghcr.io/silogen/rocm-ray:6.4"
      headPodMemory: "32Gi"
    
    # Storage configuration
    data:
      defaultStorageClass: "fast-ssd"
      defaultDataMountPath: "/data"
    
    # GPU configuration
    nodes:
      defaultGpuResourceKey: "amd.com/gpu"
      excludeMasterNodesFromNodePools: true
    
    # Resource monitoring
    resourceMonitoring:
      enabled: true
      pollingInterval: "30s"

# Enable queue configuration
kaiwoQueueConfig:
  enabled: true
  spec:
    # Define cluster queues
    clusterQueues:
    - name: main
      spec:
        resourceGroups:
        - flavors:
          - name: gpu-nodes
          resources:
          - name: cpu
            nominalQuota: "200"
          - name: memory
            nominalQuota: "2000Gi"
          - name: amd.com/gpu
            nominalQuota: "8"
      namespaces:
      - default
      - ai-workloads
    
    # Define resource flavors
    resourceFlavors:
    - name: gpu-nodes
      nodeLabels:
        node-type: gpu
        accelerator: amd-mi300x
```

Install with the configuration:

```bash
helm install kaiwo oci://ghcr.io/silogen/kaiwo-operator -f production-values.yaml
```

## Installation Parameters

### Basic Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Container registry | `ghcr.io` |
| `image.repository` | Image repository | `silogen/kaiwo-operator` |
| `image.tag` | Image tag (uses chart's appVersion if empty) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `namespace` | Installation namespace | `kaiwo-system` |
| `replicas` | Controller replicas | `1` |
| `resources.requests.cpu` | CPU requests | `500m` |
| `resources.requests.memory` | Memory requests | `1Gi` |
| `resources.limits.memory` | Memory limits | `4Gi` |

### Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `kaiwoConfig.enabled` | Create KaiwoConfig resource | `false` |
| `kaiwoConfig.spec.ray.defaultRayImage` | Default Ray image | `ghcr.io/silogen/rocm-ray:6.4` |
| `kaiwoConfig.spec.data.defaultStorageClass` | Default storage class | `""` |
| `kaiwoConfig.spec.nodes.defaultGpuResourceKey` | GPU resource key | `amd.com/gpu` |
| `kaiwoQueueConfig.enabled` | Create KaiwoQueueConfig resource | `false` |
| `kaiwoQueueConfig.spec.clusterQueues` | List of cluster queues | `[]` |
| `kaiwoQueueConfig.spec.resourceFlavors` | List of resource flavors | `[]` |

## Upgrading

```bash
# Upgrade to specific version
helm upgrade kaiwo oci://ghcr.io/silogen/kaiwo-operator --version 1.1.0

# Upgrade with new configuration
helm upgrade kaiwo oci://ghcr.io/silogen/kaiwo-operator -f new-values.yaml
```

## Uninstalling

```bash
# Uninstall Kaiwo (keeps CRDs by default)
helm uninstall kaiwo

# Manually remove only Kaiwo CRDs if needed (WARNING: This will delete all data!)
kubectl delete crd \
  kaiwojobs.kaiwo.silogen.ai \
  kaiwoservices.kaiwo.silogen.ai \
  kaiwoqueueconfigs.kaiwo.silogen.ai \
  kaiwonodes.kaiwo.silogen.ai \
  kaiwoconfigs.config.kaiwo.silogen.ai \
  resourceflavors.kaiwo.silogen.ai \
  topologies.kaiwo.silogen.ai
```

!!! danger "Data Loss Warning"
    Deleting CRDs will **permanently delete all associated custom resources** (jobs, services, configurations). Only do this if you want to completely remove Kaiwo and all its data.

## Configuration Behavior

### Default Behavior (configurations disabled)
- The operator creates default configurations automatically
- Suitable for quick setup and development
- Uses sensible defaults for most environments

### Custom Configurations (enabled)
- `KaiwoConfig`: Provides global operator settings (Ray images, storage paths, GPU settings)
- `KaiwoQueueConfig`: Defines Kueue resources (cluster queues, resource flavors, priorities)
- These override operator defaults and provide fine-grained control
- Required for production environments with specific requirements

### Important Notes
- If configurations are enabled, they become the source of truth
- The operator will reconcile Kueue resources to match the KaiwoQueueConfig spec
- Both configurations are cluster-scoped (not namespaced)
- Only one of each configuration type should exist per cluster

## Troubleshooting

### Chart Installation Issues

1. **Verify Helm is properly configured**:
   ```bash
   helm version
   ```

2. **Check if chart exists**:
   ```bash
   helm search repo kaiwo-operator
   # or for OCI
   helm show chart oci://ghcr.io/silogen/kaiwo-operator
   ```

3. **Test chart rendering**:
   ```bash
   helm template kaiwo oci://ghcr.io/silogen/kaiwo-operator --debug
   ```

### Operator Issues

See the main [Troubleshooting Guide](troubleshooting.md) for general operator issues.

## Next Steps

- **Configure cluster resources**: Customize the `KaiwoQueueConfig` to match your cluster's hardware
- **Set up monitoring**: Configure metrics collection and alerting
- **User onboarding**: Provide CLI access to your users following the [User Quickstart](../scientist/quickstart.md)