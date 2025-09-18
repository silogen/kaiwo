#!/bin/bash

set -e

USE_LOCAL=false

# Parse arguments
for arg in "$@"; do
  case $arg in
    --local)
      USE_LOCAL=true
      ;;
    *)
      echo "Unknown option: $arg"
      exit 1
      ;;
  esac
done

CERT_MANAGER_PATH="dependencies/kustomization-client-side"
SERVER_SIDE_DEPS_PATH="dependencies/kustomization-server-side"
CLIENT_SIDE_DEPS_PATH="dependencies/kustomization-client-side"
if [ "$USE_LOCAL" != "true" ]; then
  CERT_MANAGER_PATH="github.com/silogen/kaiwo//$CERT_MANAGER_PATH?ref=main"
  SERVER_SIDE_DEPS_PATH="github.com/silogen/kaiwo//$SERVER_SIDE_DEPS_PATH?ref=main"
  CLIENT_SIDE_DEPS_PATH="github.com/silogen/kaiwo//$CLIENT_SIDE_DEPS_PATH?ref=main"
fi

echo "Deploying Cert-Manager"
kubectl apply -k "$CERT_MANAGER_PATH"
echo "Waiting for Cert-Manager to be deployed..."
for deploy in cert-manager cert-manager-webhook cert-manager-cainjector; do
  echo "Waiting for deployment: $deploy"
  if kubectl get deployment/$deploy -n cert-manager >/dev/null 2>&1; then
    kubectl rollout status deployment/$deploy -n cert-manager --timeout=5m
  else
    echo "Warning: $deploy deployment not found, skipping wait."
  fi
done
echo "Cert-Manager deployed"

#- https://github.com/kubernetes-sigs/node-feature-discovery/deployment/overlays/default?ref=v0.17.3

echo "Deploying other dependencies"
kubectl apply --server-side --force-conflicts -k "$SERVER_SIDE_DEPS_PATH"
echo "Waiting for other dependencies to be deployed..."
kubectl rollout status deployment/kuberay-operator --timeout=5m
kubectl rollout status deployment/appwrapper-controller-manager -n appwrapper-system --timeout=5m
echo "Other dependencies deployed"

kubectl wait \
	--for condition=Established \
	--all CustomResourceDefinition \
	--namespace=monitoring

echo "All dependencies are deployed"
