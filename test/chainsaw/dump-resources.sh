#!/bin/bash
# Dump Kubernetes resources status for debugging Chainsaw test failures

set -euo pipefail

NAMESPACE=${1:-""}

if [ -z "$NAMESPACE" ]; then
  echo "Usage: $0 <namespace>"
  exit 1
fi

echo "============================================="
echo "Resource Status Dump for namespace: $NAMESPACE"
echo "Time: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "============================================="
echo ""

# Function to dump resources with full YAML status and related events
dump_resource() {
  local resource=$1
  local ns_flag=$2
  local label=$3

  echo "=== $label ==="

  local names
  if [ "$ns_flag" = "namespaced" ]; then
    names=$(kubectl get "$resource" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.items[].metadata.name' 2>/dev/null || echo "")
  else
    names=$(kubectl get "$resource" -o json 2>/dev/null | jq -r '.items[].metadata.name' 2>/dev/null || echo "")
  fi

  if [ -z "$names" ]; then
    echo "  (no resources found)"
    echo ""
    return
  fi

  for name in $names; do
    echo "--- $name ---"

    # Print full resource YAML
    if [ "$ns_flag" = "namespaced" ]; then
      kubectl get "$resource" "$name" -n "$NAMESPACE" -o yaml 2>/dev/null || echo "  (error getting resource)"
    else
      kubectl get "$resource" "$name" -o yaml 2>/dev/null || echo "  (error getting resource)"
    fi

    echo ""
    echo "Events for $name:"

    # Get events related to this specific object
    # Extract kind from resource (e.g., "pods" -> "Pod", "aimservices.aim.silogen.ai" -> "AIMService")
    local kind
    kind=$(kubectl get "$resource" "$name" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.kind' 2>/dev/null || echo "")

    if [ -n "$kind" ]; then
      if [ "$ns_flag" = "namespaced" ]; then
        kubectl get events -n "$NAMESPACE" -o json 2>/dev/null | \
          jq -r --arg name "$name" --arg kind "$kind" \
          '.items[] | select(.involvedObject.name == $name and .involvedObject.kind == $kind) |
          "  \(.lastTimestamp // .eventTime) \(.type) \(.reason): \(.message)"' 2>/dev/null || echo "  (no events)"
      else
        kubectl get events --all-namespaces -o json 2>/dev/null | \
          jq -r --arg name "$name" --arg kind "$kind" \
          '.items[] | select(.involvedObject.name == $name and .involvedObject.kind == $kind) |
          "  \(.lastTimestamp // .eventTime) \(.type) \(.reason): \(.message)"' 2>/dev/null || echo "  (no events)"
      fi
    else
      echo "  (could not determine kind for event filtering)"
    fi

    echo ""
  done
  echo ""
}

# Namespace-scoped resources
dump_resource "pods" "namespaced" "Pods"
dump_resource "jobs.batch" "namespaced" "Jobs"
dump_resource "aimservices.aim.silogen.ai" "namespaced" "AIMServices"
dump_resource "aimmodels.aim.silogen.ai" "namespaced" "AIMModels"
dump_resource "aimservicetemplates.aim.silogen.ai" "namespaced" "AIMServiceTemplates"
dump_resource "aimtemplatecaches.aim.silogen.ai" "namespaced" "AIMTemplateCaches"
dump_resource "aimmodelcaches.aim.silogen.ai" "namespaced" "AIMModelCaches"
dump_resource "inferenceservices.serving.kserve.io" "namespaced" "InferenceServices"
dump_resource "httproutes.gateway.networking.k8s.io" "namespaced" "HTTPRoutes"
dump_resource "servingruntimes.serving.kserve.io" "namespaced" "ServingRuntimes"

# Cluster-scoped resources
dump_resource "aimclustermodels.aim.silogen.ai" "cluster" "AIMClusterModels"
dump_resource "aimclusterservicetemplates.aim.silogen.ai" "cluster" "AIMClusterServiceTemplates"

echo "============================================="
echo "All Events in namespace $NAMESPACE (sorted by time)"
echo "============================================="
kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' 2>/dev/null || echo "(no events found)"
echo ""

echo "Dump complete at $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
