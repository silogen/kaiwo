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

# Use ttl.sh as default registry for development
default_registry('ttl.sh/kaiwo-operator-dev')

# The image as referenced in your kustomize config
IMAGE = 'ghcr.io/silogen/kaiwo-operator:v-e2e'

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
# K8s resources
# ---------------------------------------------------------------------------

# Apply CRDs with server-side apply (they're too large for client-side)
local_resource(
    'crds',
    cmd='kustomize build config/crd | kubectl apply --server-side --force-conflicts -f -',
    deps=['config/crd'],
    labels=['setup'],
)

# Generate YAML and apply manifests
local_resource(
    'config',
    cmd='kustomize build config/tilt | kubectl apply -f -',
    deps=['config/tilt'],
    labels=['setup'],
    resource_deps=['crds'],  # Apply CRDs first
)

# Load YAML for Tilt to track resources
yaml = kustomize('config/tilt')
k8s_yaml(yaml)

# Configure the controller resource
k8s_resource(
    workload='kaiwo-controller-manager',
    new_name='controller',
    labels=['controller'],
    port_forwards=[
        '8080:8080',  # metrics
        '9443:9443',  # webhook
    ],
    extra_pod_selectors=[{'control-plane': 'controller-manager'}],
)
