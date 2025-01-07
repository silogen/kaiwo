# AIWO - AI Workload Orchestrator

AI Workload Orchestrator for Kubernetes


## Description

#TODO

- describe components and purpose of the AI Workload Orchestrator

## Installation

#TODO

- describe CLI tool installation
- refer user to install directory for installation of AIWO components

## Usage

#TODO

- describe how to use the AI Workload Orchestrator
- Describe commands and flags (Number of GPUs and path are required. Path must include entrypoint file at minimum)
- describe example multi-node workloads
  - Distributed pretraining/finetuning (SFT), latter with LORA
  - Distributed inference with VLLM (tensor/pipeline parallel) online/offline
  - Distributed DPO (TBA)
- Multi-node workloads become single-node by adjustting GPU requests (notice also changes to VLLM pipeline parallel) 
- recommendation to use separate cluster for online inference workloads (due to resource contention)
- Note about typical secrets and environment variables (s3 keys, HF TOKEN, etc)
- Note about how secrets are managed (ExternalSecrets, etc)