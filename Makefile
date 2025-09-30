# Default tag from git
TAG ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "latest")

# Image URL to use for all building/pushing image targets
IMG ?= ghcr.io/silogen/kaiwo-operator:${TAG}

# Helm chart configuration
CHART_DIR ?= chart
CHART_VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.1.0")
APP_VERSION ?= ${TAG}

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTAINER_TOOL ?= docker

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ Development

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:maxDescLen=512,allowDangerousTypes=true,generateEmbeddedObjectMeta=true \
		paths=./apis/kaiwo/v1alpha1/... \
		paths=./apis/config/v1alpha1/... \
		paths=./apis/aim/v1alpha1/... \
		output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen
	@sed 's/^/\/\/ /' .copyright-template > .copyright-template.goheader
	$(CONTROLLER_GEN) object:headerFile=".copyright-template.goheader" \
		paths=./apis/kaiwo/v1alpha1/... \
		paths=./apis/aim/v1alpha1/... \
		paths=./apis/config/v1alpha1/...
	@rm .copyright-template.goheader
	
.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

TEST_NAME ?= "kaiwo-test"

.PHONY: test-e2e
test-e2e: manifests generate fmt vet ## Run the e2e tests. Requires an isolated environment using Kind.
	@command -v kind >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@kind get clusters | grep -q "$(TEST_NAME)" || { \
		echo "No Kind cluster named '$(TEST_NAME)' is running. Please start a Kind cluster before running the e2e tests."; \
		exit 1; \
	}
	go test ./test/e2e/ -v -ginkgo.v -timeout 30m

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/operator/main.go

.PHONY: build-cli
build-cli: ## Build CLI binary.
	go build -o bin/kaiwo cmd/cli/main.go

.PHONY: build-log
build-log: ## Build log collection binary for Chainsaw tests.
	mkdir -p builds
	go build -o builds/log pkg/cli/dev/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/operator/main.go

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./pkg/... -coverprofile cover.out

.PHONY: docker-build
docker-build: ## Build Docker image for the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push Docker image for the manager.
	$(CONTAINER_TOOL) push ${IMG}

PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name kaiwo-builder
	$(CONTAINER_TOOL) buildx use kaiwo-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm kaiwo-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml
	find config/static -type f -name "*.yaml" -exec sh -c 'echo "---" >> dist/install.yaml && cat {} >> dist/install.yaml' \;
	# Reset the image to the default
	cd config/manager && $(KUSTOMIZE) edit set image controller=ghcr.io/silogen/kaiwo-operator:v-e2e

##@ Helm

# Function to copy resources needed for Helm chart
define copy-helm-resources
	@echo "Copying RBAC resources from config/rbac..."
	@cat config/rbac/role.yaml > $(CHART_DIR)/rbac-resources.yaml
	@echo "---" >> $(CHART_DIR)/rbac-resources.yaml
	@cat config/rbac/role_binding.yaml >> $(CHART_DIR)/rbac-resources.yaml
	@echo "Copying scheduler resources from config/static/scheduler..."
	@cat config/static/scheduler/kaiwo-scheduler.yaml > $(CHART_DIR)/scheduler-resources.yaml
endef

# Function to clean up copied resources
define clean-helm-resources
	@rm -f $(CHART_DIR)/rbac-resources.yaml $(CHART_DIR)/webhook-resources.yaml $(CHART_DIR)/scheduler-resources.yaml
endef

.PHONY: helm-package
helm-package: build-installer ## Package the Helm chart
	@command -v helm >/dev/null 2>&1 || { \
		echo "Helm is not installed. Please install Helm."; \
		exit 1; \
	}
	@echo "Packaging Helm chart with version $(CHART_VERSION) and app version $(APP_VERSION)"
	$(call copy-helm-resources)
	@sed -i.bak 's/^version:.*/version: $(CHART_VERSION)/' $(CHART_DIR)/Chart.yaml
	@sed -i.bak 's/^appVersion:.*/appVersion: "$(APP_VERSION)"/' $(CHART_DIR)/Chart.yaml
	helm package $(CHART_DIR) --version=$(CHART_VERSION) --app-version=$(APP_VERSION) --destination=dist/
	@rm -f $(CHART_DIR)/Chart.yaml.bak
	$(call clean-helm-resources)

.PHONY: helm-install
helm-install: helm-package ## Install the Helm chart locally
	@echo "Installing Helm chart to kaiwo-system namespace..."
	helm upgrade --install kaiwo dist/kaiwo-operator-$(CHART_VERSION).tgz --namespace kaiwo-system --create-namespace

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the Helm chart
	helm uninstall kaiwo -n kaiwo-system

.PHONY: helm-template
helm-template: build-installer ## Generate Helm templates for inspection
	$(call copy-helm-resources)
	@sed -i.bak 's/^appVersion:.*/appVersion: "$(APP_VERSION)"/' $(CHART_DIR)/Chart.yaml
	helm template kaiwo $(CHART_DIR) --namespace kaiwo-system --output-dir dist/helm-output/
	@rm -f $(CHART_DIR)/Chart.yaml.bak
	$(call clean-helm-resources)

.PHONY: helm-push-oci
helm-push-oci: helm-package ## Push Helm chart to OCI registry
	@echo "Pushing Helm chart to OCI registry..."
	helm push dist/kaiwo-$(CHART_VERSION).tgz oci://ghcr.io/$(shell echo $(IMG) | cut -d'/' -f2 | cut -d':' -f1)

.PHONY: helm-release
helm-release: helm-package ## Package chart for release (used by CI)
	@echo "Helm chart packaged for release: dist/kaiwo-operator-$(CHART_VERSION).tgz"


##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply --server-side -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply --server-side -f -
	$(KUBECTL) rollout status deployment/kaiwo-controller-manager -n kaiwo-system --timeout=5m  

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.17.1
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v1.63.4

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: generate-crd-docs
generate-crd-docs:
	go install github.com/elastic/crd-ref-docs@latest
	crd-ref-docs --source-path apis/kaiwo/v1alpha1/ --renderer=markdown --output-path=docs/docs/reference/crds/kaiwo.silogen.ai.md --config docs/crd-ref-cocs-config.yaml
	#sed -i '1i---\nhide:\n  - navigation\n---\n' docs/docs/reference/crds/kaiwo.silogen.ai.md

	crd-ref-docs --source-path apis/config/v1alpha1/ --renderer=markdown --output-path=docs/docs/reference/crds/config.kaiwo.silogen.ai.md --config docs/crd-ref-cocs-config.yaml
	#sed -i '1i---\nhide:\n  - navigation\n---\n' docs/docs/reference/crds/config.kaiwo.silogen.ai.md

	crd-ref-docs --source-path apis/aim/v1alpha1/ --renderer=markdown --output-path=docs/docs/reference/crds/aim.silogen.ai.md --config docs/crd-ref-cocs-config.yaml

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
