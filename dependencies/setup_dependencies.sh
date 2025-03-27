#!/bin/bash

set -e

kubectl apply -k dependencies/kustomization-client-side
echo "Waiting for Cert-Manager to be deployed..."
for deploy in cert-manager cert-manager-webhook cert-manager-cainjector; do
    echo "Waiting for deployment: $deploy"
    if kubectl get deployment/$deploy -n cert-manager >/dev/null 2>&1; then
        kubectl rollout status deployment/$deploy -n cert-manager --timeout=5m
    else
        echo "Warning: $deploy deployment not found, skipping wait."
    fi
done
echo "Cert-Manager deployed."

kubectl apply --server-side -k dependencies/kustomization-server-side
kubectl rollout status deployment/kueue-controller-manager -n kueue-system --timeout=5m
kubectl rollout status deployment/kuberay-operator --timeout=5m
kubectl rollout status deployment/appwrapper-controller-manager -n appwrapper-system --timeout=5m
echo "Other dependencies deployed"
