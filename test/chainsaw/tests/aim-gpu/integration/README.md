# AIM GPU Integration Tests

These tests require secrets to be configured before running.

## Required Secrets

### 1. GHCR Pull Secret (`ghcr-pull-secret`)

For pulling images from `ghcr.io/silogen`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-pull-secret
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-docker-config>
```

**Namespace placement:**
- **Most tests**: Create in the **test namespace** (chainsaw creates a namespace per test)
- **`cluster-model-*` tests**: Create in **`kaiwo-system`** namespace (cluster-scoped models use operator namespace)

### 2. HuggingFace Token (`hf-token-secret`)

For downloading gated models from HuggingFace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hf-token-secret
type: Opaque
data:
  token: <base64-encoded-hf-token>
```

## Quick Setup

```bash
# Create GHCR pull secret (for namespace-scoped tests)
kubectl create secret docker-registry ghcr-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-pat> \
  -n <test-namespace>

# Create GHCR pull secret (for cluster-model tests)
kubectl create secret docker-registry ghcr-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-pat> \
  -n kaiwo-system

# Create HF token secret
kubectl create secret generic hf-token-secret \
  --from-literal=token=<your-hf-token> \
  -n <test-namespace>
```

## Test-specific Notes

| Test Folder | GHCR Secret Namespace | HF Token Required |
|-------------|----------------------|-------------------|
| `cache-from-service-public` | test namespace | Yes |
| `cache-precreated-private` | `kaiwo-system` | Yes |
| `cluster-model-public-image` | `kaiwo-system` | Yes |
| `service-image-private-hf` | test namespace | Yes |

## Using secrets.yaml

Each test folder contains a `secrets.yaml` file that is automatically applied by the chainsaw test.
The `secrets.yaml` files are gitignored to prevent credential leaks.

**Before running tests**, populate the secrets files with your credentials

