# DeepSpeed Zero3: Single and Multi-Node finetuning


## Pre-requisites

Make sure you have S3 endpoint and credentials set up in your environment. If you don't, you can deploy a minio instance with `workloads/dev-storage/s3-deployment.yaml`

## Overview

This workload acts as a finetuning overlay over pre-training workload `workloads/training/LLMs/full-parameter-pretraining/full-param-zero3-single-multinode`.

Keep in mind the following:
- LORA finetuning: if you use a different model architecture, you may need to adjust LORA configuration and `target_modules` in particular.
- Supports single-node and multi-node scenarios
- DeepSpeed Zero stage 3 partitions LLM parameters, gradients, and optimizer states across multiple GPUs
- set `num_devices` to total number of GPUs.

Run example with:

`kubectl apply -f kaiwojob-llama-3.1-8b-instruct.yaml`

Or if you're using kaiwo-cli which can also set user email and clusterQueue to the correct one, run

`kaiwo submit -f kaiwojob-llama-3.1-8b-instruct.yaml`

## Dependencies
- hf-token: Hugging Face API token for model download
- s3-secret: S3 secret for model upload or GCS secret for model upload - refactor `env` file and code accordingly