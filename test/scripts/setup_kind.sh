#!/bin/bash

set -euo pipefail

TEST_NAME=${TEST_NAME:-"kaiwo-test"}

SKIP_DEPENDENCIES=false
SKIP_KAIWO_STATIC=false

for arg in "$@"; do
  if [[ "$arg" == "--no-dependencies" ]]; then
    SKIP_DEPENDENCIES=true
  fi
  if [[ "$arg" == "--skip-static" ]]; then
    SKIP_KAIWO_STATIC=true
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

NODES=(
  "${TEST_NAME}-worker"
  "${TEST_NAME}-worker2"
  "${TEST_NAME}-worker3"
  "${TEST_NAME}-worker4"
)

# Some setups may not have all 4 workers; filter to only existing nodes.
EXISTING_NODES=()
for n in "${NODES[@]}"; do
  if kubectl get node "$n" >/dev/null 2>&1; then
    EXISTING_NODES+=("$n")
  fi
done

if ((${#EXISTING_NODES[@]})); then
  echo "Labeling nodes: ${EXISTING_NODES[*]}"
  # Combine all labels in ONE kubectl call (much faster) and overwrite if present
  kubectl label node "${EXISTING_NODES[@]}" \
    run.ai/simulated-gpu-node-pool=default \
    nvidia.com/gpu.product=Tesla-K80 \
    nvidia.com/gpu.count=8 \
    feature.node.kubernetes.io/pci-10de.present=true \
    kaiwo.silogen.ai/node.gpu.partitioned=false \
    amd.com/gpu.product-name=AMD_Instinct_MI300X_OAM \
    --overwrite
else
  echo "No matching worker nodes found to label (skipping)."
fi


if [ "$SKIP_KAIWO_STATIC" = false ]; then
  kubectl apply -R -f config/static
fi


make generate
make manifests
make install

kubectl apply -f test/configs/kaiwoconfig/local-kind.yaml

kind get kubeconfig -n "$TEST_NAME" > kaiwo_test_kubeconfig.yaml 

# if [[ -n "${CI:-}"  ]]; then
#   echo "Running in CI. Skipping cert generation"
# else  
#   bash test/scripts/generate_certs.sh
# fi

WEBHOOK_CERT_DIRECTORY=$(pwd)/certs
KUBECONFIG=$(pwd)/kaiwo_test_kubeconfig.yaml

ENV_FILE=".env"

update_env_var() {
    local var_name="$1"
    local var_value="$2"

    if grep -q "^${var_name}=" "$ENV_FILE"; then
        sed -i "s|^${var_name}=.*|${var_name}=${var_value}|" "$ENV_FILE"
    else
        echo "${var_name}=${var_value}" >> "$ENV_FILE"
    fi
}

touch "$ENV_FILE"


# update_env_var "WEBHOOK_CERT_DIRECTORY" "$WEBHOOK_CERT_DIRECTORY"
update_env_var "KUBECONFIG" "$KUBECONFIG"



sh test/aimdummy/populate_kind.sh

# When running debugger in IDE
kubectl create ns kaiwo-system

echo "You can now run debugger in your IDE"

echo "Cluster is ready!"

