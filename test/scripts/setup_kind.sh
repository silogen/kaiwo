#!/bin/bash

set -e

TEST_NAME=${TEST_NAME:-"kaiwo-test"}

SKIP_DEPENDENCIES=false

for arg in "$@"; do
  if [[ "$arg" == "--no-dependencies" ]]; then
    SKIP_DEPENDENCIES=true
  fi
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

create_cluster() {
  echo "Creating Kind cluster '$TEST_NAME'..."
  kind create cluster --name "$TEST_NAME" --config test/kind/kind-test-cluster.yaml
}

# Check if the Kind cluster exists
if kind get clusters | grep -q "$TEST_NAME"; then
        echo "Deleting existing Kind cluster '$TEST_NAME'..."
        kind delete cluster --name "$TEST_NAME"
        create_cluster
else
    create_cluster
fi

if [ "$SKIP_DEPENDENCIES" = false ]; then
  bash "dependencies/deploy.sh" kind-test up
else
  echo "Skipping standard dependency installation"
fi

kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 run.ai/simulated-gpu-node-pool=default
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 nvidia.com/gpu.product=Tesla-K80
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 nvidia.com/gpu.count=8

find config/static -name '*.yaml' -print0 | xargs -0 -n1 kubectl apply -f
make install

kubectl apply -f test/configs/kaiwoconfig/local-kind.yaml

echo "Cluster is ready!"

