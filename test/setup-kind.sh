#!/bin/bash

set -e

RECREATE=false
POST_CLUSTER_SCRIPT=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --recreate)
            RECREATE=true
            shift
            ;;
        --post-cluster-setup)
            if [[ -n "$2" ]]; then
                POST_CLUSTER_SCRIPT="$2"
                shift 2
            else
                echo "Error: --post-cluster-setup requires a script path as an argument."
                exit 1
            fi
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

CLUSTER_NAME="kaiwo-test"

create_cluster() {
  echo "Creating Kind cluster '$CLUSTER_NAME'..."
  kind create cluster --name $CLUSTER_NAME

  if [[ -n "$POST_CLUSTER_SCRIPT" ]]; then
      if [[ -f "$POST_CLUSTER_SCRIPT" && -x "$POST_CLUSTER_SCRIPT" ]]; then
          echo "Executing post-cluster-setup script: $POST_CLUSTER_SCRIPT..."
          bash "$POST_CLUSTER_SCRIPT"
      else
          echo "Error: Post-cluster setup script '$POST_CLUSTER_SCRIPT' does not exist or is not executable."
          exit 1
      fi
  fi
}

# Check if the Kind cluster exists
if kind get clusters | grep -q "$CLUSTER_NAME"; then
    if [ "$RECREATE" = true ]; then
        echo "Deleting existing Kind cluster '$CLUSTER_NAME'..."
        kind delete cluster --name $CLUSTER_NAME
        create_cluster
    else
        echo "Kind cluster '$CLUSTER_NAME' already exists. Skipping creation."
    fi
else
    create_cluster
fi

# Add Cert manager
if ! helm list -q -n cert-manager | grep -q "cert-manager"; then
    echo "Adding Cert-Manager Helm repo..."
    helm repo add jetstack https://charts.jetstack.io
    helm repo update
fi

if ! helm list -q -n cert-manager | grep -q "cert-manager"; then
    echo "Deploying Cert-Manager..."
    helm install cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --set installCRDs=true \
        --version v1.17.1

    echo "Waiting for Cert-Manager to be deployed..."
    kubectl wait --for=condition=available --timeout=5m deployment/cert-manager -n cert-manager
    kubectl wait --for=condition=available --timeout=5m deployment/cert-manager-webhook -n cert-manager
    kubectl wait --for=condition=available --timeout=5m deployment/cert-manager-cainjector -n cert-manager
    echo "Cert-Manager deployed."
else
    echo "Cert-Manager is already installed. Skipping installation."
fi

# Add Kueue
echo "Deploying Kueue operator..."
kubectl apply --server-side -f https://github.com/kubernetes-sigs/kueue/releases/download/v0.10.1/manifests.yaml
echo "Waiting for Kueue to be deployed..."
kubectl wait deploy/kueue-controller-manager -n kueue-system --for=condition=available --timeout=5m
echo "Kueue deployed."

# Add KubeRay
# Check if the KubeRay Helm repo exists
if ! helm repo list | grep -q "kuberay"; then
    echo "Adding KubeRay Helm repo..."
    helm repo add kuberay https://ray-project.github.io/kuberay-helm/
    helm repo update
else
    echo "KubeRay Helm repo already exists. Skipping addition."
fi

# Check if KubeRay operator is installed
if ! helm list -q | grep -q "kuberay-operator"; then
    echo "Deploying KubeRay operator..."
    helm install kuberay-operator kuberay/kuberay-operator --version 1.1.0
    echo "Waiting for KubeRay to be deployed..."
    kubectl wait deploy/kuberay-operator --for=condition=available --timeout=5m
    echo "KubeRay deployed."
else
    echo "KubeRay operator is already installed. Skipping installation."
fi

if ! kubectl get namespaces | grep -q "$CLUSTER_NAME"; then
  echo "Creating test namespace"
  kubectl create namespace $CLUSTER_NAME
else
  echo "Test namespace '$CLUSTER_NAME' already exists"
fi

make install
kubectl apply -f config/webhook_local_dev/webhooks.yaml

echo "Cluster is ready!"
