#!/bin/bash
set -euo pipefail

REPO_URL=${REPO_URL:-"ghcr.io/silogen/kaiwo"}
CLUSTER_NAME=${CLUSTER_NAME:-"kaiwo-test"}
VERSION_TAG=${1:?Usage: $0 <version_tag>}

echo "Snapshotting Kind cluster '$CLUSTER_NAME' with tag '$VERSION_TAG'..."

CONTAINERS=$(docker ps -a --filter "label=io.x-k8s.kind.cluster=$CLUSTER_NAME" --format '{{.Names}}')

if [[ -z "$CONTAINERS" ]]; then
  echo "No containers found for cluster '$CLUSTER_NAME'"
  exit 1
fi

# Stop all services and archive /var
for CONTAINER in $CONTAINERS; do
  echo "  $CONTAINER: stopping containers and services..."
  docker exec "$CONTAINER" bash -c '
    # Stop all containers gracefully via crictl
    CONTAINER_IDS=$(crictl ps -q 2>/dev/null || true)
    if [[ -n "$CONTAINER_IDS" ]]; then
      echo "Stopping containers: $CONTAINER_IDS"
      echo "$CONTAINER_IDS" | xargs -r -P 10 crictl stop -t 3 2>/dev/null || true
    fi
    
    # Stop containerd and kubelet
    systemctl stop kubelet containerd 2>/dev/null || true
    
    # Wait for everything to settle
    sleep 3
  '
  
  echo "  $CONTAINER: archiving /var..."
  docker exec "$CONTAINER" tar -C /var -czf /var_snapshot.tar.gz . &
done
wait

echo "Stopping containers..."
docker stop $CONTAINERS

echo "Committing snapshots..."
for CONTAINER in $CONTAINERS; do
  IMAGE_NAME="${REPO_URL}/kind-snapshot-${CONTAINER}:${VERSION_TAG}"
  echo "  $CONTAINER -> $IMAGE_NAME"
  docker commit "$CONTAINER" "$IMAGE_NAME"
done

echo "Restarting containers..."
docker start $CONTAINERS

echo "Snapshot complete! Images created:"
docker images --filter "reference=${REPO_URL}/kind-snapshot-*:${VERSION_TAG}" --format "  {{.Repository}}:{{.Tag}}"