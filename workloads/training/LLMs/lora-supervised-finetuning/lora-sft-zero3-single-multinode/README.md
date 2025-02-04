# DeepSpeed Zero3: Single and Multi-Node finetuning

## Overview

This workload acts as a finetuning overlay over pre-training workload `workloads/training/LLMs/full-parameter-pretraining/full-param-zero3-single-multinode`.

Keep in mind the following:
- LORA finetuning: if you use a different model architecture, you may need to adjust LORA configuration and `target_modules` in particular.
- Supports single-node and multi-node scenarios
- DeepSpeed Zero stage 3 partitions LLM parameters, gradients, and optimizer states across multiple GPUs
- set `num_devices` to total number of GPUs.

To run this workload on 16 GPUs in `kaiwo` namespace, set `num_devices` in `entrypoint` to `16` and use the following command:

`kaiwo submit -p workloads/training/LLMs/full-parameter-pretraining/full-param-zero3-single-multinode --overlay-path workloads/training/LLMs/lora-supervised-finetuning/lora-sft-zero3-single-multinode -g 16 --ray --storage=100Gi,nameofyourstorageclass`

## Dependencies
- hf-token: Hugging Face API token for model download
- s3-secret: S3 secret for model upload or GCS secret for model upload - refactor `env` file and code accordingly