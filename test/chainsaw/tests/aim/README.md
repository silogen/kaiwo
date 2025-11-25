# AIM Template Controller Tests

This directory contains Chainsaw tests for the AIM engine functionality. You must run these against a Kubernetes cluster with at least one node with MI300X GPUs.

## Pre-requisites

You must first apply the cluster default resources:

```bash
kubectl apply -f service/_shared/default-cluster-image.yaml
kubectl apply -f service/_shared/default-cluster-template.yaml
```

## Running Tests

Run all AIM tests:
```bash
chainsaw test --test-dir test/chainsaw/tests/aim/ \
  --values=test/chainsaw/values/kvcache.yaml
```

Run a specific test:
```bash
chainsaw test --test-dir test/chainsaw/tests/aim/kvcache/basic \
  --values=test/chainsaw/values/kvcache.yaml
```

**Note**: The `--values` flag is required for kvcache tests. YAMLs use `($values.storageClass)` templating.
