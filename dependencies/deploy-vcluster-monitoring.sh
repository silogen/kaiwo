#!/bin/bash
set -euo pipefail

# Deploy vCluster monitoring components (Alloy, kube-state-metrics, audit config)
# This script should be called after vCluster is created and you're in the vCluster context

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MONITORING_DIR="${SCRIPT_DIR}/monitoring"

# Check if we're in a vCluster context
CURRENT_CONTEXT=$(kubectl config current-context)
NAMESPACE=$(kubectl config view --minify -o jsonpath='{..namespace}' 2>/dev/null || echo "default")

echo "Current context: $CURRENT_CONTEXT"
echo "Current namespace: $NAMESPACE"

# Get environment variables for labels (with defaults)
VCLUSTER_NAMESPACE=${VCLUSTER_NAMESPACE:-${NAMESPACE}}
GITHUB_RUN_ID=${GITHUB_RUN_ID:-"local"}
GITHUB_RUN_ATTEMPT=${GITHUB_RUN_ATTEMPT:-"1"}
INSTALLER=${INSTALLER:-"manual"}

echo "Deploying vCluster monitoring components..."
echo "  VCLUSTER_NAMESPACE: $VCLUSTER_NAMESPACE"
echo "  GITHUB_RUN_ID: $GITHUB_RUN_ID"
echo "  GITHUB_RUN_ATTEMPT: $GITHUB_RUN_ATTEMPT"
echo "  INSTALLER: $INSTALLER"

# Step 1: Deploy monitoring components using kustomize
echo "1. Deploying monitoring components (kube-state-metrics, event-exporter)..."
kubectl apply -k "${MONITORING_DIR}/client"

# Step 2: Wait for deployments to be ready
echo "2. Waiting for kube-state-metrics to be ready..."
kubectl rollout status deployment/kube-state-metrics -n kube-system --timeout=3m || \
  echo "Warning: kube-state-metrics deployment not ready after 3m"

echo "3. Waiting for event-exporter to be ready..."
kubectl rollout status deployment/event-exporter -n kube-system --timeout=3m || \
  echo "Warning: event-exporter deployment not ready after 3m"

echo "vCluster monitoring deployment complete!"
echo ""
echo "To verify:"
echo "  kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-state-metrics"
echo "  kubectl get pods -n kube-system -l app=event-exporter"
