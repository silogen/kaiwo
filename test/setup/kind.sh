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

echo "Waiting for nodes to be ready"
kubectl wait --for=condition=Ready nodes --all --timeout=300s

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

# choose infra folder based on environment
INFRA_DIR="dev-kind"
if [ "${GITHUB_ACTIONS:-false}" = "true" ] || [ "${CI:-false}" = "true" ]; then
  INFRA_DIR="dev-kind-ci"
fi

echo "Applying $INFRA_DIR infrastructure configuration..."
kubectl apply -f "https://raw.githubusercontent.com/silogen/kaiwo/$GIT_REF/automation/flux/clusters/$INFRA_DIR/infrastructure.yaml"

echo "Waiting for kaiwo-infrastructure to be ready..."
echo "This may take several minutes as all components are installed and configured..."

echo "Checking status of kustomization/kaiwo-infrastructure..."
kubectl get kustomization/kaiwo-infrastructure -n flux-system -o wide

echo "Waiting for Kustomizations to be ready..."
if ! kubectl wait --for=condition=ready kustomization/kaiwo-infrastructure -n flux-system --timeout=600s; then
    echo "❌ kaiwo-infrastructure failed to become ready. Debugging..."
    echo "📋 Kustomization status:"
    kubectl describe kustomization/kaiwo-infrastructure -n flux-system
    echo "🔍 Recent events:"
    kubectl get events -n flux-system --sort-by='.lastTimestamp' --field-selector reason!=Scheduled
    echo "💾 Kustomize controller logs:"
    kubectl logs -n flux-system -l app=kustomize-controller --tail=100 --prefix=true || true
    exit 1
fi

echo "Waiting for all Helm releases to be ready..."
echo "Checking current status of Helm releases..."
kubectl get helmrelease -A -o wide

echo "Checking Flux logs for any errors..."
kubectl logs -n flux-system -l app=source-controller --tail=50 --prefix=true || true
kubectl logs -n flux-system -l app=helm-controller --tail=50 --prefix=true || true

echo "Attempting to wait for Helm releases..."
if ! kubectl wait --for=condition=ready helmrelease --all -n flux-system --timeout=600s; then
    echo "❌ Helm releases failed to become ready. Debugging..."
    
    echo "📊 Final status of all Helm releases:"
    kubectl get helmrelease -A -o wide
    
    echo "📋 Detailed status of failed Helm releases:"
    kubectl get helmrelease -A -o json | jq -r '.items[] | select(.status.conditions // [] | map(.status) | contains(["False"])) | "\(.metadata.namespace)/\(.metadata.name): \(.status.conditions // [] | map(select(.status == "False")) | .[0].message // "Unknown error")"'
    
    echo "🔍 Events in flux-system namespace:"
    kubectl get events -n flux-system --sort-by='.lastTimestamp' --field-selector type!=Normal
    
    echo "📦 Pod status in flux-system:"
    kubectl get pods -n flux-system -o wide
    
    echo "💾 Recent logs from Helm controller:"
    kubectl logs -n flux-system -l app=helm-controller --tail=100 --prefix=true || true
    
    echo "💾 Recent logs from Source controller:"
    kubectl logs -n flux-system -l app=source-controller --tail=100 --prefix=true || true

    echo "Monitoring debug"

    kubectl get helmrelease loki -n flux-system -o yaml

    kubectl get pods -n monitoring -o wide || true
    kubectl describe pods -n monitoring || true
    kubectl logs -n monitoring -l app.kubernetes.io/name=loki --tail=50 || true

    exit 1
fi

echo "Creating test namespace..."
kubectl create ns "$TEST_NAME" --dry-run=client -o yaml | kubectl apply -f -

echo "Base cluster setup complete!"

echo "Cluster is ready for testing!"