# Test directories for different environments
# Automated AIM tests are disabled for now
KIND_DIRS = chainsaw/tests/aim chainsaw/tests/aim-gpu # chainsaw/tests/standard

# Values file configuration (can be overridden via env var)
VALUES_FILE ?= values/base.yaml

# Chainsaw configuration (can be overridden for CI vs local)
CHAINSAW_CONFIG ?= chainsaw/configs/local.yaml

# Default environment variables for testing
GH_TOKEN_BASE64 ?=
HF_TOKEN_BASE64 ?=
AMD_GPU_PARTITIONING_NODE ?=
AMD_GPU_UNPARTITIONED_VRAM ?=
AMD_GPU_PARTITIONED_VRAM ?=
OPERATOR_NAMESPACE ?= kaiwo-system
TEST_TIMEOUT ?= 300s

export GH_TOKEN_BASE64 HF_TOKEN_BASE64 AMD_GPU_PARTITIONING_NODE AMD_GPU_UNPARTITIONED_VRAM AMD_GPU_PARTITIONED_VRAM OPERATOR_NAMESPACE TEST_TIMEOUT

.PHONY: test
test: test-kind ## Default target - run tests against Kind cluster

.PHONY: substitute-values
substitute-values: ## Create runtime values file with environment variable substitution
	@if [ -z "$(VALUES_FILE)" ]; then \
		echo "Error: VALUES_FILE is not set"; \
		exit 1; \
	fi
	@sed -e "s/\$${GH_TOKEN_BASE64}/$(GH_TOKEN_BASE64)/g" \
	     -e "s/\$${HF_TOKEN_BASE64}/$(HF_TOKEN_BASE64)/g" \
	     -e "s/\$${AMD_GPU_PARTITIONING_NODE}/$(AMD_GPU_PARTITIONING_NODE)/g" \
	     -e "s/\$${AMD_GPU_UNPARTITIONED_VRAM}/$(AMD_GPU_UNPARTITIONED_VRAM)/g" \
	     -e "s/\$${AMD_GPU_PARTITIONED_VRAM}/$(AMD_GPU_PARTITIONED_VRAM)/g" \
	     -e "s/\$${OPERATOR_NAMESPACE}/$(OPERATOR_NAMESPACE)/g" \
	     -e "s/\$${TEST_TIMEOUT}/$(TEST_TIMEOUT)/g" \
	     $(VALUES_FILE) > values/runtime.yaml


.PHONY: test-kind
test-kind: substitute-values ## Run tests against Kind cluster
	kubectl apply -f configs/kaiwoconfig/local-kind.yaml
	sleep 3
	@PATH="$(shell pwd)/../builds:$$PATH" chainsaw test \
	  --config ./$(CHAINSAW_CONFIG) \
	  --test-dir $(KIND_DIRS) \
	  --values values/runtime.yaml
