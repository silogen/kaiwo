#!/bin/bash
set -euo pipefail

REPO_URL=${REPO_URL:-"ghcr.io/silogen/kaiwo"}
CLUSTER_NAME=${CLUSTER_NAME:-"kaiwo-test"}
VERSION_TAG=${1:?Usage: $0 <version_tag> [num_workers]}
NUM_WORKERS=${2:-5}


echo "Pulling Kind cluster snapshots for '$CLUSTER_NAME' with tag '$VERSION_TAG' from '$REPO_URL'..."

# Build list of expected container names
CONTROL_PLANE="${CLUSTER_NAME}-control-plane"
WORKERS=()
for i in $(seq 1 $NUM_WORKERS); do
  [[ $i -eq 1 ]] && WORKERS+=("${CLUSTER_NAME}-worker") || WORKERS+=("${CLUSTER_NAME}-worker${i}")
done

ALL_NODES=("$CONTROL_PLANE" "${WORKERS[@]}")

# Create temp directory for tracking results
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

# Pull all images in parallel
echo "Pulling ${#ALL_NODES[@]} snapshot images..."
for NODE in "${ALL_NODES[@]}"; do
  IMAGE="${REPO_URL}/kind-snapshot-${NODE}:${VERSION_TAG}"
  (
    if docker pull "$IMAGE" > "$TMPDIR/${NODE}.log" 2>&1; then
      echo "  ✓ $IMAGE"
      touch "$TMPDIR/${NODE}.success"
    else
      echo "  ✗ $IMAGE"
      touch "$TMPDIR/${NODE}.failed"
    fi
  ) &
done

# Wait for all pulls to complete
wait

# Check for failures
FAILED=()
for NODE in "${ALL_NODES[@]}"; do
  if [[ -f "$TMPDIR/${NODE}.failed" ]]; then
    FAILED+=("${REPO_URL}/kind-snapshot-${NODE}:${VERSION_TAG}")
  fi
done

if [[ ${#FAILED[@]} -gt 0 ]]; then
  echo ""
  echo "ERROR: Failed to pull the following images:"
  for IMG in "${FAILED[@]}"; do
    echo "  - $IMG"
  done
  exit 1
fi

echo ""
echo "Successfully pulled all snapshot images:"
for NODE in "${ALL_NODES[@]}"; do
  IMAGE="${REPO_URL}/kind-snapshot-${NODE}:${VERSION_TAG}"
  echo "  $IMAGE"
done