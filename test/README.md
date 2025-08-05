# Kaiwo E2E Testing Guide

This directory contains the end-to-end testing infrastructure for Kaiwo. The testing system is built around **Ginkgo** for Go test orchestration and **Chainsaw** for Kubernetes-native testing.

## Quick Start

You can run different test profiles from the `Makefile` in this folder. The makefile profiles run with different sets of Ginkgo labels, which are used to filter the tests to be run. Before running tests, please note the following points:

* When running the operator locally, in order to benefit from the log aggregation that the testing framework does, you should run the operator with the environmental variable `KAIWO_LOG_FILE` set to the same path as the makefile expects. In most cases this will be `<repository_root>/kaiwo.logs`. The operator will write its logs to this file, from which the testing framework can extract them.
* If you are running the AMD GPU tests, these include partitioning tests. Rather than hardcoding the partitioned node names and GPU vRAM amounts, you must provide these values yourself via a YAML file, referenced by the environmental variable `KAIWO_TEST_BASE_VALUES_FILE`.

### `make test-kind` (Kind cluster, operator already running)

This profile will run the tests against a Kind cluster deployed with the `setup/kind.sh` script. It expects that an operator is running, either locally or manually deployed inside the cluster. It includes the following tests:

* Basic non-GPU tests
* Kind-environment specific tests (against mocked NVIDIA GPUs)
* CLI tests

This profile is useful to run while developing locally or when you want to debug tests.

### `make test-kind-deploy` (Kind cluster, deploy operator to cluster)

This profile will run the tests against a Kind cluster deployed with the `setup/kind.sh` script. It includes the following tests:

* Basic non-GPU tests
* Kind-environment specific tests (against mocked NVIDIA GPUs)
* CLI tests

### `make test-amd-gpu` (AMD GPU cluster, operator running locally)

This profile will run the tests against an AMD GPU cluster deployed with the `setup/gpu.sh` script. It expects that an operator is running, either locally or manually deployed inside the cluster. It includes the following tests:

* Basic non-GPU tests
* AMD GPU specific tests, including partitioning tests
* CLI tests

As the tests include partitioning tests, the tests must be run against a cluster with GPUs that support partitioning, such as the MI300X.

## Test discovery

Each test profile (kind, basic, cli, gpu, etc.) corresponds to a folder under `chainsaw/tests`. The test framework first discovers the available Chainsaw tests, and builds the Ginkgo test structure. After this, the Chainsaw tests are executed, and the success or failure is reported in the test output.
