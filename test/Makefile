# Makefile for E2E test execution
# Usage: make <target>
# Targets: test-kind, test-kind-deploy, test-gpu
#
# Prerequisites:
# - For test-kind: Operator must be running locally with KAIWO_LOG_FILE set
# - For test-kind-deploy: Kind cluster configured, operator will be deployed
# - For test-gpu: GPU cluster configured, operator running locally, tokens set

.PHONY: help test-kind test-kind-deploy test-gpu test-clean

# Default target
.DEFAULT_GOAL := help

# Pattern 2: Kind with local operator (requires operator running locally)
test-kind:
	@echo "Running Kind tests with local operator..."
	@echo "PREREQUISITE: Operator should be running locally with KAIWO_LOG_FILE set"
	@echo "Example: KAIWO_LOG_FILE=\$$PWD/../kaiwo.logs go run ../cmd/operator/main.go --zap-log-level=debug"
	@cd e2e && \
	KAIWO_LOG_FILE="$(PWD)/../kaiwo.logs" \
	KAIWO_TEST_CHAINSAW_CONFIG="test/chainsaw/configs/local.yaml" \
	KAIWO_TEST_KAIWOCONFIG="test/configs/kaiwoconfig/local-kind.yaml" \
	go test -timeout=30m -count=1 -v --ginkgo.v --ginkgo.label-filter="basic || kind || cli"

# Pattern 1: Kind with deployed operator (deploys operator automatically)
test-kind-deploy:
	@echo "Running Kind tests with operator deployment..."
	@cd e2e && \
	KAIWO_TEST_DEPLOY_TO_KIND=true \
	KAIWO_TEST_CHAINSAW_CONFIG="test/chainsaw/configs/local.yaml" \
	KAIWO_TEST_KAIWOCONFIG="test/configs/kaiwoconfig/local-kind.yaml" \
	go test -timeout=30m -count=1 -v --ginkgo.label-filter="kind || basic || cli" -ginkgo.v

# Pattern 3: GPU tests against remote cluster (requires operator running locally)
test-gpu:
	@if [ -z "$(KUBECONFIG)" ]; then \
		echo "Error: KUBECONFIG must be set for GPU tests"; \
		echo "Example: make test-gpu KUBECONFIG=/path/to/gpu-cluster-config"; \
		exit 1; \
	fi
	@if [ -z "$(GH_TOKEN)" ] || [ -z "$(HF_TOKEN)" ]; then \
		echo "Warning: GH_TOKEN and HF_TOKEN should be set for sensitive tests"; \
	fi
	@echo "Running GPU tests on cluster: $(KUBECONFIG)..."
	@echo "PREREQUISITE: Operator should be running locally with KAIWO_LOG_FILE set"
	@echo "Example: KAIWO_LOG_FILE=\$$PWD/../kaiwo.logs go run ../cmd/operator/main.go --zap-log-level=debug"
	@cd e2e && \
	KAIWO_LOG_FILE="$(PWD)/../kaiwo.logs" \
	KAIWO_TEST_CHAINSAW_CONFIG="test/chainsaw/configs/mi300x-dev.yaml" \
	KAIWO_TEST_KAIWOCONFIG="test/configs/kaiwoconfig/kaiwo-tw.yaml" \
	GH_TOKEN="$(GH_TOKEN)" \
	HF_TOKEN="$(HF_TOKEN)" \
	go test -timeout=30m -count=1 -v --ginkgo.v --ginkgo.label-filter="basic || gpu || cli || (gpu && amd) || (gpu && amd && partitioning)"

# Help target
help:
	@echo "Kaiwo E2E Test Execution"
	@echo "========================"
	@echo ""
	@echo "Available targets:"
	@echo "  test-kind         - Run tests with local operator (Pattern 2)"
	@echo "  test-kind-deploy  - Run tests with deployed operator (Pattern 1)" 
	@echo "  test-gpu          - Run GPU tests with local operator (Pattern 3)"
	@echo "  test-clean        - Clean test artifacts and logs"
	@echo "  help              - Show this help message"
	@echo ""
	@echo "Prerequisites:"
	@echo "  test-kind:"
	@echo "    - Kind cluster configured (KUBECONFIG pointing to Kind)"
	@echo "    - Operator running locally with KAIWO_LOG_FILE set"
	@echo "    - Example: KAIWO_LOG_FILE=\$$PWD/kaiwo.logs go run cmd/operator/main.go"
	@echo ""
	@echo "  test-kind-deploy:"
	@echo "    - Kind cluster configured"
	@echo "    - Operator will be deployed automatically"
	@echo ""
	@echo "  test-gpu:"
	@echo "    - GPU cluster configured (set KUBECONFIG)"
	@echo "    - Operator running locally with KAIWO_LOG_FILE set"
	@echo "    - GH_TOKEN and HF_TOKEN environment variables (for sensitive tests)"
	@echo "    - Example: make test-gpu KUBECONFIG=/path/to/gpu-config"
	@echo ""
	@echo "Environment variables:"
	@echo "  KUBECONFIG        - Path to kubernetes config (required for test-gpu)"
	@echo "  GH_TOKEN          - GitHub token for private repositories"
	@echo "  HF_TOKEN          - Hugging Face token for gated models"
	@echo "  KAIWO_LOG_FILE    - Set to same path in both operator and tests"

# Clean test artifacts
test-clean:
	@echo "Cleaning test artifacts..."
	@rm -f ../kaiwo.logs
	@rm -rf /tmp/kaiwo-test-reports/*
	@echo "Cleaned: kaiwo.logs and test reports"
