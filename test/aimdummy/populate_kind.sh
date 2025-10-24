#!/usr/bin/env bash
cd "$(dirname "$0")"

docker build -t aimfaker:dev-latest .
kind load docker-image aimfaker:dev-latest --name kaiwo-test
