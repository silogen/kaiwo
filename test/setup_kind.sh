#!/bin/bash

set -e

TEST_NAME=${TEST_NAME:-"kaiwo-test"}

CERT_MANAGER_MANIFESTS="https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml"
KUEUE_MANIFESTS="https://github.com/kubernetes-sigs/kueue/releases/download/v0.10.1/manifests.yaml"
KUBERAY_MANIFESTS="github.com/ray-project/kuberay/ray-operator/config/default?ref=master"


create_cluster() {
  echo "Creating Kind cluster '$TEST_NAME'..."
  kind create cluster --name "$TEST_NAME" --config test/kind-test-cluster.yaml
}

# Check if the Kind cluster exists
if kind get clusters | grep -q "$TEST_NAME"; then
        echo "Deleting existing Kind cluster '$TEST_NAME'..."
        kind delete cluster --name "$TEST_NAME"
        create_cluster
else
    create_cluster
fi

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

# Add fake-gpu-operator
kubectl create ns gpu-operator
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged 
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 run.ai/simulated-gpu-node-pool=default
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 nvidia.com/gpu.product=Tesla-K80
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 nvidia.com/gpu.count=8
kubectl apply -f test/fake-gpu-operator/fake-gpu-operator.yaml


# Add Kueue
echo "Deploying Kueue operator..."
kubectl apply --server-side -f "$KUEUE_MANIFESTS"
echo "Waiting for Kueue to be deployed..."
kubectl rollout status deployment/kueue-controller-manager -n kueue-system --timeout=5m
echo "Kueue deployed."

# Add KubeRay
kubectl create -k "$KUBERAY_MANIFESTS"
kubectl rollout status deployment/kuberay-operator --timeout=5m
echo "KubeRay deployed."

echo "Cluster is ready!"

