# Dev base image with pre-populated Go module and build cache
FROM docker.io/golang:1.24

# Create workspace and cache directories
RUN mkdir -p /workspace /workspace/.cache/go-build /workspace/.cache/go-mod && \
    chown -R 65532:65532 /workspace

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Switch to non-root user and set cache locations
USER 65532:65532
ENV GOCACHE=/workspace/.cache/go-build
ENV GOMODCACHE=/workspace/.cache/go-mod

# Pre-download all dependencies to populate the module cache
RUN go mod download

# Copy source code to warm up the build cache
COPY --chown=65532:65532 cmd ./cmd
COPY --chown=65532:65532 apis ./apis
COPY --chown=65532:65532 pkg ./pkg
COPY --chown=65532:65532 internal ./internal

# Warm up build cache by doing a full build
# This makes the first dev build much faster
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/manager ./cmd/operator/main.go

# Remove the source and binary - tilt.Dockerfile will copy fresh source
# But keep the populated caches!
RUN rm -rf /tmp/manager ./cmd ./apis ./pkg ./internal
