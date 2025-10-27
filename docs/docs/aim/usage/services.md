# Deploying Inference Services

`AIMService` is the primary resource for deploying inference endpoints. It combines a model image, optional runtime configuration, and HTTP routing to produce a production-ready inference service.

## Quick Start

The minimal service requires only a model reference:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    ref: meta-llama-3-8b
```

This creates an inference service using the default runtime configuration and automatically selected template.

Alternatively, you can specify a container image directly, and AIM will auto-create a model resource if one doesn't exist:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    image: ghcr.io/example/llama-3.1-8b-instruct:v1.2.0
```

## Common Configuration

### Scaling

Control the number of replicas:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  replicas: 3
```

### Resource Limits

Override default resource allocations:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  resources:
    limits:
      cpu: "8"
      memory: 64Gi
    requests:
      cpu: "4"
      memory: 32Gi
```

### Runtime Overrides

Customize optimization settings without creating templates:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  overrides:
    metric: throughput      # or 'latency' for interactive workloads
    precision: fp16         # fp4, fp8, fp16, bf16, int4, int8, auto
    gpuSelector:
      count: 2
      model: MI300X
```

The controller automatically creates a template incorporating these overrides.

## Runtime Configuration

Reference a specific runtime configuration for credentials and defaults:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  runtimeConfigName: team-config  # defaults to 'default' if omitted
```

Runtime configurations provide:
- Image pull secrets for private registries
- Service account configuration
- Routing defaults
- Model creation scope (when using `spec.model.image`)

See [Runtime Configuration](runtime-config.md) for details.

## HTTP Routing

Enable external HTTP access through Gateway API:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
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
    ref: meta-llama-3-8b
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

## Templates

Templates define runtime profiles (optimization goals, GPU requirements, precision). The controller can select templates automatically or you can reference them explicitly.

### Automatic Selection

When `templateRef` is omitted, the controller finds available templates for your model and selects the best match:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  # templateRef omitted - automatic selection
```

Templates are filtered by:
- Model identifier (from `spec.model.ref` or matched via `spec.model.image`)
- Availability status (only healthy templates)
- GPU availability in the cluster
- Service overrides (if specified)

### Explicit Reference

Reference a specific template by name:

```yaml
spec:
  model:
    ref: meta-llama-3-8b
  templateRef: llama-3-8b-latency
```

The controller searches namespace-scoped templates first, then cluster-scoped templates.

## Authentication

For models requiring authentication (e.g., gated Hugging Face models):

```yaml
spec:
  model:
    ref: meta-llama-3-8b
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
    ref: meta-llama-3-8b
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

The `status` field shows:
- **Pending**: Initial state, resolving resources
- **Starting**: Creating infrastructure
- **Running**: Service is ready and serving traffic
- **Degraded**: Service is running but has warnings (e.g., routing issues)
- **Failed**: Service cannot start

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

- [Runtime Configuration](runtime-config.md) - Configure runtime settings and credentials
- [Models](../concepts/models.md) - Understanding the model catalog
- [Templates](../concepts/templates.md) - Deep dive on templates and discovery
