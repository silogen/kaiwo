#!/bin/bash

set -e

TEST_NAME=${TEST_NAME:-"kaiwo-test"}
#NFS_DIR="/nfs-data"

SKIP_DEPENDENCIES=false

for arg in "$@"; do
  if [[ "$arg" == "--no-dependencies" ]]; then
    SKIP_DEPENDENCIES=true
  fi
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

create_cluster() {
  echo "Creating Kind cluster '$TEST_NAME'..."
  kind create cluster --name "$TEST_NAME" --config test/kind-test-cluster.yaml
}

# Check if the Kind cluster exists
if kind get clusters | grep -q "$TEST_NAME"; then
        echo "Deleting existing Kind cluster '$TEST_NAME'..."
        kind delete cluster --name "$TEST_NAME"
        create_cluster
else
    create_cluster
fi

SCRIPT_PATH="$SCRIPT_DIR/../dependencies/install_dependencies.sh"

if [ "$SKIP_DEPENDENCIES" = false ]; then
  bash "$SCRIPT_PATH" --local
else
  echo "Skipping standard dependency installation"
fi

# Add fake-gpu-operator
kubectl create ns gpu-operator
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged 
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 run.ai/simulated-gpu-node-pool=default
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 nvidia.com/gpu.product=Tesla-K80
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 nvidia.com/gpu.count=8
kubectl apply -f test/fake-gpu-operator/fake-gpu-operator.yaml

## Deploy NFS-backed storageclass
#if ! systemctl is-active --quiet nfs-server || [[ ! -d "$NFS_DIR" ]]; then
#    echo "NFS server is not running OR $NFS_DIR is missing!"
#    echo "Running 'setup_nfs_server.sh' to set up NFS..."
#
#    sudo bash test/setup_nfs_server.sh
#
#    if ! systemctl is-active --quiet nfs-server || [[ ! -d "$NFS_DIR" ]]; then
#        echo "NFS setup failed. Please check logs."
#        exit 1
#    fi
#
#    echo "NFS setup completed successfully!"
#fi
#
#echo "Available interfaces:"
#ip -4 addr show
#
#HOST_IP=$(ip -4 addr show | awk '/inet / {print $2, $NF}' | grep -E '^(10\.|172\.16\.|172\.17\.|172\.18\.|172\.19\.|172\.20\.|172\.21\.|172\.22\.|172\.23\.|172\.24\.|172\.25\.|172\.26\.|172\.27\.|172\.28\.|172\.29\.|172\.30\.|172\.31\.|192\.168\.)' | grep -vE 'docker|br-' | awk '{print $1}' | cut -d'/' -f1 | head -n 1)
#
## Debugging output
#echo "Detected candidate IPs: $HOST_IP"
#
#if [[ -z "$HOST_IP" ]]; then
#    echo "No suitable private IP found for NFS server!"
#    exit 1
#fi
#
#echo "Using IP: $HOST_IP for NFS provisioner"
#
#echo "Installing NFS provisioner..."
#helm repo add nfs-subdir-external-provisioner https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner/
#helm repo update
#
#helm install nfs-subdir-external-provisioner nfs-subdir-external-provisioner/nfs-subdir-external-provisioner \
#    --set nfs.server="$HOST_IP" \
#    --set nfs.path=$NFS_DIR
#
#echo "NFS provisioner deployed successfully!"

kubectl create ns "$TEST_NAME"

find config/static -name '*.yaml' -print0 | xargs -0 -n1 kubectl apply -f
make install
kubectl apply -f test/kaiwoconfig.yaml

echo "Cluster is ready!"

