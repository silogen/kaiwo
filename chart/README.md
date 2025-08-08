# Kaiwo Operator Helm Chart

This Helm chart deploys the Kaiwo (AI Workload Orchestrator) operator to your Kubernetes cluster.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.8+
- cert-manager (recommended)

## Installation

### Quick Start

```bash
# Build and package the chart
make helm-package

# Install with default values
make helm-install
```

### Manual Installation

```bash
# Install directly from source
helm install kaiwo ./chart

# Or install from packaged chart
helm install kaiwo dist/kaiwo-operator-0.1.0.tgz
```

### Custom Configuration

```bash
# Override image and replicas
helm install kaiwo ./chart \
  --set image.tag=v1.0.0 \
  --set replicas=2

# Enable configurations
helm install kaiwo ./chart \
  --set kaiwoConfig.enabled=true \
  --set kaiwoQueueConfig.enabled=true

# Use example values files
helm install kaiwo ./chart -f examples/helm-values/with-configurations.yaml
helm install kaiwo ./chart -f examples/helm-values/minimal-config.yaml

# Production deployment
helm install kaiwo ./chart -f examples/helm-values/production.yaml
```

## Configuration

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Container registry | `ghcr.io` |
| `image.repository` | Image repository | `silogen/kaiwo-operator` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `namespace` | Installation namespace | `kaiwo-system` |
| `replicas` | Controller replicas | `1` |
| `resources.requests.cpu` | CPU requests | `500m` |
| `resources.requests.memory` | Memory requests | `1Gi` |
| `resources.limits.memory` | Memory limits | `4Gi` |
| `nodeSelector` | Node selector | `{}` |
| `affinity` | Pod affinity | `{}` |
| `tolerations` | Pod tolerations | `[]` |

### Optional Configurations

#### KaiwoConfig (Global Operator Settings)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `kaiwoConfig.enabled` | Create KaiwoConfig resource | `false` |
| `kaiwoConfig.spec.ray.defaultRayImage` | Default Ray container image | `ghcr.io/silogen/rocm-ray:6.4` |
| `kaiwoConfig.spec.data.defaultStorageClass` | Default storage class | `""` |
| `kaiwoConfig.spec.nodes.defaultGpuResourceKey` | GPU resource key | `amd.com/gpu` |
| `kaiwoConfig.spec.resourceMonitoring.enabled` | Enable resource monitoring | `false` |

#### KaiwoQueueConfig (Kueue Resource Management)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `kaiwoQueueConfig.enabled` | Create KaiwoQueueConfig resource | `false` |
| `kaiwoQueueConfig.spec.clusterQueues` | List of cluster queues to create | `[]` |
| `kaiwoQueueConfig.spec.resourceFlavors` | List of resource flavors to create | `[]` |
| `kaiwoQueueConfig.spec.workloadPriorityClasses` | List of priority classes | `[]` |

### Configuration Behavior

**Default Behavior (configurations disabled):**
- The operator creates default configurations automatically
- Suitable for quick setup and development
- Uses sensible defaults for most environments

**Custom Configurations (enabled):**
- `KaiwoConfig`: Provides global operator settings (Ray images, storage paths, GPU settings, etc.)
- `KaiwoQueueConfig`: Defines Kueue resources (cluster queues, resource flavors, priorities)
- These override operator defaults and provide fine-grained control
- Required for production environments with specific requirements

**Important Notes:**
- If configurations are enabled, they become the source of truth
- The operator will reconcile Kueue resources to match the KaiwoQueueConfig spec
- Both configurations are cluster-scoped (not namespaced)
- Only one of each configuration type should exist per cluster

## Upgrading

```bash
# Upgrade with new image
helm upgrade kaiwo ./chart --set image.tag=v1.1.0

# Upgrade from packaged chart
helm upgrade kaiwo dist/kaiwo-operator-0.1.0.tgz
```

## Uninstalling

```bash
make helm-uninstall
# or
helm uninstall kaiwo
```

## Development

```bash
# Generate templates for inspection
make helm-template

# Lint the chart
helm lint ./chart

# Test the chart
helm test kaiwo
```