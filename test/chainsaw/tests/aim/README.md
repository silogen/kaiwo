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
chainsaw test test/chainsaw/tests/standard/aim/
```

Run a specific test:
```bash
chainsaw test test/chainsaw/tests/standard/aim/01-cluster-template-no-image.yaml
```
