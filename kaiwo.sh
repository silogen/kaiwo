#!/usr/bin/env bash
set -euo pipefail

# Change to repo root regardless of where script is called from
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# ---------- Default Configuration ----------
IMAGE_REGISTRY="${IMAGE_REGISTRY:-ghcr.io/silogen}"
TAG="${TAG:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"
TTL="${TTL:-1h}"
KIND_CLUSTER="${KIND_CLUSTER:-kaiwo-test}"
HELM_RELEASE_NAME="${HELM_RELEASE_NAME:-kaiwo}"
HELM_NAMESPACE="${HELM_NAMESPACE:-kaiwo-system}"
CONTAINER_TOOL="${CONTAINER_TOOL:-docker}"

# Image push/deploy selection
PUSH=""
DEPLOY_VIA=""
BUILD=false
INSTALL_CRDS=false
COMMAND=""
ORIGINAL_ARGS_COUNT=$#

# ---------- Logging helpers ----------
c_reset='\033[0m'; c_info='\033[1;36m'; c_ok='\033[1;32m'; c_warn='\033[1;33m'; c_err='\033[1;31m'
info()  { echo -e "${c_info}==>${c_reset} $*"; }
ok()    { echo -e "${c_ok}✔${c_reset} $*"; }
warn()  { echo -e "${c_warn}⚠${c_reset} $*"; }
err()   { echo -e "${c_err}✖${c_reset} $*" >&2; }

need() {
  command -v "$1" >/dev/null 2>&1 || { err "Missing required command: $1"; exit 1; }
}

# ---------- Help and usage ----------
usage() {
  cat <<'EOF'
Usage: ./kaiwo.sh [OPTIONS] [COMMAND]

A unified script for building and deploying the Kaiwo operator using multiple methods.

OPTIONS:
  --build                 Build the operator image
  --push=TARGET           Push target: ttl.sh|kind
  --deploy-via=METHOD     Deployment method: helm|kustomization
  --install-crds          Install CRDs (can be used independently)
  --ttl=DURATION          TTL for ttl.sh uploads (e.g. 30m, 1h, 2d)
  --help                  Show this help

COMMANDS:
  up                      Deploy the operator (default if --push specified)
  down                    Undeploy the operator

ENVIRONMENT VARIABLES:
  IMAGE_REGISTRY          Image registry (default: ghcr.io/silogen)
  IMAGE_NAME              Override image repository/name (or full ref). Examples:
                          - kaiwo-operator
                          - ghcr.io/silogen/kaiwo-operator
                          - ttl.sh/your-uuid:1h
  TAG                     Image tag for fallback paths (default: git short hash)
  TTL                     TTL for ttl.sh (default: 1h)
  KIND_CLUSTER            Kind cluster name (default: kaiwo-test)
  HELM_RELEASE_NAME       Helm release name (default: kaiwo-operator)
  HELM_NAMESPACE          Helm namespace (default: kaiwo-system)
  CONTAINER_TOOL          Container tool (default: docker)  [docker|podman]

EXAMPLES:
  # Install CRDs only
  ./kaiwo.sh --install-crds

  # Build and deploy to ttl.sh with Helm (1h TTL)
  ./kaiwo.sh --install-crds --build --push=ttl.sh --deploy-via=helm up

  # Deploy existing image to Kind with Helm (no build)
  ./kaiwo.sh --install-crds --push=kind --deploy-via=helm up

  # Only build and upload to ttl.sh (no deploy)
  ./kaiwo.sh --build --push=ttl.sh

  # Deploy to Kind with Kustomization
  ./kaiwo.sh --install-crds --push=kind --deploy-via=kustomization up

  # Undeploy Helm installation
  ./kaiwo.sh --deploy-via=helm down

  # Custom TTL for ttl.sh
  ./kaiwo.sh --install-crds --build --push=ttl.sh --deploy-via=helm up --ttl=48h
EOF
}

# ---------- Argument parsing ----------
while [[ $# -gt 0 ]]; do
  case $1 in
    --build) BUILD=true; shift;;
    --push=*) PUSH="${1#*=}"; shift;;
    --deploy-via=*) DEPLOY_VIA="${1#*=}"; shift;;
    --install-crds) INSTALL_CRDS=true; shift;;
    --ttl=*) TTL="${1#*=}"; shift;;
    --help|-h) usage; exit 0;;
    up|down) COMMAND="$1"; shift;;
    *) err "Unknown option: $1"; usage; exit 1;;
  esac
done

# Default command if push is specified but no command given
# Only set default "up" if we also have a deploy method specified
if [[ -n "$PUSH" && -z "$COMMAND" && -n "$DEPLOY_VIA" ]]; then
  COMMAND="up"
fi

# ---------- Image reference cache ----------
IMAGE_REF_FILE=".image-ref"       # stores full image reference (e.g., ttl.sh/<uuid>:1h)
LEGACY_NAME_FILE=".image-name"    # legacy: stored only name/uuid (kept for back-compat)

IMAGE_REF=""

# ---------- Small helpers ----------
uuid_lower() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]'
  elif [[ -r /proc/sys/kernel/random/uuid ]]; then
    tr '[:upper:]' '[:lower:]' < /proc/sys/kernel/random/uuid
  else
    # Very rough fallback
    date +%s%N | sha256sum | head -c 32
  fi
}

# parse_image_ref REF -> sets: PARSED_REGISTRY, PARSED_REPOSITORY, PARSED_TAG (tag may be empty)
parse_image_ref() {
  local ref="${1%%@*}" # strip digest if any
  local tag=""
  if [[ "$ref" == *:* && "$ref" == */*:* ]]; then
    tag="${ref##*:}"
    ref="${ref%:*}"
  fi
  local first="${ref%%/*}"
  local rest="${ref#*/}"
  if [[ "$ref" == "$first" ]]; then
    # No registry part present (e.g., "alpine")
    PARSED_REGISTRY=""
    PARSED_REPOSITORY="$ref"
  else
    PARSED_REGISTRY="$first"
    PARSED_REPOSITORY="$rest"
  fi
  PARSED_TAG="$tag"
}

save_image_ref() {
  printf '%s\n' "$1" > "${IMAGE_REF_FILE}"
  # Keep legacy updated with "name" only for compatibility
  # If ttl.sh, store uuid/name; else store repository last segment
  parse_image_ref "$1"
  local name_part="${PARSED_REPOSITORY##*/}"
  printf '%s\n' "$name_part" > "${LEGACY_NAME_FILE}"
}

# Container ops (docker|podman)
container_build() { "$CONTAINER_TOOL" build -t "$1" .; }
container_push()  { "$CONTAINER_TOOL" push "$1"; }

kind_load_image() {
  local ref="$1"
  if [[ "$CONTAINER_TOOL" == "docker" ]]; then
    kind load docker-image "$ref" --name "${KIND_CLUSTER}"
  else
    # Podman path: save and load archive
    local tmp_tar
    tmp_tar="$(mktemp -t image-XXXXXX.tar)"
    "$CONTAINER_TOOL" save -o "$tmp_tar" "$ref"
    kind load image-archive "$tmp_tar" --name "${KIND_CLUSTER}"
    rm -f "$tmp_tar"
  fi
}

# ---------- Validation ----------
validate_args() {
  # Validate push target
  if [[ -n "$PUSH" && "$PUSH" != "ttl.sh" && "$PUSH" != "kind" ]]; then
    err "Invalid --push target: $PUSH. Must be 'ttl.sh' or 'kind'"
    exit 1
  fi

  # Validate deploy-via method
  if [[ -n "$DEPLOY_VIA" && "$DEPLOY_VIA" != "helm" && "$DEPLOY_VIA" != "kustomization" ]]; then
    err "Invalid --deploy-via method: $DEPLOY_VIA. Must be 'helm' or 'kustomization'"
    exit 1
  fi

  # Validate command
  if [[ -n "$COMMAND" && "$COMMAND" != "up" && "$COMMAND" != "down" ]]; then
    err "Invalid command: $COMMAND. Must be 'up' or 'down'"
    exit 1
  fi

  # Check required tools
  need make
  need kubectl
  if [[ "$BUILD" == true ]]; then
    need "$CONTAINER_TOOL"
  fi
  if [[ "$PUSH" == "kind" ]]; then
    need kind
  fi
  if [[ "$DEPLOY_VIA" == "helm" ]]; then
    need helm
  fi

  # TTL must have a unit
  if [[ "$PUSH" == "ttl.sh" || "${IMAGE_NAME:-}" =~ ^ttl\.sh/ ]]; then
    if [[ ! "$TTL" =~ ^[0-9]+[smhd]$ ]]; then
      err "TTL must include a unit, e.g. '30m', '1h', '2d' (got: $TTL)"
      exit 1
    fi
  fi
}

# ---------- Compute IMAGE_REF ----------
# Priority:
# 1) If IMAGE_NAME provided as a *full reference*, use as-is.
# 2) If IMAGE_NAME provided as a *repository*, add the appropriate tag for the push target.
# 3) If PUSH=ttl.sh and not building but .image-ref exists, reuse it as-is (avoids tag mismatch).
# 4) Else generate appropriate reference.

compute_image_ref() {
  if [[ -n "${IMAGE_NAME:-}" ]]; then
    # If contains ':' after a '/', assume full ref with tag
    if [[ "$IMAGE_NAME" == */*:* || "$IMAGE_NAME" == ttl.sh/*:* ]]; then
      IMAGE_REF="$IMAGE_NAME"
      return
    fi
    # No tag in IMAGE_NAME; add target-appropriate tag
    if [[ "$PUSH" == "ttl.sh" || "$IMAGE_NAME" == ttl.sh/* ]]; then
      # For ttl.sh you typically want a UUID name with TTL tag. If user passed ttl.sh/<name>, respect it.
      if [[ "$IMAGE_NAME" == ttl.sh/* ]]; then
        IMAGE_REF="${IMAGE_NAME}:${TTL}"
      else
        IMAGE_REF="ttl.sh/${IMAGE_NAME}:${TTL}"
      fi
      return
    fi
    # Otherwise assume normal registry/repo with v-e2e tag
    if [[ "$IMAGE_NAME" == */* ]]; then
      IMAGE_REF="${IMAGE_NAME}:v-e2e"
    else
      IMAGE_REF="${IMAGE_REGISTRY}/${IMAGE_NAME}:v-e2e"
    fi
    return
  fi

  # No IMAGE_NAME override
  if [[ "$PUSH" == "ttl.sh" ]]; then
    if [[ "$BUILD" == false && -f "$IMAGE_REF_FILE" ]]; then
      IMAGE_REF="$(cat "$IMAGE_REF_FILE")" # reuse the exact ref (name+tag) previously built
      return
    fi
    # Build path or no cache: create new UUID
    local uuid; uuid="$(uuid_lower)"
    IMAGE_REF="ttl.sh/${uuid}:${TTL}"
    return
  fi

  if [[ "$PUSH" == "kind" ]]; then
    # Fixed name for kind by default
    IMAGE_REF="${IMAGE_REGISTRY}/kaiwo-operator:v-e2e"
    return
  fi

  # Fallback (no push), only meaningful if someone calls kustomize path w/ TAG
  IMAGE_REF="${IMAGE_REGISTRY}/kaiwo-operator:${TAG}"
}

# ---------- Build Functions ----------
build_image() {
  local ref="$1"
  info "Building image: ${ref}"
  container_build "${ref}"
}

build_and_upload_ttl() {
  local ref="$1"
  build_image "${ref}"
  info "Pushing to ttl.sh (${ref})"
  container_push "${ref}"
  save_image_ref "${ref}"
  ok "Image uploaded: ${ref}"
  info "Stored image reference in ${IMAGE_REF_FILE}"
}

build_and_load_kind() {
  local ref="$1"
  build_image "${ref}"
  info "Loading image into Kind cluster: ${KIND_CLUSTER}"
  kind_load_image "${ref}"
  save_image_ref "${ref}"
  ok "Image loaded into Kind: ${ref}"
}

# ---------- CRD Functions ----------
install_crds() {
  info "Installing CRDs"
  make install
  ok "CRDs installed successfully"
}

# ---------- Deploy Functions ----------
helm_chart_path() {
  make helm-package >/dev/null 2>&1
  # Return first matching chart path
  find dist/ -name "kaiwo-*.tgz" -print -quit
}

deploy_helm_with_ref() {
  local ref="$1"
  parse_image_ref "$ref"
  local chart_path
  chart_path="$(helm_chart_path)"
  [[ -n "$chart_path" ]] || { err "Failed to find packaged Helm chart in dist/"; exit 1; }

  # Create namespace if needed
  kubectl get namespace "${HELM_NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${HELM_NAMESPACE}"
  # Pod Security Standards label
  kubectl label --overwrite namespace "${HELM_NAMESPACE}" "pod-security.kubernetes.io/enforce=restricted"

  # image.registry is the hostname (or empty). image.repository is the path. image.tag is tag.
  local img_registry="${PARSED_REGISTRY}"
  local img_repo="${PARSED_REPOSITORY}"
  local img_tag="${PARSED_TAG}"

  info "Deploying via Helm with image: ${img_registry:+$img_registry/}$img_repo${img_tag:+:$img_tag}"
  helm upgrade --install "${HELM_RELEASE_NAME}" "${chart_path}" \
    --namespace "${HELM_NAMESPACE}" \
    ${img_registry:+--set image.registry="${img_registry}"} \
    --set image.repository="${img_repo}" \
    ${img_tag:+--set image.tag="${img_tag}"} \
    --wait

  ok "Helm deployment completed"
}

deploy_kustomization_with_ref() {
  local ref="$1"
  info "Deploying using Kustomization with image: ${ref}"
  make generate
  make manifests
  make build-installer "IMG=${ref}"

  # Prepare test overlay and apply
  cp dist/install.yaml test/merged.yaml
  kubectl apply --server-side -k test/

  # Wait for operator readiness
  local selector="app.kubernetes.io/name=kaiwo"
  info "Waiting for operator pods to be Ready in namespace ${HELM_NAMESPACE}"
  # Wait for at least one pod to appear (max ~120s)
  for _ in {1..60}; do
    if kubectl -n "${HELM_NAMESPACE}" get pods -l "${selector}" --no-headers 2>/dev/null | grep -q .; then
      break
    fi
    sleep 2
  done
  kubectl -n "${HELM_NAMESPACE}" wait --for=condition=Ready pods -l "${selector}" --timeout=180s

  ok "Kustomization deployment completed"
}

# ---------- Undeploy Functions ----------
undeploy_helm() {
  info "Undeploying Helm release: ${HELM_RELEASE_NAME}"
  if helm status "${HELM_RELEASE_NAME}" -n "${HELM_NAMESPACE}" >/dev/null 2>&1; then
    helm uninstall "${HELM_RELEASE_NAME}" -n "${HELM_NAMESPACE}"
    ok "Helm release uninstalled"
  else
    warn "Helm release ${HELM_RELEASE_NAME} not found in ${HELM_NAMESPACE}"
  fi
}

undeploy_kustomization() {
  info "Undeploying Kustomization resources"

  # Clean up Chainsaw namespaces
  info "Cleaning Chainsaw namespaces (prefix chainsaw-)"
  to_delete="$(kubectl get ns -o name --no-headers 2>/dev/null | sed 's#^namespace/##' | grep '^chainsaw-' || true)"
  if [[ -n "${to_delete}" ]]; then
    echo "${to_delete}" | xargs -r -n1 kubectl delete ns --ignore-not-found
  fi

  # Remove KaiwoJob finalizers if present
  info "Removing KaiwoJob finalizers if present"
  if command -v jq >/dev/null 2>&1; then
    if kubectl get kaiwojob -A -o json >/dev/null 2>&1; then
      kubectl get kaiwojob -A -o json \
        | jq '(.items[]? | select(.metadata.finalizers != null) | .metadata.finalizers) |=
              (map(select(. != "kaiwo.silogen.ai/finalizer")))' \
        | kubectl apply -f - || true
    fi
  fi

  # Delete resources
  kubectl delete -k test/ --force --grace-period=0 || true

  info "Waiting briefly for resource deletions"
  sleep 5

  kubectl delete namespace "${HELM_NAMESPACE}" --ignore-not-found || true
  ok "Kustomization resources undeployed"
}

# ---------- Main Execution ----------
main() {
  validate_args
  compute_image_ref

  info "Starting deployment script with:"
  info "  Build: ${BUILD}"
  info "  Install CRDs: ${INSTALL_CRDS}"
  info "  Push: ${PUSH}"
  info "  Deploy Via: ${DEPLOY_VIA}"
  info "  Command: ${COMMAND}"
  info "  Image Ref: ${IMAGE_REF}"

  # CRD installation phase
  if [[ "$INSTALL_CRDS" == true ]]; then
    install_crds
  fi

  # Build phase
  if [[ "$BUILD" == true ]]; then
    case "$PUSH" in
      "ttl.sh") build_and_upload_ttl "${IMAGE_REF}";;
      "kind")   build_and_load_kind  "${IMAGE_REF}";;
      "")
        err "Build specified but no --push target. Use --push=ttl.sh|kind"
        exit 1
        ;;
    esac
  fi

  # Deploy/Undeploy phase
  if [[ -n "$COMMAND" ]]; then
    case "$COMMAND" in
      "up")
        case "${DEPLOY_VIA}" in
          "helm")          deploy_helm_with_ref "${IMAGE_REF}";;
          "kustomization") deploy_kustomization_with_ref "${IMAGE_REF}";;
          *)
            err "Deploy requires --deploy-via=helm|kustomization"
            exit 1
            ;;
        esac
        ;;
      "down")
        case "$DEPLOY_VIA" in
          "helm")          undeploy_helm;;
          "kustomization") undeploy_kustomization;;
          "")
            err "Undeploy requires --deploy-via=helm|kustomization"
            exit 1
            ;;
        esac
        ;;
    esac
  fi

  ok "Script completed successfully"
}

# Show usage if no arguments provided
if [[ $ORIGINAL_ARGS_COUNT -eq 0 ]]; then
  usage
  exit 1
fi

# Validate that we have something meaningful to do
if [[ "$BUILD" == false && -z "$COMMAND" && "$INSTALL_CRDS" == false ]]; then
  err "Nothing to do. Specify --build, --install-crds, and/or a command (up/down)"
  usage
  exit 1
fi

main
