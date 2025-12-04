#!/usr/bin/env bash
set -euo pipefail

cd /workspace

echo "Dev entrypoint: building manager..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager ./cmd/main.go

echo "Starting manager..."
exec ./manager
