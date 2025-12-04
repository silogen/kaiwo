# Dev base image with pre-populated Go module cache
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
# This layer will be cached and reused by tilt.Dockerfile
RUN go mod download
