#!/bin/bash
set -euo pipefail

# --- Resolve absolute paths ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ENVIRONMENT=${1:-}
ACTION=${2:-}

if [ -z "$ENVIRONMENT" ] || [ -z "$ACTION" ]; then
  echo "Usage: $0 <environment> <up|down>"
  echo "Available environments:"
  ls "${SCRIPT_DIR}/environments/" | sed 's/.yaml$//' | sed 's/^/  /'
  exit 1
fi

HELMFILE_ABS="${SCRIPT_DIR}/helmfile.yaml.gotmpl"
ENV_FILE_ABS="${SCRIPT_DIR}/environments/${ENVIRONMENT}.yaml"
KUSTOMIZE_CLIENT="${SCRIPT_DIR}/kustomization-client-side"
KUSTOMIZE_SERVER="${SCRIPT_DIR}/kustomization-server-side/overlays/environments/${ENVIRONMENT}"
POST_HELM="${SCRIPT_DIR}/post-helm"

# Validate inputs
[[ -f "$HELMFILE_ABS" ]] || { echo "Error: Helmfile not found: $HELMFILE_ABS"; exit 1; }
[[ -f "$ENV_FILE_ABS"  ]] || { echo "Error: Env values file not found: $ENV_FILE_ABS"; exit 1; }

# Helpers
safe_kubectl_delete_k() {
  local path="$1"
  if [ -d "$path" ]; then
    kubectl delete -k "$path" --ignore-not-found=true || true
  else
    echo "Skip delete: kustomize path '$path' not found."
  fi
}
safe_kubectl_apply_k() {
  local path="$1"
  if [ -d "$path" ]; then
    kubectl apply -k "$path"
  else
    echo "Skip apply: kustomize path '$path' not found."
  fi
}
safe_helmfile() {
  local subcmd="$1"; shift
  helmfile -f "$HELMFILE_ABS" --state-values-file "$ENV_FILE_ABS" "$subcmd" "$@" || true
}

case "$ACTION" in
  up)
    echo "Deploying dependencies for environment: $ENVIRONMENT"

    echo "1. Applying client-side Kustomize resources..."
    safe_kubectl_apply_k "$KUSTOMIZE_CLIENT"

    # While we wait for cert-manager (the hard dependency), do SAFE background work:
    #  - pre-render server-side kustomize manifest (no cluster changes yet)
    #  - helm repo updates (so sync doesn't stall)
    TMP_BUILD="$(mktemp)"
    render_pid=""
    if [ -d "$KUSTOMIZE_SERVER" ]; then
      ( kubectl kustomize "$KUSTOMIZE_SERVER" > "$TMP_BUILD" ) &
      render_pid=$!
    fi
    ( safe_helmfile repos ) &

    echo "Waiting for Cert-Manager to be deployed (gate for downstream components)..."
    for deploy in cert-manager cert-manager-webhook cert-manager-cainjector; do
      echo "Waiting for deployment: $deploy"
      if kubectl get "deployment/$deploy" -n cert-manager >/dev/null 2>&1; then
        kubectl rollout status "deployment/$deploy" -n cert-manager --timeout=5m
      else
        echo "Warning: $deploy deployment not found, skipping wait."
      fi
    done
    # Join the background render (if any) now that cert-manager is ready
    if [ -n "${render_pid}" ]; then wait "${render_pid}" || true; fi

    echo "2. Applying server-side Kustomize resources from $KUSTOMIZE_SERVER..."
    if [ -d "$KUSTOMIZE_SERVER" ]; then
      # Apply CRDs first (server-side) from the pre-rendered manifest
      yq eval 'select(.kind == "CustomResourceDefinition")' "$TMP_BUILD" \
        | kubectl apply --server-side --force-conflicts -f -

       # Robust CRD wait (tolerate transient nil conditions)
      yq -r 'select(.kind == "CustomResourceDefinition") | .metadata.name' "$TMP_BUILD" \
        | xargs -r -n1 -P8 -I{} bash -c '
            CRD="{}"
            for i in {1..6}; do
              if kubectl wait --for=condition=Established --timeout=15s "crd/${CRD}" >/dev/null 2>&1; then
                echo "CRD ${CRD}: Established"
                exit 0
              fi
              sleep 3
            done
            echo "WARN: CRD ${CRD} not reported Established after retries; continuing"
            exit 0
          '

      # Apply the rest (server-side)
      kubectl apply --server-side --force-conflicts -f "$TMP_BUILD"
      rm -f "$TMP_BUILD"
    else
      echo "Skip apply: kustomize path '$KUSTOMIZE_SERVER' not found."
      [ -f "$TMP_BUILD" ] && rm -f "$TMP_BUILD"
    fi

    echo "Waiting for other dependencies to be deployed..."
    pids=()
    (kubectl rollout status deployment/kueue-controller-manager -n kueue-system --timeout=5m || true) & pids+=($!)
    (kubectl rollout status deployment/kuberay-operator --timeout=5m || true) & pids+=($!)
    (kubectl rollout status deployment/appwrapper-controller-manager -n appwrapper-system --timeout=5m || true) & pids+=($!)
    for pid in "${pids[@]}"; do wait "$pid" || true; done

    echo "3. Installing Helm charts..."
    safe_helmfile sync

    echo "4. Applying post-Helm Kustomize resources..."
    safe_kubectl_apply_k "$POST_HELM"

    echo "Deployment complete for environment: $ENVIRONMENT"
    ;;

  down)
    echo "Tearing down environment: $ENVIRONMENT (reverse order, ignore missing)"

    echo "1. Deleting post-Helm Kustomize resources..."
    safe_kubectl_delete_k "$POST_HELM"

    echo "2. Uninstalling Helm charts..."
    safe_helmfile destroy

    echo "3. Deleting server-side Kustomize resources..."
    safe_kubectl_delete_k "$KUSTOMIZE_SERVER"

    echo "Waiting briefly to ensure dependent resources finalize..."
    sleep 5

    echo "4. Deleting client-side Kustomize resources..."
    safe_kubectl_delete_k "$KUSTOMIZE_CLIENT"

    echo "Teardown complete for environment: $ENVIRONMENT"
    ;;

  *)
    echo "Error: Action must be 'up' or 'down'"
    exit 1
    ;;
esac
