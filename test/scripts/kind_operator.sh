#!/usr/bin/env bash
set -euo pipefail

# Change to repo root regardless of where script is called from
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

# ---------- Config (override via env) ----------
IMAGE="${IMAGE:-ghcr.io/silogen/kaiwo-operator:v-e2e}"
TAG="${TAG:-v-e2e}"
KIND_CLUSTER="${KIND_CLUSTER:-kaiwo-test}"
NS="${KAIWO_SYSTEM_NAMESPACE:-kaiwo-system}"
OPERATOR_SELECTOR="${OPERATOR_SELECTOR:-app.kubernetes.io/name=kaiwo}"  # adjust if needed
KAIWO_CONFIG="${KAIWO_CONFIG:-${KAIWO_TEST_KAIWOCONFIG:-}}"                      # compat with your env var
METRICS_RB_NAME="${METRICS_RB_NAME:-kaiwo-metrics-access}"                       # adjust to your actual name

# ---------- Logging helpers ----------
c_reset='\033[0m'; c_info='\033[1;36m'; c_ok='\033[1;32m'; c_warn='\033[1;33m'; c_err='\033[1;31m'
info()  { echo -e "${c_info}==>${c_reset} $*"; }
ok()    { echo -e "${c_ok}✔${c_reset} $*"; }
warn()  { echo -e "${c_warn}⚠${c_reset} $*"; }
err()   { echo -e "${c_err}✖${c_reset} $*" >&2; }

need() {
  command -v "$1" >/dev/null 2>&1 || { err "Missing required command: $1"; exit 1; }
}

# ---------- Preflight ----------
preflight() {
  need kubectl
  need make
  need kind
}

# ---------- Up steps ----------
build_image() {
  info "Building operator image: ${IMAGE}"
  make docker-build "IMG=${IMAGE}"
  ok "Built ${IMAGE}"
}

create_ns_and_label() {
  info "Ensuring namespace ${NS} exists and is labeled for restricted PSP"
  kubectl get ns "${NS}" >/dev/null 2>&1 || kubectl create ns "${NS}"
  kubectl label --overwrite ns "${NS}" "pod-security.kubernetes.io/enforce=restricted"
  ok "Namespace ${NS} ready"
}

generate_crds_and_manifests() {
  info "Generating CRDs (make generate) and manifests (make manifests)"
  make generate
  make manifests
  ok "CRDs & manifests generated"
}

build_installer() {
  info "Building installer bundle (make build-installer TAG=${TAG})"
  make build-installer "TAG=${TAG}"
  ok "Installer built at dist/install.yaml"
}

prep_kustomize_overlay() {
  info "Preparing test overlay (copy dist/install.yaml → test/merged.yaml)"
  cp dist/install.yaml test/merged.yaml
  ok "Overlay ready (test/merged.yaml)"
}

kind_load_image() {
  info "Loading image into Kind cluster \"${KIND_CLUSTER}\""
  kind load docker-image "${IMAGE}" --name "${KIND_CLUSTER}"
  ok "Image loaded into Kind"
}

deploy_operator() {
  info "Deploying operator"
  kubectl apply --server-side -k test/
  ok "Manifests applied"
}

wait_operator_ready() {
  info "Waiting for operator pods to be Ready (selector: ${OPERATOR_SELECTOR})"
  # wait for at least one pod to exist
  for i in {1..60}; do
    if kubectl -n "${NS}" get pods -l "${OPERATOR_SELECTOR}" --no-headers 2>/dev/null | grep -q .; then
      break
    fi
    sleep 2
  done
  kubectl -n "${NS}" wait --for=condition=Ready pods -l "${OPERATOR_SELECTOR}" --timeout=180s
  ok "Operator is Ready"
}

apply_kaiwo_config_if_set() {
  if [[ -n "${KAIWO_CONFIG}" ]]; then
    info "Applying KaiwoConfig from: ${KAIWO_CONFIG}"
    kubectl apply -f "${KAIWO_CONFIG}"
    ok "KaiwoConfig applied"
  else
    warn "KAIWO_CONFIG not set; skipping KaiwoConfig apply"
  fi
}

cmd_up() {
  preflight
  build_image
  create_ns_and_label
  generate_crds_and_manifests
  build_installer
  prep_kustomize_overlay
  kind_load_image
  deploy_operator
  wait_operator_ready
  apply_kaiwo_config_if_set
  ok "Kind 'up' completed."
}

# ---------- Down steps ----------
cleanup_chainsaw_namespaces() {
  info "Cleaning Chainsaw namespaces (prefix chainsaw-)"
  kubectl get ns -o name --no-headers | sed 's#^namespace/##' | grep '^chainsaw-' || true
  to_delete=$(kubectl get ns -o name --no-headers | sed 's#^namespace/##' | grep '^chainsaw-' || true)
  if [[ -n "${to_delete}" ]]; then
    echo "${to_delete}" | xargs -r -n1 kubectl delete ns --ignore-not-found
  fi
  ok "Chainsaw namespaces cleanup done"
}

strip_kaiwojob_finalizers() {
  info "Removing KaiwoJob finalizers (kaiwo.silogen.ai/finalizer) if present"
  if command -v jq >/dev/null 2>&1; then
    if kubectl get kaiwojob -A -o json >/dev/null 2>&1; then
      kubectl get kaiwojob -A -o json \
        | jq '(.items[]? | select(.metadata.finalizers != null) | .metadata.finalizers) |=
              (map(select(. != "kaiwo.silogen.ai/finalizer")))' \
        | kubectl apply -f - || true
      ok "Finalizers pruned via jq"
    else
      warn "No KaiwoJobs found"
    fi
  else
    # fallback: simple replace (only works if finalizers is exactly the single value)
    warn "jq not found; using naive finalizer replacement"
    if kubectl get kaiwojob -A -o json >/dev/null 2>&1; then
      kubectl get kaiwojob -A -o json \
        | sed 's/"finalizers":\["kaiwo.silogen.ai\/finalizer"\]/"finalizers":[]/g' \
        | kubectl apply -f - || true
      ok "Finalizers replaced (best-effort)"
    fi
  fi
}

delete_misc() {
  info "Deleting curl-metrics pod (if any)"
  kubectl -n "${NS}" delete pod curl-metrics --force --grace-period=0 --ignore-not-found || true

  info "Deleting metrics ClusterRoleBinding: ${METRICS_RB_NAME} (if any)"
  kubectl delete clusterrolebinding "${METRICS_RB_NAME}" --ignore-not-found || true

  info "Deleting all KaiwoJobs (force)"
  kubectl delete kaiwojob -A --all --timeout=30s --force --grace-period=0 || true
}

undeploy_operator() {
  info "Undeploying operator (-k test/)"
  kubectl delete -k test/ --force --grace-period=0 || true
}

maybe_delete_ns() {
  info "Deleting namespace ${NS} (optional)"
  kubectl delete ns "${NS}" --ignore-not-found || true
}

cmd_down() {
  preflight
  cleanup_chainsaw_namespaces
  strip_kaiwojob_finalizers
  delete_misc
  undeploy_operator
  # Give background deletions a moment to proceed
  info "Waiting briefly for resource deletions"
  sleep 5
  maybe_delete_ns
  ok "Kind 'down' completed."
}

# ---------- Main ----------
usage() {
  cat <<EOF
Usage: $0 {up|down}

Env vars:
  IMAGE                   Operator image (default: ${IMAGE})
  TAG                     Installer tag (default: ${TAG})
  KIND_CLUSTER            Kind cluster name (default: ${KIND_CLUSTER})
  KAIWO_SYSTEM_NAMESPACE  Namespace for operator (default: ${NS})
  OPERATOR_SELECTOR       Label selector for operator pods (default: ${OPERATOR_SELECTOR})
  KAIWO_CONFIG            Path to KaiwoConfig manifest to apply (optional)
  METRICS_RB_NAME         ClusterRoleBinding name for metrics (default: ${METRICS_RB_NAME})
EOF
}

case "${1:-}"  in
  up)   shift; cmd_up   "$@";;
  down) shift; cmd_down "$@";;
  *) usage; exit 1;;
esac
