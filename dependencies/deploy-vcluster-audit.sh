#!/bin/bash
set -euo pipefail

# Deploy audit logging components to vCluster namespace on HOST cluster
# This must be run BEFORE creating the vCluster, with HOST kubeconfig

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MONITORING_DIR="${SCRIPT_DIR}/monitoring"

# Get vCluster namespace (on host cluster)
VCLUSTER_NAMESPACE=${VCLUSTER_NAMESPACE:-""}
GITHUB_RUN_ID=${GITHUB_RUN_ID:-"local"}
GITHUB_RUN_ATTEMPT=${GITHUB_RUN_ATTEMPT:-"1"}
INSTALLER=${INSTALLER:-"manual"}

if [ -z "$VCLUSTER_NAMESPACE" ]; then
  echo "Error: VCLUSTER_NAMESPACE environment variable must be set"
  echo "Example: export VCLUSTER_NAMESPACE=kaiwo-test-123-1-helm"
  exit 1
fi

# Check if namespace exists
if ! kubectl get namespace "$VCLUSTER_NAMESPACE" >/dev/null 2>&1; then
  echo "Error: Namespace $VCLUSTER_NAMESPACE does not exist"
  echo "Create it first: kubectl create namespace $VCLUSTER_NAMESPACE"
  exit 1
fi

echo "Deploying audit logging components to namespace: $VCLUSTER_NAMESPACE"

# Step 1: Deploy audit policy ConfigMap
echo "1. Deploying audit policy ConfigMap..."
kubectl apply -f "${MONITORING_DIR}/vcluster-audit-policy.yaml" -n "$VCLUSTER_NAMESPACE"

echo ""
echo "Audit policy ConfigMap deployed successfully!"
echo ""
echo "Note: Audit logs will be written to stdout and collected by host Alloy"
echo ""
echo "Next step: Create vCluster with:"
echo "  vcluster create <name> --namespace $VCLUSTER_NAMESPACE --values dependencies/vcluster.yaml"
echo ""
echo "To view audit logs after vCluster creation:"
echo "  kubectl logs -n $VCLUSTER_NAMESPACE -l app=vcluster -c vcluster --tail=100 | grep '{\"kind\":\"Event\"'"
