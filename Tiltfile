# ---------------------------------------------------------------------------
# Cluster safety / settings
# ---------------------------------------------------------------------------

allow_k8s_contexts([
    'vcluster_alex-kaiwo-dev_alex-kaiwo-dev_kaiwo-tw',
    'vcluster_alex-test_alex-test_app-dev',
])

update_settings(max_parallel_updates=3, k8s_upsert_timeout_secs=60)

# ---------------------------------------------------------------------------
# Image configuration
# ---------------------------------------------------------------------------

# Dev image built by Tilt and pushed to ttl.sh
# Example:
#   DEV_IMG=ttl.sh/kaiwo-operator-dev tilt up
IMAGE = os.getenv('DEV_IMG', 'ttl.sh/kaiwo-operator-dev')

# The image hard-coded in your kustomize config that we want to override
ORIGINAL_IMAGE = 'ghcr.io/silogen/kaiwo-operator:v-e2e'

# ---------------------------------------------------------------------------
# Thin docker build on top of your base image
# ---------------------------------------------------------------------------

docker_build(
    IMAGE,
    context='.',
    dockerfile='hack/tilt.Dockerfile',  # uses your base image + copies source + dev entrypoint
    live_update=[
        # If deps or Dockerfile change, fall back to full rebuild + redeploy
        fall_back_on(['go.mod', 'go.sum', 'Dockerfile']),

        # Sync source code into the running container
        sync('./cmd',      '/workspace/cmd'),
        sync('./internal', '/workspace/internal'),
        sync('./pkg', '/workspace/pkg'),
        sync('./apis',      '/workspace/apis'),

        # Rebuild the manager binary in-place on source changes
        run(
            'cd /workspace && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager ./cmd/main.go',
            trigger=['./cmd', './internal', './api'],
        ),
    ],
)

# ---------------------------------------------------------------------------
# K8s resources – server-side apply via k8s_custom_deploy
# ---------------------------------------------------------------------------

apply_cmd = """
bash -lc '
  set -euo pipefail
  kustomize build config/tilt \\
    | sed "s#{ORIGINAL_IMAGE}#{IMAGE}#g" \\
    | tee >(kubectl apply --server-side --force-conflicts -f -)
'
""".format(ORIGINAL_IMAGE=ORIGINAL_IMAGE, IMAGE=IMAGE)

delete_cmd = """
bash -lc '
  set -euo pipefail
  kustomize build config/tilt \\
    | sed "s#{ORIGINAL_IMAGE}#{IMAGE}#g" \\
    | kubectl delete -f - || true
'
""".format(ORIGINAL_IMAGE=ORIGINAL_IMAGE, IMAGE=IMAGE)

k8s_custom_deploy(
    'kaiwo-controller',          # Tilt resource name
    apply_cmd=apply_cmd,
    delete_cmd=delete_cmd,
    deps=['config/tilt'],        # rerun when manifests change
    image_selector=IMAGE,        # tie this deploy to IMAGE / Live Update
)

k8s_resource(
    'kaiwo-controller',
    port_forwards=[
        '8080:8080',  # metrics
        '9443:9443',  # webhook
    ],
    extra_pod_selectors=[{'control-plane': 'controller-manager'}],
)
