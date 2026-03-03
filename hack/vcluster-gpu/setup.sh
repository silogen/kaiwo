#!/usr/bin/env bash
#
# Set up a vcluster with GPU metrics access for GPU preemption development.
#
# This script handles:
#   - vcluster creation with DNS host fallback
#   - Per-vcluster OTel collector deployment on the host cluster
#   - Metrics endpoint reachability verification
#
# It does NOT install kaiwo dependencies, CRDs, or the operator itself.
# Do that the regular way (deploy.sh, make install, Helm, Tilt, etc.)
# after this script completes.
#
# Prerequisites:
#   - vcluster CLI, kustomize, envsubst (gettext) installed
#   - OTel operator installed on the host cluster (see README.md)
#
# Usage:
#   ./setup.sh <host-context> [vcluster-name]
#   ./setup.sh --teardown <host-context> [vcluster-name]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    echo "Usage: $0 [--teardown] <host-context> [vcluster-name]"
    exit 1
}

TEARDOWN=false
if [[ "${1:-}" == "--teardown" ]]; then
    TEARDOWN=true
    shift
fi

HOST_CTX="${1:?host-context required$(usage)}"
VCLUSTER_NAME="${2:-kaiwo-$(whoami)-dev}"
HOST_NS="$VCLUSTER_NAME"
METRICS_ENDPOINT="http://gpu-metrics-${HOST_NS}-collector.kube-amd-gpu.svc:5000/metrics"

teardown() {
    echo "=== Tearing down GPU metrics collector for $VCLUSTER_NAME ==="
    kubectl --context "$HOST_CTX" delete opentelemetrycollector "gpu-metrics-$HOST_NS" \
        -n kube-amd-gpu --ignore-not-found
    echo "Collector deleted. Shared RBAC left in place."
    echo "To also delete the vcluster: vcluster delete $VCLUSTER_NAME --namespace $HOST_NS"
}

if $TEARDOWN; then
    teardown
    exit 0
fi

echo "=== Setting up vcluster with GPU metrics access ==="
echo "  Host context:     $HOST_CTX"
echo "  vcluster name:    $VCLUSTER_NAME"
echo "  Host namespace:   $HOST_NS"
echo "  Metrics endpoint: $METRICS_ENDPOINT"
echo ""

# --- Step 1: Create vcluster with DNS fallback ---
if vcluster list --output json 2>/dev/null | grep -q "\"Name\":\"$VCLUSTER_NAME\""; then
    echo "[1/3] vcluster '$VCLUSTER_NAME' already exists, skipping creation."
else
    echo "[1/3] Creating vcluster '$VCLUSTER_NAME'..."
    vcluster create "$VCLUSTER_NAME" --namespace "$HOST_NS" \
        -f "$SCRIPT_DIR/vcluster-values.yaml"
fi

# --- Step 2: Deploy per-vcluster OTel collector on host ---
echo "[2/3] Deploying GPU metrics collector on host cluster..."
export VCLUSTER_HOST_NS="$HOST_NS"
kustomize build "$SCRIPT_DIR" | envsubst '$VCLUSTER_HOST_NS' \
    | kubectl --context "$HOST_CTX" apply -f -

echo "    Waiting for collector to be ready..."
kubectl --context "$HOST_CTX" rollout status deployment/"gpu-metrics-${HOST_NS}-collector" \
    -n kube-amd-gpu --timeout=60s 2>/dev/null || \
    echo "    (collector deployment not found yet — the OTel operator may need a moment)"

# --- Step 3: Verify metrics reachability ---
echo "[3/3] Verifying metrics endpoint from vcluster..."
if kubectl run gpu-metrics-check --rm -it --restart=Never --image=curlimages/curl:latest \
    -- curl -sf --max-time 5 "$METRICS_ENDPOINT" >/dev/null 2>&1; then
    echo "    Metrics endpoint reachable."
else
    echo "    WARNING: Could not reach $METRICS_ENDPOINT"
    echo "    The collector may still be starting. Verify manually:"
    echo "      kubectl run curl-test --rm -it --restart=Never --image=curlimages/curl:latest \\"
    echo "        -- curl -s '$METRICS_ENDPOINT' | grep gpu_gfx_activity | head -5"
fi

echo ""
echo "=== Done ==="
echo ""
echo "Next steps:"
echo "  1. Install kaiwo dependencies, CRDs, and operator the regular way"
echo "  2. Set the metrics endpoint on the operator:"
echo "     kubectl set env deployment/kaiwo-controller-manager -n kaiwo-system \\"
echo "       GPU_PREEMPTION_ENABLED=true \\"
echo "       GPU_PREEMPTION_METRICS_ENDPOINT='$METRICS_ENDPOINT'"
