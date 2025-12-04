#!/usr/bin/env bash
set -euo pipefail

cd /workspace

# Set Go cache directories to writable locations for non-root user
export GOCACHE=/workspace/.cache/go-build
export GOMODCACHE=/workspace/.cache/go-mod

echo "Dev entrypoint: building manager..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager ./cmd/operator/main.go

echo "Starting manager..."
exec ./manager "$@"
