# AIM Runtime Configuration

The AIM operator resolves runtime settings from two complementary Custom Resource Definitions (CRDs):

- **`AIMClusterRuntimeConfig`** provides cluster-wide defaults that apply to every namespace.
- **`AIMRuntimeConfig`** is namespaced and supplies tenant-specific overrides, primarily for authentication secrets.

Every AIM workload that needs runtime settings (images, templates, services, caches) references a configuration by name using the `runtimeConfigName` field. If the field is omitted, the operator automatically falls back to a config named `default`.

## Resolution model

1. The controller first looks for an `AIMRuntimeConfig` in the workload's namespace whose name matches `runtimeConfigName` (or `default`).
2. If no namespaced config exists, the controller falls back to an `AIMClusterRuntimeConfig` with the same name.
3. When both exist, fields are merged deterministically: namespace-scoped values override cluster defaults, while unset fields inherit from the cluster config.
4. The merged configuration is published in the consumer's `status.effectiveRuntimeConfig` field, including references to the contributing objects and a hash of the merged spec.

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

`AIMRuntimeConfig` inherits the cluster fields and adds authentication data for a specific namespace:

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMRuntimeConfig
metadata:
  name: default
  namespace: ml-team
spec:
  defaultStorageClassName: team-ssd         # overrides the cluster default
  serviceAccountName: aim-runtime
  imagePullSecrets:
  - name: ghcr-team-secret
```

| Field | Purpose |
| ----- | ------- |
| `defaultStorageClassName` | Namespace override for cache storage class. |
| `serviceAccountName` | ServiceAccount used by discovery jobs and other supporting pods. |
| `imagePullSecrets` | Additional registry credentials appended to the pod spec. Namespace entries override any duplicates from the cluster config. |
| `routing.routeTemplate` | Optional HTTP path template applied to services in the namespace. See [Routing templates](#routing-templates) for details. |

## Referencing runtime configs from workloads

The following AIM resources accept the `runtimeConfigName` field:

- `AIMImage` / `AIMClusterImage`
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

If `runtimeConfigName` is omitted, the controller resolves the `default` config and records the chosen objects in the template's status:

```yaml
status:
  effectiveRuntimeConfig:
    namespaceRef:
      kind: AIMRuntimeConfig
      namespace: ml-team
      name: default
    clusterRef:
      kind: AIMClusterRuntimeConfig
      name: default
    hash: 792403ad…
```

## Error and warning behaviour

- **Missing explicit config**: reconciliation fails and the workload reports `ConfigResolved=False` (or an equivalent failure condition) until the config appears.
- **Missing default config**: the controller emits `DefaultRuntimeConfigNotFound` and continues without overrides. Workloads that rely on private registries will fail later unless a namespace config supplies credentials.
- **Auth expectations**: discovery and other cluster-scoped operations run in the operator namespace and use the ServiceAccount/imagePullSecrets provided by the resolved runtime config. Namespaced configs should therefore include any required credentials.

## Operator namespace

The AIM controllers determine the operator namespace from the `AIM_OPERATOR_NAMESPACE` environment variable (default: `kaiwo-system`). Cluster-scoped workflows such as cluster templates run auxiliary pods in this namespace and resolve namespaced runtime configs there.

## Routing templates

Runtime configs can supply a reusable HTTP route template via `spec.routing.routeTemplate`. The template is rendered against the `AIMService` object using JSONPath expressions wrapped in `{…}`:

```yaml
spec:
  routing:
    routeTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/{.spec.model}/"
```

During reconciliation the controller:

- Evaluates each placeholder with JSONPath (missing or multi-value expressions fail the render).
- Lowercases and RFC 3986–encodes every path segment.
- Trims duplicate slashes and the trailing slash, enforcing a 200-character maximum.

A rendered path longer than 200 characters, an invalid JSONPath, or missing data degrades the `AIMService` with reason `RouteTemplateInvalid` and skips HTTPRoute creation.  

Services may override the runtime config by setting `spec.routing.routeTemplate`; if omitted, the runtime config (or the built-in namespace/UID default) is used. The service-level behaviour is described in [AIMService routing](./service.md#routing-templates).
