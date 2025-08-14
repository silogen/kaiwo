KIND_SELECTOR_BASE = "suite=base,env in (all,kind)"
KIND_DIRS = tests/base tests/kind-only
GPU_SELECTOR_BASE  = "suite=gpu,env in (all,gpu)"
GPU_DIRS = tests/base tests/gpu-only

# Build CLI
# Values file

.PHONY: test-kind-dev
test-kind-dev: ## Operator runs in IDE, against Kind
	@chainsaw test \
	  --config ./.chainsaw.yaml \
	  --selector $(KIND_SELECTOR_BASE) \
	  --test-dir $(KIND_DIRS) \
	  --values ./values.kind.dev.yaml

.PHONY: test-kind-deployed
test-kind-deployed: kind-operator-deploy
	@chainsaw test \
	  --config ./.chainsaw.yaml \
	  --selector $(KIND_SELECTOR_BASE) \
	  --test-dir $(KIND_DIRS) \
	  --values ./values.kind.deployed.yaml
	@$(MAKE) kind-operator-undeploy

.PHONY: kind-operator-deploy
kind-operator-deploy:
	@./scripts/kind_operator.sh up   # build, kind load, kubectl apply, wait rollout

.PHONY: kind-operator-undeploy
kind-operator-undeploy:
	@./scripts/kind_operator.sh down # kubectl delete, etc.

.PHONY: test-gpu
test-gpu: ## Remote GPU cluster; operator typically local
	@chainsaw test \
	  --config ./chainsaw/configs/amd-gpu-cluster.yaml \
	  --test-dir chainsaw/tests/amd-gpu/general

.PHONY: test-gpu-partition
test-gpu-partition: ## Run GPU partitioning suite in isolation, with setup/teardown
	# 1) setup
	@chainsaw test \
	  --config ./.chainsaw.yaml \
	  --kube-context gpu-cluster \
	  --selector "suite=gpu-partition,phase=setup" \
	  --parallel 1 \
	  --test-dir tests/gpu-partition
	# 2) tests
	@chainsaw test \
	  --config ./.chainsaw.yaml \
	  --kube-context gpu-cluster \
	  --selector "suite=gpu-partition,phase=tests" \
	  --parallel 1 \
	  --test-dir tests/gpu-partition
	# 3) teardown (always run)
	-@chainsaw test \
	  --config ./.chainsaw.yaml \
	  --kube-context gpu-cluster \
	  --selector "suite=gpu-partition,phase=teardown" \
	  --parallel 1 \
	  --test-dir tests/gpu-partition
