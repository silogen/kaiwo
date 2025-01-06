#!/bin/bash
set -e

# Install Ray Operator
helm repo add kuberay https://ray-project.github.io/kuberay-helm/
helm repo update
helm install kuberay-operator kuberay/kuberay-operator --version 1.2.2

# Install Kueue Operator
kubectl apply --server-side -f ./kueue-install.yaml