# Tilt Development Setup

Fast local development with live code updates into Kubernetes.

## Quick Start

1. **Build the dev base image** (first time, or when dependencies change):
   ```bash
   docker build -f hack/tilt/dev.Dockerfile -t kaiwo-dev-base:latest .
   ```

2. **Start Tilt**:
   ```bash
   tilt up
   ```

3. **Edit code** - changes sync automatically and the controller restarts in seconds.

## When to Rebuild the Base Image

Rebuild whenever you update Go dependencies:
- Changed `go.mod` or `go.sum`
- Added/removed packages

The base image is local-only and pre-downloads dependencies + warms the build cache for fast startup.

## Components

- **dev.Dockerfile** - Base image with Go toolchain, dependencies, and build cache
- **tilt.Dockerfile** - Thin layer that copies source code for live updates
- **dev-entrypoint.sh** - Builds and runs the controller on container start
- **kustomization.yaml** - Dev-specific K8s manifests (relaxed security, probes, resources)
- **patches/** - Kustomize patches for dev mode (startup probes, entrypoint, etc.)
- **resources/** - Additional K8s resources needed for dev

## How Live Updates Work

1. You edit source code
2. Tilt syncs changed files to the running container
3. Go rebuilds the binary (fast, using cached dependencies)
4. Process restarts with new code
5. Total time: ~2-5 seconds + leader election / manager startup
