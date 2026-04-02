# kaiwo

This branch contains the auto-generated Helm chart and CRDs for kaiwo-operator.

## Installation

CRDs must be installed before the operator:

```bash
# 1. Install CRDs
kubectl apply -f https://raw.githubusercontent.com/silogen/kaiwo/artifacts/crd/crds.yaml
kubectl wait --for=condition=Established crd --all --timeout=60s

# 2a. Install operator via Helm OCI (recommended)
helm install kaiwo oci://ghcr.io/silogen/charts/kaiwo-operator \
  --version 0.0.0-test.1 --namespace kaiwo-system --create-namespace

# 2b. Or install from this branch
helm install kaiwo ./chart --namespace kaiwo-system --create-namespace
```

Or clone this branch:

```bash
git clone -b artifacts https://github.com/silogen/kaiwo.git kaiwo
cd kaiwo

kubectl apply -f crd/crds.yaml
kubectl wait --for=condition=Established crd --all --timeout=60s
helm install kaiwo ./chart --namespace kaiwo-system --create-namespace
```

## Source

Generated from tag: v0.0.0-test.1 (commit: 00b624fb3ac8803e910866076be9c07eb57b4ed9)
