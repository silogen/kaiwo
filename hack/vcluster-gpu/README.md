# vcluster GPU Preemption Dev Setup

The GPU preemption operator needs a single Prometheus-compatible endpoint serving
`gpu_gfx_activity` metrics with `pod` and `namespace` labels. On bare-metal
clusters this is straightforward, but in a vcluster it requires extra wiring
because the AMD GPU exporter DaemonSet runs on the **host** cluster and reports
host-mangled pod/namespace names that the vcluster operator can't resolve.

## Architecture

```
Host cluster                              vcluster
┌──────────────────────────┐     DNS fallback     ┌──────────────────┐
│  DaemonSet               │     ─────────────►   │  kaiwo operator  │
│  default-metrics-exporter│                      │  (scrapes :5000) │
│  (per-node, kube-amd-gpu)│                      └──────────────────┘
│          │               │                               │
│          ▼               │                               │
│  OTel Collector          │      ◄── HTTP GET ────────────┘
│  gpu-metrics-{ns}        │
│  - scrapes all nodes     │
│  - filters to this ns    │
│  - rewrites pod/ns labels│
│    to vcluster-local     │
│  - re-exports on :5000   │
└──────────────────────────┘
```

**Why it works:** The vcluster is created with
`networking.advanced.fallbackHostCluster: true` (see `vcluster-values.yaml`),
which makes its CoreDNS fall back to host cluster DNS for unknown services. This
lets the operator inside the vcluster reach
`gpu-metrics-{ns}-collector.kube-amd-gpu.svc:5000` without any extra networking.

## Prerequisites

**OTel operator on host** (one-time, shared across all vclusters):

```bash
HOST_CTX=<your-host-context>

kustomize build dependencies/kustomization-server-side/overlays/gpu/amd/metrics-collector \
  | kubectl --context "$HOST_CTX" apply --server-side --force-conflicts -f -

kubectl --context "$HOST_CTX" rollout status deployment/opentelemetry-operator-controller-manager \
  -n opentelemetry-operator-system --timeout=120s
```

If the `OpenTelemetryCollector` CR fails with a webhook error from a stale Helm
install, delete the old webhooks first:

```bash
kubectl --context "$HOST_CTX" delete mutatingwebhookconfiguration \
  otel-operator-opentelemetry-operator-mutation
kubectl --context "$HOST_CTX" delete validatingwebhookconfiguration \
  otel-operator-opentelemetry-operator-validation
```

## Quick start

```bash
./hack/vcluster-gpu/setup.sh <host-context>            # default name: kaiwo-<user>-dev
./hack/vcluster-gpu/setup.sh <host-context> my-vcluster # custom name
```

The script creates the vcluster, installs dependencies/CRDs, deploys the
per-vcluster OTel collector on the host, and verifies metrics reachability.
It prints the `kubectl set env` command to wire the operator at the end.

To tear down the collector:

```bash
./hack/vcluster-gpu/setup.sh --teardown <host-context> [vcluster-name]
```

## Manual setup

1. **Create the vcluster** with DNS fallback:

```bash
vcluster create kaiwo-$(whoami)-dev --namespace kaiwo-$(whoami)-dev \
  -f hack/vcluster-gpu/vcluster-values.yaml
```

2. **Deploy the per-vcluster OTel collector** on the host:

```bash
HOST_CTX=<your-host-context>
HOST_NS=kaiwo-$(whoami)-dev

VCLUSTER_HOST_NS=$HOST_NS kustomize build hack/vcluster-gpu \
  | envsubst '$VCLUSTER_HOST_NS' \
  | kubectl --context "$HOST_CTX" apply -f -
```

3. **Set the metrics endpoint** on the operator:

```bash
kubectl set env deployment/kaiwo-controller-manager -n kaiwo-system \
  GPU_PREEMPTION_ENABLED=true \
  GPU_PREEMPTION_METRICS_ENDPOINT="http://gpu-metrics-${HOST_NS}-collector.kube-amd-gpu.svc:5000/metrics"
```

## Verify

```bash
HOST_NS=kaiwo-$(whoami)-dev

kubectl run curl-test --rm -it --restart=Never --image=curlimages/curl:latest \
  -- curl -s "http://gpu-metrics-${HOST_NS}-collector.kube-amd-gpu.svc:5000/metrics" \
  | grep gpu_gfx_activity | head -5
```

`pod` and `namespace` labels should show vcluster-local names, not host-mangled
names like `my-pod-x-my-ns-x-vcluster`.

## Files

| File | Purpose |
|------|---------|
| `kustomization.yaml` | Bundles RBAC + collector for `kustomize build` |
| `vcluster-values.yaml` | vcluster Helm values: node sync + DNS host fallback |
| `otel-collector.yaml` | Per-vcluster OTel collector template (`${VCLUSTER_HOST_NS}` placeholder) |
| `otel-collector-rbac.yaml` | Host-cluster RBAC for the `k8sattributes` processor |
| `setup.sh` | Automated setup/teardown script |
