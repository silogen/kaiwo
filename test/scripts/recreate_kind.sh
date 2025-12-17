#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_URL=${REPO_URL:-"ghcr.io/silogen/kaiwo"}
CLUSTER_NAME=${CLUSTER_NAME:-"kaiwo-test"}
VERSION_TAG=${1:?Usage: $0 <version_tag> [num_workers]}
NUM_WORKERS=${2:-5}
KIND_CONFIG="${SCRIPT_DIR}/../kind/kind-test-cluster.yaml"

echo "=============================================="
echo "Recreating Kind cluster '$CLUSTER_NAME' from snapshot '$VERSION_TAG'"
echo "=============================================="

# Step 1: Pull snapshot images
echo ""
echo "Step 1: Pulling snapshot images..."
echo "----------------------------------------------"
REPO_URL="$REPO_URL" CLUSTER_NAME="$CLUSTER_NAME" "$SCRIPT_DIR/pull_kind_snapshot.sh" "$VERSION_TAG" "$NUM_WORKERS"

# Step 2: Delete existing cluster if present
echo ""
echo "Step 2: Deleting existing cluster (if any)..."
echo "----------------------------------------------"
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "Deleting existing cluster '$CLUSTER_NAME'..."
  kind delete cluster --name "$CLUSTER_NAME"
else
  echo "No existing cluster '$CLUSTER_NAME' found."
fi

# Step 3: Create fresh Kind cluster
echo ""
echo "Step 3: Creating fresh Kind cluster..."
echo "----------------------------------------------"
if [[ ! -f "$KIND_CONFIG" ]]; then
  echo "ERROR: Kind config not found at: $KIND_CONFIG"
  exit 1
fi
kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG"

# Step 4: Restore from snapshot
echo ""
echo "Step 4: Restoring cluster state from snapshot..."
echo "----------------------------------------------"
REPO_URL="$REPO_URL" CLUSTER_NAME="$CLUSTER_NAME" "$SCRIPT_DIR/restore_kind.sh" "$VERSION_TAG" "$NUM_WORKERS"

echo ""
echo "=============================================="
echo "Kind cluster '$CLUSTER_NAME' recreated successfully!"
echo "=============================================="


