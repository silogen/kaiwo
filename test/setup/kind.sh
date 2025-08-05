#!/bin/bash

set -e

TEST_NAME=${TEST_NAME:-"kaiwo-test"}
FLUX_BRANCH=${FLUX_BRANCH:-"main"}
FLUX_COMMIT=${FLUX_COMMIT:-""}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse arguments
for arg in "$@"; do
  case $arg in
    --debug-dev)
      DEBUG_DEV=true
      ;;
    --help)
      echo "Usage: $0 [--debug-dev]"
      echo "  --debug-dev    Enable development debugging setup (certs, kubeconfig, make commands)"
      echo ""
      echo "Environment variables:"
      echo "  FLUX_BRANCH    Git branch for Flux to track (default: main)"
      echo "  FLUX_COMMIT    Specific commit for Flux to track (optional)"
      exit 0
      ;;
    *)
      echo "Unknown option: $arg"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

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

echo "Labeling nodes..."

kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 run.ai/simulated-gpu-node-pool=default
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 nvidia.com/gpu.product=Tesla-K80
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 nvidia.com/gpu.count=8
kubectl label node "$TEST_NAME"-worker "$TEST_NAME"-worker2 "$TEST_NAME"-worker3 "$TEST_NAME"-worker4 kaiwo.silogen.ai/node.gpu.partitioned=false

echo "Check if Flux CLI is present..."
if ! command -v flux &> /dev/null; then
    echo "Flux CLI not found. Please install it first:"
    echo "curl -s https://fluxcd.io/install.sh | sudo bash"
    exit 1
fi

echo "Installing Flux on the cluster..."
flux install

# Set git reference for sources
GIT_REF="main"
if [ -n "$FLUX_COMMIT" ]; then
    echo "Using commit: $FLUX_COMMIT"
    GIT_REF="$FLUX_COMMIT"
elif [ "$FLUX_BRANCH" != "main" ]; then
    echo "Using branch: $FLUX_BRANCH"
    GIT_REF="$FLUX_BRANCH"
fi

echo "Applying Flux sources from GitHub repo..."
kubectl apply -k "https://github.com/silogen/kaiwo//automation/flux/sources?ref=$GIT_REF"

echo "Waiting for sources to be ready..."
kubectl wait --for=condition=ready gitrepository/kaiwo-source -n flux-system --timeout=300s
kubectl wait --for=condition=ready helmrepository --all -n flux-system --timeout=300s

# Patch git repository with branch/commit if specified (after sources are applied)
if [ -n "$FLUX_COMMIT" ]; then
    echo "Updating git repository to track commit: $FLUX_COMMIT"
    kubectl patch gitrepository kaiwo-source -n flux-system --type='merge' -p="{\"spec\":{\"ref\":{\"commit\":\"$FLUX_COMMIT\"}}}"
elif [ "$FLUX_BRANCH" != "main" ]; then
    echo "Updating git repository to track branch: $FLUX_BRANCH"
    kubectl patch gitrepository kaiwo-source -n flux-system --type='merge' -p="{\"spec\":{\"ref\":{\"branch\":\"$FLUX_BRANCH\"}}}"
fi

echo "Applying dev-kind infrastructure configuration..."
kubectl apply -f "https://raw.githubusercontent.com/silogen/kaiwo/$GIT_REF/automation/flux/clusters/dev-kind/infrastructure.yaml"

echo "Waiting for kaiwo-infrastructure to be ready..."
echo "This may take several minutes as all components are installed and configured..."
kubectl wait --for=condition=ready kustomization/kaiwo-infrastructure -n flux-system --timeout=600s

echo "Waiting for all Helm releases to be ready..."
kubectl wait --for=condition=ready helmrelease --all -n flux-system --timeout=600s

echo "Creating test namespace..."
kubectl create ns "$TEST_NAME" --dry-run=client -o yaml | kubectl apply -f -

echo "Base cluster setup complete!"

echo "Cluster is ready for testing!"