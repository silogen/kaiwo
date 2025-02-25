#!/bin/bash

set -e

RECREATE="true"
TEST_NAME=${TEST_NAME:-"kaiwo-test"}

CERT_MANAGER_MANIFESTS="https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml"
KUEUE_MANIFESTS="https://github.com/kubernetes-sigs/kueue/releases/download/v0.10.1/manifests.yaml"
KUBERAY_MANIFESTS="github.com/ray-project/kuberay/ray-operator/config/default?ref=master"


create_cluster() {
  echo "Creating Kind cluster '$TEST_NAME'..."
  kind create cluster --name "$TEST_NAME"
}

# Check if the Kind cluster exists
if kind get clusters | grep -q "$TEST_NAME"; then
    if [ "$RECREATE" = "true" ]; then
        echo "Deleting existing Kind cluster '$TEST_NAME'..."
        kind delete cluster --name "$TEST_NAME"
        create_cluster
    else
        echo "Kind cluster '$TEST_NAME' already exists. Skipping creation."
    fi
else
    create_cluster
fi

kubectl delete -f "$CERT_MANAGER_MANIFESTS" --ignore-not-found
kubectl apply -f "$CERT_MANAGER_MANIFESTS"
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

# Add Kueue
echo "Deploying Kueue operator..."
kubectl delete -f "$KUEUE_MANIFESTS" --ignore-not-found 
kubectl apply --server-side -f "$KUEUE_MANIFESTS"
echo "Waiting for Kueue to be deployed..."
kubectl rollout status deployment/kueue-controller-manager -n kueue-system --timeout=1m
echo "Kueue deployed."

# Add KubeRay
kubectl delete -k "$KUBERAY_MANIFESTS" --ignore-not-found
kubectl create -k "$KUBERAY_MANIFESTS"
kubectl rollout status deployment/kuberay-operator --timeout=1m
echo "KubeRay deployed."

# Ensure test namespace exists
if ! kubectl get namespace "$TEST_NAME" >/dev/null 2>&1; then
  echo "Creating test namespace '$TEST_NAME'"
  kubectl create namespace "$TEST_NAME"
else
  echo "Test namespace '$TEST_NAME' already exists."
fi

echo "Cluster is ready!"

