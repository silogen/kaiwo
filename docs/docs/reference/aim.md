# AMD Inference Microservices (AIM)

AIM provides a consistent way to deploy optimized LLM inference services on AMD GPUs using KServe and a vLLM-based runtime. This document describes the conceptual model, resource types, and the recommended workflow to publish, template, cache, and deploy AIM-based services. It targets both cluster administrators and ML engineers.

## Concepts

### AIMModel

An AIMModel is a cluster‑scoped catalog entry that maps a canonical model name to a single AIM container image. Canonical model names include a version and revision, for example `meta/llama-3-8b:1.2.1`. The operator uses this mapping to prepare the runtime with KServe for that image.

Example:
```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: ClusterModel
metadata:
  name: llama3-8b
spec:
  aim:
    name: meta/llama-3-8b:1.2.1
    image: ghcr.io/example/aim/llama3-8b:v1.1
```

### AIMServiceTemplate

An AIMServiceTemplate is a namespaced and versioned template that selects a runtime profile for a given AIMModel. A template references exactly one model by its canonical name. It defines the use case (latency or throughput), precision (for example bf16 or fp16), GPUs per replica, a single target GPU model, and tensor parallelism. It may also request cache warming through `warmCache`. On creation, the operator inspects the image associated with the referenced model and selects the appropriate runtime profile.

### ModelCache

ModelCache represents a cache for a single source such as a Hugging Face repository or an S3 bucket. Each cache is stored on a ReadWriteMany persistent volume and is indexed by its `sourceUri`, so templates that resolve to the same source share the same cache. Caches can be warmed through two paths: by setting `warmCache: true` on the template, or by creating an explicit ModelCache resource. When a template leaves `warmCache: false` (the default), a KaiwoService can still ensure caching is performed by setting `spec.aim.cacheModel: true` (default is `true`).

Example:

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: ModelCache
metadata:
  name: llama3-8b-weights
  namespace: my-llm
spec:
  sourceUri: hf://meta-llama/Llama-3-8b-Instruct
  size: 600Gi
  storageClassName: rwx-sc
```

### AIMNamespaceConfig

AIMNamespaceConfig is a namespaced configuration that carries credentials (via Secret references) and routing settings. It references the KGateway instance to use for exposure and routing in the namespace. The operator can create or link the KGateway instance according to this configuration.

Example:
```yaml
apiVersion: config.kaiwo.silogen.ai/v1alpha1
kind: AIMNamespaceConfig
metadata:
  name: default
  namespace: my-llm
spec:
  gateway:
    name: kgw-default
    namespace: gateway-system
  credentials:
    huggingFaceToken:
      secretName: hf-creds
      secretKey: token
    s3:
      - endpointUrl: https://s3.us-east-1.amazonaws.com
        accessKeyId:
          secretName: s3-creds
          secretKey: accessKeyId
        secretAccessKey:
          secretName: s3-creds
          secretKey: secretAccessKey
```

### KaiwoService (AIM)

Users deploy services by binding a model to a template through `KaiwoService`. The `spec.aim` section references the canonical model and a namespaced template, and may override the number of replicas. If the template does not warm caches, a service can still request caching with `spec.aim.cacheModel: true` (default). Deployments are implemented with KServe.

Example:

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoService
metadata:
  name: llama3-8b-svc
  namespace: my-llm
spec:
  aim:
    workloadId: 33e2b24e-96a7-465f-8478-105d55b220a2 # if not provided, this is set to the UUID of the KaiwoService
    model: meta/llama-3-8b:1.1+20240915
    templateRef: llama3-8b-mi300x-latency-v1
    cacheModel: true
    replicas: 2
```

## Lifecycle

1. Publish an AIMModel with the canonical model name and image.
2. Create an AIMServiceTemplate in the target namespace, referencing the AIMModel by its canonical name and selecting use case, precision, GPUs per replica, GPU model, and tensor parallelism. Optionally set `warmCache: true` to warm immediately after discovery.
3. If cache warming is enabled, caches are created and filled. Templates that resolve to the same source share the same ReadWriteMany cache.
4. Deploy a KaiwoService referencing the model and the template. Optionally set `replicas`.
5. If exposure and routing are enabled, the service is reachable through the namespace’s KGateway instance.

## Exposure and Routing

When exposure and routing are enabled, requests are routed through the KGateway instance configured for the namespace. The default path structure includes the namespace and a workload identifier: `/<namespace>/<workload_id>/`. For example:

```bash
curl -s -X POST \
  "http://<gateway-host>/<namespace>/<workload_id>/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model": "meta/llama-3-8b", "messages": [{"role": "user", "content": "Hello"}]}'
```

## GPU Scheduling

The controller places workloads on nodes that match the specified GPU model in the template. No user action is required.

## Prerequisites and Limits

ROCm 7 on worker nodes with the AMD GPU Operator installed, KServe available in the cluster, and a ReadWriteMany StorageClass for model caches. Configure namespace credentials via `AIMNamespaceConfig` when accessing private sources.

## Examples

### AIMServiceTemplate examples

Latency‑optimized on MI300X:

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama3-8b-mi300x-latency-v1
  namespace: my-llm
spec:
  model: meta/llama-3-8b:1.1+20240915
  useCase: latency
  precision: bf16
  gpusPerReplica: 1
  gpuModel: MI300X
  tensorParallelism: 1
  warmCache: true
```

Throughput‑optimized on MI325X:

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: qwen2-7b-mi325x-throughput-v1
  namespace: my-llm
spec:
  model: qwen-ai/qwen2-7b:2.0+20240915
  useCase: throughput
  precision: bf16
  gpusPerReplica: 2
  gpuModel: MI325X
  tensorParallelism: 2
  warmCache: false
```

### KaiwoService example

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoService
metadata:
  name: llama3-8b-svc
  namespace: my-llm
spec:
  aim:
    model: meta/llama-3-8b:1.1+20240915
    templateRef: llama3-8b-mi300x-latency-v1
    cacheModel: true
    replicas: 2
```
