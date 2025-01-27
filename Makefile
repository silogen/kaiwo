# Variables
CLI_BINARY = kaiwo-cli
OPERATOR_BINARY = kaiwo-operator
REGISTRY = your-docker-registry
IMAGE_NAME = $(REGISTRY)/kaiwo-operator
VERSION = v0.1.0

# Default target
.PHONY: all
all: build-cli build-operator

# Build the CLI
.PHONY: build-cli
build-cli:
	go build -o bin/$(CLI_BINARY) ./main.go

# Build the Operator
.PHONY: build-operator
build-operator:
	go build -o bin/$(OPERATOR_BINARY) ./cmd/operator/main.go

# Docker build for operator
.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(VERSION) -f Dockerfile .

# Push operator image to the registry
.PHONY: docker-push
docker-push:
	docker push $(IMAGE_NAME):$(VERSION)

# Apply CRDs to the cluster
.PHONY: install-crds
install-crds:
	kubectl apply -f config/crd/bases/

# Apply RBAC configurations
.PHONY: install-rbac
install-rbac:
	kubectl apply -f config/rbac/

# Deploy operator
.PHONY: deploy-operator
deploy-operator:
	kubectl apply -f config/deployment/operator-deployment.yaml

# Clean up CRDs and operator
.PHONY: clean
clean:
	kubectl delete -f config/crd/bases/
	kubectl delete -f config/deployment/operator-deployment.yaml

# Generate code
.PHONY: generate
generate:
	controller-gen object paths="./pkg/api/v1/..."

# Generate CRD manifests
.PHONY: manifests
manifests:
	controller-gen crd paths="./pkg/api/v1/..." output:crd:dir="./config/crd/bases"
