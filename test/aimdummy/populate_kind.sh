#!/usr/bin/env bash
cd "$(dirname "$0")"

docker build -t aimdummy:dev-latest .
kind load docker-image aimdummy:dev-latest --name kaiwo-test
