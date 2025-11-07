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

# Step 1: Deploy kube-state-metrics
echo "1. Deploying kube-state-metrics..."
kubectl apply -f "${MONITORING_DIR}/kube-state-metrics.yaml"

# Step 2: Deploy Alloy ConfigMap (with variable substitution)
echo "2. Deploying Alloy ConfigMap..."
cat "${MONITORING_DIR}/alloy-vcluster-config.yaml" | \
  sed "s/\${NAMESPACE}/${NAMESPACE}/g" | \
  kubectl apply -f -

# Step 3: Deploy Alloy DaemonSet (with variable substitution)
echo "3. Deploying Alloy DaemonSet..."
cat "${MONITORING_DIR}/alloy-vcluster-daemonset.yaml" | \
  sed "s/\${NAMESPACE}/${NAMESPACE}/g" | \
  sed "s/\${GITHUB_RUN_ID}/${GITHUB_RUN_ID}/g" | \
  sed "s/\${GITHUB_RUN_ATTEMPT}/${GITHUB_RUN_ATTEMPT}/g" | \
  sed "s/\${INSTALLER}/${INSTALLER}/g" | \
  kubectl apply -f -

# Step 4: Deploy audit policy and webhook config (only if not already deployed)
echo "4. Deploying audit logging configuration..."

# Get the vCluster namespace in the host cluster (parent namespace)
# This requires switching context temporarily to deploy to the vCluster's host namespace
if [ "$VCLUSTER_NAMESPACE" != "$NAMESPACE" ]; then
  echo "   Note: Audit ConfigMaps should be deployed to host vCluster namespace: $VCLUSTER_NAMESPACE"
  echo "   Skipping automatic deployment. Please deploy manually to the host cluster:"
  echo "   kubectl apply -f ${MONITORING_DIR}/vcluster-audit-policy.yaml -n $VCLUSTER_NAMESPACE"
  echo "   kubectl apply -f ${MONITORING_DIR}/vcluster-audit-webhook.yaml -n $VCLUSTER_NAMESPACE"
else
  kubectl apply -f "${MONITORING_DIR}/vcluster-audit-policy.yaml"

  # Deploy webhook config with namespace substitution
  cat "${MONITORING_DIR}/vcluster-audit-webhook.yaml" | \
    sed "s/\${NAMESPACE}/${NAMESPACE}/g" | \
    kubectl apply -f -
fi

# Step 5: Wait for deployments to be ready
echo "5. Waiting for kube-state-metrics to be ready..."
kubectl rollout status deployment/kube-state-metrics -n kube-system --timeout=3m || \
  echo "Warning: kube-state-metrics deployment not ready after 3m"

echo "6. Waiting for Alloy DaemonSet to be ready..."
kubectl rollout status daemonset/alloy-vcluster -n "${NAMESPACE}" --timeout=3m || \
  echo "Warning: Alloy DaemonSet not ready after 3m"

echo "vCluster monitoring deployment complete!"
echo ""
echo "To verify:"
echo "  kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-state-metrics"
echo "  kubectl get pods -n ${NAMESPACE} -l app=alloy-vcluster"
echo "  kubectl logs -n ${NAMESPACE} -l app=alloy-vcluster -f"
