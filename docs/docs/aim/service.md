# AIMService Overview

`AIMService` is the user-facing CRD that turns discovery results into a running inference endpoint. It stitches together an image, service template, runtime configuration, and optional routing to produce a KServe `InferenceService` (and, if requested, an HTTPRoute).

This guide summarises the key fields, how runtime configs flow into services, and the route-template behaviour.

## Spec

```yaml
apiVersion: aim.silogen.ai/v1alpha1
kind: AIMService
metadata:
  name: llama-chat
  namespace: ml-team
spec:
  aimImageName: meta-llama-3-8b
  templateRef: llama-3-8b-latency
  runtimeConfigName: team-config
  replicas: 2
  resources:
    limits:
      cpu: "6"
      memory: 48Gi
    requests:
      cpu: "3"
      memory: 32Gi
  routing:
    enabled: true
    gatewayRef:
      name: inference-gateway
      namespace: gateways
    routeTemplate: "/{.metadata.namespace}/{.metadata.labels['team']}/{.spec.model}/"
```

### Core fields

| Field | Purpose |
| ----- | ------- |
| `aimImageName` | Canonical model ID (maps to an `AIMImage`/`AIMClusterImage`). |
| `templateRef` | Name of an `AIMServiceTemplate` or `AIMClusterServiceTemplate`. If omitted, the controller uses the image’s `defaultServiceTemplate` (and falls back to the service name if none is defined). |
| `runtimeConfigName` | Overrides registry credentials / routing defaults (`default` by default). |
| `replicas` | Optional replica override; template values still govern GPU profile. |
| `resources` | Optional `ResourceRequirements` override that sits on top of template/image defaults. |
| `routing` | Enables Gateway exposure and optional per-service route template. |

The service controller first resolves the template (preferring namespace templates, then cluster templates), resolves runtime config data, and creates/updates the downstream KServe resources. If the referenced template is absent, the controller creates a derived namespace template on the fly (`templateRef` defaults to the service name). When `templateRef` is omitted but the service provides overrides, the controller copies the image's default template into a namespace-scoped template owned by the service before applying the overrides.

When overrides trigger a derived template, the controller appends a short hash suffix (e.g., `-ovr-2926054f`) to the base template name so multiple override sets do not collide.

### Runtime config flow

- `runtimeConfigName` → resolve namespace `AIMRuntimeConfig`, then cluster `AIMClusterRuntimeConfig`.
- **Namespace config completely replaces cluster config** — there is no field-level merging. If a namespace config exists, it is used exclusively. If not, the cluster config is used as a fallback.
- The resolved config is recorded in the service status (`status.effectiveRuntimeConfig`) for audit purposes.
- If the service references a config that does not exist at either scope, it enters a `Degraded` state with reason `RuntimeConfigMissing` until the config is created.

### Resource resolution

`AIMService` surfaces a three-tier resource lookup:

1. `spec.resources` on the service wins when present.
2. Otherwise, the controller falls back to `spec.resources` on the resolved template.
3. Finally, it uses the catalog defaults defined on the underlying `AIMImage` / `AIMClusterImage`.

After the merge, if neither requests nor limits specify `amd.com/gpu`, the controller fills them from the discovery metadata stored on the template (`status.profile.metadata.gpu_count`) so the resulting KServe `InferenceService` always requests the expected number of devices, unless explicitly overridden.

### Routing templates

When `spec.routing.enabled` is true:

1. The controller chooses the HTTP path prefix in this order:
   - `spec.routing.routeTemplate` (service override).
   - `spec.routing.routeTemplate` from the runtime config.
   - Default: `/<namespace>/<service-uid>`.
2. Templates use JSONPath expressions wrapped in `{…}` and are rendered against the entire `AIMService` object. Label and annotation lookups (for keys such as `aim.silogen.ai/workload-id`) are resolved directly.
3. Each segment is lowercased and RFC-3986 encoded; trailing slashes are trimmed and the final path must be ≤ 200 characters.
4. Rendering failures (invalid JSONPath, missing label/annotation, oversized path) degrade the service with reason `RouteTemplateInvalid`. The controller skips HTTPRoute creation but still manages the ServingRuntime.

Runtime config templates provide namespace-wide defaults, while services can opt in to custom paths as shown above.

### Status

`status` mirrors reconciliation progress and routing outcomes:

| Field | Description |
| ----- | ----------- |
| `status` | Coarse lifecycle: `Pending`, `Starting`, `Running`, `Failed`, `Degraded`. |
| `conditions` | `Resolved`, `RuntimeReady`, `RoutingReady`, `CacheReady`, `Failure`, etc. |
| `effectiveRuntimeConfig` | Reference to the runtime config used (namespace or cluster) and a hash of the spec. |
| `resolvedTemplateRef` | Template name actually applied (namespace or cluster), including derived templates created for overrides. |
| `routing.path` | Resolved HTTP path when routing is enabled and the template rendered successfully. |

Example degraded status due to a missing runtime config:

```yaml
status:
  status: Degraded
  conditions:
  - type: Failure
    status: "True"
    reason: RuntimeConfigMissing
    message: AIMRuntimeConfig "team-config" not found in namespace "ml-team"
  - type: RuntimeReady
    status: "False"
    reason: RuntimeConfigMissing
    message: Cannot configure runtime without AIMRuntimeConfig
```

When routing templates fail to render:

```yaml
  - type: Failure
    status: "True"
    reason: RouteTemplateInvalid
    message: failed to evaluate route template ".metadata.labels['team']": label "team" not found
```

## Events and debugging

The controller emits events on the service object:

- `RuntimeConfigResolved` / `DefaultRuntimeConfigNotFound`
- `RouteTemplateInvalid` (warning) – includes the precise evaluation error.
- Standard KServe apply/update events.

To inspect the inferred HTTPRoute, use:

```bash
kubectl -n ml-team get httproute llama-chat-route -o yaml
```

The `spec.rules[0].matches[0].path.value` field shows the encoded path produced by the template logic.

## Summary

`AIMService` wraps the lower-level image/template primitives to deliver an inference endpoint. Runtime configs handle credentials and routing defaults, and the new route-template support lets platform teams enforce namespace-wide path structures while giving service owners room to customise their URL layout safely.
