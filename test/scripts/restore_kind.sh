#!/bin/bash
set -euo pipefail

CLUSTER_NAME=${CLUSTER_NAME:-"kaiwo-test"}
VERSION_TAG=${1:?Usage: $0 <version_tag> [num_workers]}
NUM_WORKERS=${2:-5}

echo "Restoring Kind cluster '$CLUSTER_NAME' from tag '$VERSION_TAG'..."

# Expected container names
CONTROL_PLANE="${CLUSTER_NAME}-control-plane"
WORKERS=()
for i in $(seq 1 $NUM_WORKERS); do
  [[ $i -eq 1 ]] && WORKERS+=("${CLUSTER_NAME}-worker") || WORKERS+=("${CLUSTER_NAME}-worker${i}")
done

ALL_NODES=("$CONTROL_PLANE" "${WORKERS[@]}")

# Verify all snapshot images exist
echo "Verifying snapshot images..."
for NODE in "${ALL_NODES[@]}"; do
  IMAGE="kind-snapshot-${NODE}:${VERSION_TAG}"
  if ! docker image inspect "$IMAGE" &>/dev/null; then
    echo "ERROR: Missing snapshot image: $IMAGE"
    exit 1
  fi
done

echo "Stopping containers..."
docker stop "${ALL_NODES[@]}" 2>/dev/null || true

echo "Restoring from snapshots..."
for NODE in "${ALL_NODES[@]}"; do
  IMAGE="kind-snapshot-${NODE}:${VERSION_TAG}"
  
  # Get the current container's config for recreation
  if docker inspect "$NODE" &>/dev/null; then
    # Extract key settings
    NETWORK=$(docker inspect "$NODE" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')
    PRIVILEGED=$(docker inspect "$NODE" --format '{{.HostConfig.Privileged}}')
    
    # Extract port mappings (critical for control-plane!)
    PORT_ARGS=""
    if [[ "$NODE" == *"control-plane"* ]]; then
      HOST_PORT=$(docker inspect "$NODE" --format '{{(index (index .HostConfig.PortBindings "6443/tcp") 0).HostPort}}' 2>/dev/null || true)
      if [[ -n "$HOST_PORT" ]]; then
        PORT_ARGS="-p 127.0.0.1:${HOST_PORT}:6443"
        echo "  Preserving API server port mapping: ${HOST_PORT}:6443"
      else
        echo "  WARNING: Could not detect original API server port!"
      fi
    fi
    
    docker rm "$NODE"

    # Create new container from snapshot, copying critical settings
    
    docker create \
      --name "$NODE" \
      --hostname "$NODE" \
      --privileged="$PRIVILEGED" \
      --network "$NETWORK" \
      $PORT_ARGS \
      --label "io.x-k8s.kind.cluster=$CLUSTER_NAME" \
      --label "io.x-k8s.kind.role=$(echo $NODE | grep -q control-plane && echo control-plane || echo worker)" \
      --tmpfs /tmp --tmpfs /run \
      --volume /var --volume /lib/modules:/lib/modules:ro \
      "$IMAGE"
    
    echo "  Restored: $NODE"
  fi
done

echo "Starting containers..."
docker start "${ALL_NODES[@]}"

echo "Waiting for containerd to start..."
sleep 10

# Restore the /var directory of each container
for NODE in "${ALL_NODES[@]}"; do
    docker exec "$NODE" bash -c '
        echo "Restoring /var from snapshot... on '"$NODE"'"
        systemctl stop containerd kubelet
        # Extract, ignoring device node creation failures
        tar -C /var -xzf /var_snapshot.tar.gz 2>&1 | grep -v "Cannot mknod" || true
        echo "    Restored /var (some device nodes skipped)"
        systemctl start containerd kubelet
    ' &
done
wait

# Fix node IPs (after /var restore overwrote kubelet config with old IPs)
echo "Fixing node IPs..."
for NODE in "${ALL_NODES[@]}"; do
  NEW_IP=$(docker inspect "$NODE" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
  echo "  $NODE -> $NEW_IP"
  
  docker exec "$NODE" sed -i "s/--node-ip=[^ \"]*/--node-ip=$NEW_IP/" /var/lib/kubelet/kubeadm-flags.env 2>/dev/null || true
  
  if [[ "$NODE" == *"control-plane"* ]]; then
    docker exec "$NODE" sed -i "s/--advertise-address=[0-9.]*/--advertise-address=$NEW_IP/" /etc/kubernetes/manifests/kube-apiserver.yaml 2>/dev/null || true
  fi
done


# Restart kubelet to let it pick up the new IP addresses and var directory
echo "Restarting kubelet..."
for NODE in "${ALL_NODES[@]}"; do
  docker exec "$NODE" systemctl restart kubelet &
done
wait

# Extract kubeconfig from the control plane node
echo "Extracting kubeconfig..."
KUBECONFIG_FILE="$(pwd)/kaiwo_test_kubeconfig.yaml"

if kind get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG_FILE" 2>/dev/null; then
  echo "Kubeconfig extracted via kind"
else
  echo "Falling back to direct extraction from container..."
  docker cp "${CONTROL_PLANE}:/etc/kubernetes/admin.conf" "$KUBECONFIG_FILE"
  
  API_PORT=$(docker inspect "${CONTROL_PLANE}" --format '{{range $p, $conf := .NetworkSettings.Ports}}{{if eq $p "6443/tcp"}}{{(index $conf 0).HostPort}}{{end}}{{end}}')
  [[ -n "$API_PORT" ]] && sed -i "s|server: https://.*:6443|server: https://127.0.0.1:${API_PORT}|g" "$KUBECONFIG_FILE"
fi

echo "Kubeconfig saved to: $KUBECONFIG_FILE"

kubectl --kubeconfig=$KUBECONFIG_FILE get nodes || true

echo "Restore complete!"