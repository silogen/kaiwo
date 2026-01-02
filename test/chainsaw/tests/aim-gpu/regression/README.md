# Dynamic Regression Tests

This directory contains a dynamic regression test system for AIMService models.

## How It Works

1. Define models in `models.csv`
2. Run `./generate-tests.sh` to automatically generate Chainsaw tests
3. Each model gets its own test folder with generated manifests
4. The script runs all tests with the provided secrets

## Setup

### 1. Create values.yaml

Copy the example and fill in your credentials:

```bash
cp values.yaml.example values.yaml
# Edit values.yaml with your actual credentials
```

The `values.yaml` file is gitignored and should contain:
- `imagePullSecret`: Docker config JSON for private registries
- `hfToken`: HuggingFace token for gated models

### 2. Define Models

Edit `models.csv` to add or remove models:

```csv
skip,registry,name,version,unoptimized,hfToken,private,timeout,concurrent
no,ghcr.io/silogen,aim-openai-gpt-oss-20b,0.9.0-rc3,yes,no,yes,10m,yes
yes,amdenterpriseai,aim-meta-llama-llama-3.2-1b,0.8.4,no,yes,no,15m,no
```

**Columns:**
- `skip` (default: no) - Set to `yes` to skip this model without removing from list
- `registry` (required) - Docker registry (e.g., ghcr.io/silogen, amdenterpriseai)
- `name` (required) - Image name (e.g., aim-openai-gpt-oss-20b)
- `version` (required) - Image tag/version (e.g., 0.9.0-rc3, 0.8.4)
- `unoptimized` (default: no) - Set to `yes` to add `allowUnoptimized: true`
- `hfToken` (default: no) - Set to `yes` to add HF_TOKEN env var
- `private` (default: no) - Set to `yes` to add imagePullSecrets
- `timeout` (default: 10m) - Timeout for readiness check (e.g., 10m, 15m, 1h)
- `concurrent` (default: yes) - Set to `no` to run test sequentially (for GPU-heavy models)

**Note:** Tests fail immediately if service enters Failed/Degraded state.

### 3. Preview (Optional)

See what tests would be generated without creating files:

```bash
./generate-tests-dry-run.sh
```

### 4. Run Tests

Execute the script to generate and run all tests:

```bash
./generate-tests.sh
```

This will:
1. Clean up old `aim-*` test folders
2. Generate new folders for each model in `models.csv`
3. Create `service.yaml` and `chainsaw-test.yaml` in each folder
4. Run `chainsaw test` with your values.yaml

**Notes:**
- Tests use a fail-fast approach with two stages:
  - **Error check (1 minute)**: Uses Chainsaw's `error` operation to detect Degraded/Failed states immediately after service creation
  - **Readiness assertion**: Waits for Running status and RuntimeReady condition
- If a service enters Failed or Degraded state during the error check, the test stops immediately
- Chainsaw runs up to 4 tests in parallel by default. Set `concurrent: no` on models that require many GPUs to prevent parallel execution.

**Generate without running tests:**

```bash
./generate-tests.sh --skip-tests
```

## Generated Structure

For each model, the script creates:

```
aim-{sanitized-image-name}/
├── service.yaml          # AIMService manifest
└── chainsaw-test.yaml    # Chainsaw test definition
```

Secrets are shared across all tests via separate secret files in the root regression folder:
- `secret-imagepull.yaml` - Image pull secret (used when `isPrivate: true`)
- `secret-hftoken.yaml` - HuggingFace token (used when `requiresHfToken: true`)

## Templates

Test files are generated from templates in the `templates/` directory:

**Service templates:**
- `service-base.yaml` - Base AIMService structure
- `service-unoptimized.yaml` - allowUnoptimized section
- `service-imagepull.yaml` - imagePullSecrets section
- `service-hftoken.yaml` - HF_TOKEN env var section

**Test templates:**
- `chainsaw-test-header.yaml` - Test metadata and description
- `chainsaw-test-service-with-failfast.yaml` - Service creation and readiness check with fail-fast on Degraded/Failed states

## Folder Naming

Image names are sanitized to create valid RFC 1123 compliant folder names:
- Converted to lowercase
- `:` → `-`
- `/` → `-`
- `.` → `-`

Example: `ghcr.io/silogen/aim-model:0.9.0` → `aim-ghcr-io-silogen-aim-model-0-9-0`
