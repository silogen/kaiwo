# AIM - AMD Inference Microservice

AIM (AMD Inference Microservice) Engine is a Kubernetes operator that simplifies the deployment and management of AI inference workloads on AMD GPUs. It provides a declarative, cloud-native approach to running ML models at scale.

## What AIM Does

AIM abstracts the complexity of inference deployment by providing:

- **Simple Service Deployment**: Deploy inference endpoints with minimal configuration using `AIMService` resources
- **Automatic Optimization**: Configure workloads for latency or throughput optimization with preset profiles
- **Model Catalog Management**: Maintain a catalog of available models across cluster and namespace scopes
- **HTTP Routing Integration**: Expose services through Gateway API with customizable path templates
- **Resource Management**: Handle GPU allocation, resource requirements, and scaling automatically

## Quick Example

Deploy an inference service:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  model:
    image: ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0
  replicas: 2
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    pathTemplate: "{.metadata.namespace}/{.metadata.name}"
```

AIM Engine automatically:

- Resolves the model container image
- Selects an appropriate runtime configuration
- Deploys a KServe InferenceService
- Creates HTTP routing through Gateway API via the path `ml-team/llama-chat`

## Documentation

- **[Usage Guides](usage/)**: Practical guides for deploying and configuring inference services
    - [Services](usage/services.md) - Deploy and manage inference endpoints
    - [Runtime Configuration](usage/runtime-config.md) - Configure credentials and settings

- **[Concepts](concepts/)**: Deep dive into AIM Engine architecture and internals
    - [Models](concepts/models.md) - Model catalog and discovery mechanism
    - [Templates](concepts/templates.md) - Runtime profiles and discovery
    - [Runtime Config](concepts/runtime-config.md) - Resolution algorithm and architecture

## Getting Started

1. **Deploy a service**: Start with the [Services usage guide](usage/services.md) to deploy your first inference endpoint

2. **Configure authentication**: Set up credentials for private registries using [Runtime Configuration](usage/runtime-config.md)

3. **Explore advanced features**: Learn about automatic template selection, model caching, and custom routing in the [Concepts documentation](concepts/)

## Architecture

AIM builds on Kubernetes and KServe to provide:

- **Declarative API**: Define inference services using Kubernetes custom resources
- **Multi-tenancy**: Namespace-scoped and cluster-scoped resources support team isolation
- **GitOps-friendly**: All configuration expressed as YAML for version control and automation
- **Gateway API Integration**: Modern, standards-based HTTP routing

## Support

For issues, questions, or contributions, please refer to the main project repository.
