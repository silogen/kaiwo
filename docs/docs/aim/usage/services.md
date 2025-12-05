# Deploying Inference Services

`AIMService` is the primary resource for deploying inference endpoints. It combines a model image, optional runtime configuration, and HTTP routing to produce a production-ready inference service.

## Quick Start

The minimal service requires just an AIM container image:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
```

This creates an inference service using the default runtime configuration and automatically selected profile.


## Common Configuration

### Scaling

Control the number of replicas:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  replicas: 3
```

### Autoscaling

AIMService supports automatic scaling based on custom metrics using [KEDA](https://keda.sh/) (Kubernetes Event-driven Autoscaling). This enables your inference services to scale dynamically based on real-time demand.

#### Basic Autoscaling

Enable autoscaling by specifying minimum and maximum replica counts:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  minReplicas: 1
  maxReplicas: 5
```

This configures KEDA to manage scaling between 1 and 5 replicas. Without custom metrics, KEDA uses default scaling behavior.

#### Custom Metrics with OpenTelemetry

For precise control over scaling behavior, configure custom metrics from the inference runtime. vLLM exposes metrics via OpenTelemetry that can drive scaling decisions:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  minReplicas: 1
  maxReplicas: 3
  autoScaling:
    metrics:
      - type: PodMetric
        podmetric:
          metric:
            backend: "opentelemetry"
            metricNames:
              - vllm:num_requests_running
            query: "vllm:num_requests_running"
            operationOverTime: "avg"
          target:
            type: Value
            value: "1"
```

This configuration scales based on the average number of running requests across pods. When the average exceeds 1, KEDA scales up; when it drops below, KEDA scales down.

#### Metric Configuration Options

| Field | Description | Default |
|-------|-------------|---------|
| `backend` | Metrics backend to use | `opentelemetry` |
| `serverAddress` | Address of the metrics server | `keda-otel-scaler.keda.svc:4317` |
| `metricNames` | List of metrics to collect from pods | - |
| `query` | Query to retrieve metrics from the backend | - |
| `operationOverTime` | Aggregation operation: `last_one`, `avg`, `max`, `min`, `rate`, `count` | `last_one` |

#### Target Types

| Type | Description | Field |
|------|-------------|-------|
| `Value` | Scale based on absolute metric value | `value` |
| `AverageValue` | Scale based on average value across pods | `averageValue` |
| `Utilization` | Scale based on percentage utilization (resource metrics only) | `averageUtilization` |

#### Common vLLM Metrics

These metrics are commonly used for autoscaling vLLM-based inference services:

| Metric | Description | Scaling Use Case |
|--------|-------------|------------------|
| `vllm:num_requests_running` | Number of requests currently being processed | Scale based on concurrent load |
| `vllm:num_requests_waiting` | Number of requests waiting in queue | Scale based on queue depth |


#### How It Works

When autoscaling is configured, AIMService:

1. Creates a KServe InferenceService with the `serving.kserve.io/autoscalerClass: keda` annotation
2. KEDA creates a `ScaledObject` that monitors the specified metrics
3. KEDA creates and manages an `HorizontalPodAutoscaler` (HPA) based on the ScaledObject
4. The HPA scales the deployment between `minReplicas` and `maxReplicas` based on metric values

#### Monitoring Autoscaling

Check the ScaledObject status:

```bash
kubectl -n <namespace> get scaledobject <service-name>-predictor -o yaml
```

Check the HPA created by KEDA:

```bash
kubectl -n <namespace> get hpa keda-hpa-<service-name>-predictor
```

Watch scaling events in real-time:

```bash
kubectl -n <namespace> get hpa keda-hpa-<service-name>-predictor -w
```

View current metrics:

```bash
kubectl -n <namespace> describe hpa keda-hpa-<service-name>-predictor
```

#### Prerequisites

Autoscaling requires:

- **KEDA** installed in the cluster
- **KEDA OpenTelemetry Scaler** (`keda-otel-scaler`) deployed if using OpenTelemetry metrics
- **OpenTelemetry Collector** configured to scrape metrics from inference pods

### Resource Limits

Override default resource allocations:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  resources:
    limits:
      cpu: "8"
      memory: 64Gi
    requests:
      cpu: "4"
      memory: 32Gi
```

### Runtime Overrides

Customize optimization settings:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  overrides:
    metric: throughput      # or 'latency' for interactive workloads
    precision: fp16         # fp4, fp8, fp16, bf16, int4, int8, auto
    gpuSelector:
      count: 2
      model: MI300X
```

Please note that not all configurations may be supported on each AIM image.

## Runtime Configuration

Reference a specific runtime configuration for credentials and defaults:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  runtimeConfigName: team-config  # defaults to 'default' if omitted
```

Runtime configurations provide:
- Routing defaults

See [Runtime Configuration](runtime-config.md) for details.

## KV Cache

### Creating a New KV Cache

The service automatically creates a KV cache when you specify the `type`:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    type: redis  # Creates 'kvcache-llama-chat' automatically
```

With custom storage:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-70b-instruct:0.7.0
  kvCache:
    type: redis
    storage:
      size: 100Gi                  # Minimum 1Gi, adjust based on model size
      storageClassName: fast-ssd   # Optional, uses cluster default if omitted
```

### Referencing an Existing KV Cache

Multiple services can share a single KV cache:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  kvCache:
    name: shared-cache  # References existing AIMKVCache resource
```

This is useful when running multiple services with the same model or overlapping use cases.

See [KV Cache](kv-cache.md) for detailed configuration and sizing guidance.

## HTTP Routing

Enable external HTTP access through Gateway API:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
```

### Custom Paths

Override the default path using templates:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    pathTemplate: "/{.metadata.namespace}/chat/{.metadata.name}"
```

Templates use JSONPath expressions wrapped in `{...}`:
- `{.metadata.namespace}` - service namespace
- `{.metadata.name}` - service name
- `{.metadata.labels['team']}` - label value (label must exist)

The final path is lowercased, URL-encoded, and limited to 200 characters.

**Note**: If a label or field doesn't exist, the service will enter a degraded state. Ensure all referenced fields are present.

## Authentication

For models requiring authentication (e.g., gated Hugging Face models):

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  env:
    - name: HF_TOKEN
      valueFrom:
        secretKeyRef:
          name: huggingface-creds
          key: token
```

For private container registries:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  imagePullSecrets:
    - name: registry-credentials
```

## Monitoring Service Status

Check service readiness:

```bash
kubectl -n <namespace> get aimservice <name>
```

View detailed status:

```bash
kubectl -n <namespace> describe aimservice <name>
```

### Status Values

The `status` field shows the overall service state:

- **Pending**: Initial state, resolving model and template references
- **Starting**: Creating infrastructure (InferenceService, routing, caches)
- **Running**: Service is ready and serving traffic
- **Degraded**: Service is running but has warnings (e.g., routing issues, template not optimal)
- **Failed**: Service cannot start due to terminal errors

### Status Fields

| Field | Description |
| ----- | ----------- |
| `status` | Overall service status (Pending, Starting, Running, Degraded, Failed) |
| `observedGeneration` | Most recent generation observed by the controller |
| `conditions` | Detailed conditions tracking different aspects of service lifecycle |
| `resolvedRuntimeConfig` | Metadata about the runtime config that was resolved (name, namespace, scope, UID) |
| `resolvedImage` | Metadata about the model image that was resolved (name, namespace, scope, UID) |
| `resolvedTemplate` | Metadata about the template that was selected (name, namespace, scope, UID) |
| `routing` | Observed routing configuration including the rendered HTTP path |

### Conditions

Services track detailed conditions to help diagnose issues:

**Resolved**: Model and template validation
- `True`: Model and template have been validated and a runtime profile has been selected
- `False`: Model or template not found, or validation failed
- Reasons: `Resolved`, `TemplateNotFound`, `ModelNotFound`, `ModelNotReady`, `TemplateSelectionAmbiguous`

**CacheReady**: Model caching status
- `True`: Required caches are present or warmed as requested
- `False`: Caches are not ready
- Reasons: `CacheWarm`, `WaitingForCache`, `CacheWarming`, `CacheFailed`

**RuntimeReady**: KServe InferenceService status
- `True`: The underlying KServe InferenceService is ready and serving traffic
- `False`: InferenceService is not ready
- Reasons: `RuntimeReady`, `CreatingRuntime`, `RuntimeFailed`

**RoutingReady**: HTTP routing status
- `True`: HTTPRoute has been created and routing is configured
- `False`: Routing is not ready or disabled
- Reasons: `RouteReady`, `ConfiguringRoute`, `RouteFailed`, `PathTemplateInvalid`

**Ready**: Overall readiness
- `True`: Service is fully ready to serve traffic (all other conditions are True)
- `False`: Service is not ready

**Progressing**: Active reconciliation
- `True`: Controller is actively reconciling towards readiness
- `False`: Service has reached a stable state

**Failure**: Terminal failures
- `True`: An unrecoverable error has occurred
- Includes detailed reason and message

### Example Status

```bash
$ kubectl -n ml-team get aimservice llama-chat -o yaml
```

```yaml
status:
  status: Running
  observedGeneration: 1
  conditions:
    - type: Resolved
      status: "True"
      reason: Resolved
      message: "Model and template resolved successfully"
    - type: CacheReady
      status: "True"
      reason: CacheWarm
      message: "Model sources cached"
    - type: RuntimeReady
      status: "True"
      reason: RuntimeReady
      message: "InferenceService is ready"
    - type: RoutingReady
      status: "True"
      reason: RouteReady
      message: "HTTPRoute configured"
    - type: Ready
      status: "True"
      reason: Ready
      message: "Service is ready"
  resolvedRuntimeConfig:
    name: default
    namespace: ml-team
    scope: Namespace
    kind: aim.silogen.ai/v1alpha1/AIMRuntimeConfig
  resolvedImage:
    name: meta-llama-3-8b
    namespace: ml-team
    scope: Namespace
    kind: aim.silogen.ai/v1alpha1/AIMModel
  resolvedTemplate:
    name: llama-3-8b-latency
    scope: Cluster
    kind: aim.silogen.ai/v1alpha1/AIMClusterServiceTemplate
  routing:
    path: /ml-team/llama-chat
```

## Complete Example

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
  labels:
    team: research
spec:
  model:
    ref: meta-llama-3-8b
  templateRef: llama-3-8b-latency
  runtimeConfigName: team-config
  replicas: 2
  resources:
    limits:
      cpu: "6"
      memory: 48Gi
  overrides:
    metric: throughput
    precision: fp16
    gpuSelector:
      count: 2
      model: MI300X
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    pathTemplate: "/team/{.metadata.labels['team']}/chat"
  env:
    - name: HF_TOKEN
      valueFrom:
        secretKeyRef:
          name: huggingface-creds
          key: token
```

## Model Caching

Enable model caching to pre-download model artifacts:

```yaml
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  cacheModel: true
```

When `cacheModel: true`:

1. An `AIMTemplateCache` is created for the service's template, if it doesn't already exist
2. The AIMTemplate will create `AIMModelCache` resources that download model artifacts to PVCs
3. The service waits for caches to become Available before starting
4. Cached models are mounted directly into the inference container

### Cache Preservation on Deletion

When you delete an `AIMService`:

- **Available caches are preserved** - they can be reused by future services
- **Non-available caches are cleaned up** - failed or incomplete downloads are removed

This means if you recreate the same service, it will immediately use the existing cached models without re-downloading.

See [Model Caching](../concepts/caching.md) for detailed information on cache lifecycle and management.

## Troubleshooting

### Service stuck in Pending

Check if the runtime config exists:
```bash
kubectl -n <namespace> get aimruntimeconfig
```

Check if templates are available:
```bash
kubectl -n <namespace> get aimservicetemplate
kubectl get aimclusterservicetemplate
```

### Routing not working

Verify the HTTPRoute was created:
```bash
kubectl -n <namespace> get httproute <service-name>-route
```

Check for path template errors in status:
```bash
kubectl -n <namespace> get aimservice <name> -o jsonpath='{.status.conditions[?(@.type=="RoutingReady")]}'
```

### Model not found

Verify the model exists:
```bash
kubectl -n <namespace> get aimmodel <model-name>
kubectl get aimclustermodel <model-name>
```

If using `spec.model.image` directly, verify the image URI is accessible and the runtime config is properly configured for model creation.

## Related Documentation

- [KV Cache](kv-cache.md) - Configure KV caching for improved performance
- [Runtime Configuration](runtime-config.md) - Configure runtime settings and credentials
- [Models](../concepts/models.md) - Understanding the model catalog
- [Templates](../concepts/templates.md) - Deep dive on templates and discovery
- [Model Caching](../concepts/caching.md) - Cache lifecycle and deletion behavior
