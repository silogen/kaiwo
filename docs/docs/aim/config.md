# AIM Runtime Configuration

The AIM operator resolves runtime settings from two Custom Resource Definitions (CRDs):

- **`AIMClusterRuntimeConfig`** provides cluster-wide defaults that apply to every namespace.
- **`AIMRuntimeConfig`** is namespaced and supplies tenant-specific configuration, including authentication secrets.

Every AIM workload that needs runtime settings (images, templates, services, caches) references a configuration by name using the `runtimeConfigName` field. If the field is omitted, the operator automatically falls back to a config named `default`.

## Resolution model

1. The controller first looks for an `AIMRuntimeConfig` in the workload's namespace whose name matches `runtimeConfigName` (or `default`).
2. If a namespace-scoped config is found, it is used **exclusively** — no field-level merging occurs.
3. If no namespaced config exists, the controller falls back to an `AIMClusterRuntimeConfig` with the same name.
4. The resolved configuration is published in the consumer's `status.effectiveRuntimeConfig` field, including a reference to the source object and a hash of the spec.

**Important**: When both namespace and cluster configs exist with the same name, the namespace config **completely replaces** the cluster config. There is no field-level merging or inheritance. This ensures predictable, easy-to-understand behavior where what you write is what you get.

If a workload explicitly references a runtime config that does not exist at either scope, reconciliation fails with a clear error. When the implicit `default` config is missing, the controller emits a warning event and continues, allowing workloads that do not require extra configuration to proceed.

## Cluster runtime configuration

`AIMClusterRuntimeConfig` captures the non-secret defaults shared across namespaces:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMClusterRuntimeConfig
metadata:
  name: default
spec:
  defaultStorageClassName: fast-nvme
```

| Field | Purpose |
| ----- | ------- |
| `defaultStorageClassName` | Storage class used for on-demand model caches when a workload does not specify one. |

Cluster configs never carry secrets. They model cluster-wide policy knobs such as storage defaults.

## Namespace runtime configuration

`AIMRuntimeConfig` provides namespace-specific configuration including authentication data:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  defaultStorageClassName: team-ssd
  serviceAccountName: aim-runtime
  imagePullSecrets:
  - name: ghcr-team-secret
  routing:
    enabled: true
    gatewayRef:
      name: kserve-gateway
      namespace: kgateway-system
```

| Field | Purpose |
| ----- | ------- |
| `defaultStorageClassName` | Storage class used for model caches. |
| `serviceAccountName` | ServiceAccount used by discovery jobs and other supporting pods. |
| `imagePullSecrets` | Registry credentials added to pod specs. |
| `routing.enabled` | Enable or disable HTTP routing for services using this config. |
| `routing.gatewayRef` | Gateway parent for HTTPRoute resources. |
| `routing.pathTemplate` | Optional HTTP path template applied to services in the namespace. See [Routing templates](#routing-templates) for details. |

**Note**: If you want cluster-wide defaults for fields like `defaultStorageClassName` or `routing`, you must explicitly specify them in each namespace config. There is no automatic inheritance from cluster configs.

## Referencing runtime configs from workloads

The following AIM resources accept the `runtimeConfigName` field:

- `AIMModel` / `AIMClusterModel`
- `AIMServiceTemplate` / `AIMClusterServiceTemplate`
- `AIMService`
- `AIMTemplateCache`

Example template referencing a non-default runtime config:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMServiceTemplate
metadata:
  name: llama-latency
  namespace: ml-team
spec:
  aimImageName: llama-3-8b
  runtimeConfigName: team-config
  metric: latency
  precision: fp16
```

If `runtimeConfigName` is omitted, the controller resolves the `default` config and records the chosen object in the template's status.

When using a namespace config:
```yaml
status:
  effectiveRuntimeConfig:
    namespaceRef:
      kind: AIMRuntimeConfig
      namespace: ml-team
      name: default
      scope: Namespace
    hash: 792403ad…
```

When falling back to a cluster config:
```yaml
status:
  effectiveRuntimeConfig:
    clusterRef:
      kind: AIMClusterRuntimeConfig
      name: default
      scope: Cluster
    hash: 5f8a9b2c…
```

Only one ref (namespace or cluster) will be present, never both.

## Error and warning behaviour

- **Missing explicit config**: reconciliation fails and the workload reports `ConfigResolved=False` (or an equivalent failure condition) until the config appears.
- **Missing default config**: the controller emits `DefaultRuntimeConfigNotFound` and continues without overrides. Workloads that rely on private registries will fail later unless a namespace config supplies credentials.
- **Auth expectations**: discovery and other cluster-scoped operations run in the operator namespace and use the ServiceAccount/imagePullSecrets provided by the resolved runtime config. Namespaced configs should therefore include any required credentials.

## Operator namespace

The AIM controllers determine the operator namespace from the `AIM_OPERATOR_NAMESPACE` environment variable (default: `kaiwo-system`). Cluster-scoped workflows such as cluster templates run auxiliary pods in this namespace and resolve namespaced runtime configs there.

## Routing templates

Runtime configs can supply a reusable HTTP route template via `spec.routing.pathTemplate`. The template is rendered against the `AIMService` object using JSONPath expressions wrapped in `{…}`:

```yaml
spec:
  routing:
    pathTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/{.spec.model}/"
```

During reconciliation the controller:

- Evaluates each placeholder with JSONPath (missing or multi-value expressions fail the render).
- Lowercases and RFC 3986–encodes every path segment.
- Trims duplicate slashes and the trailing slash, enforcing a 200-character maximum.

A rendered path longer than 200 characters, an invalid JSONPath, or missing data degrades the `AIMService` with reason `PathTemplateInvalid` and skips HTTPRoute creation.  

Services may override the runtime config by setting `spec.routing.pathTemplate`; if omitted, the runtime config (or the built-in namespace/UID default) is used. The service-level behaviour is described in [AIMService routing](./service.md#routing-templates).
