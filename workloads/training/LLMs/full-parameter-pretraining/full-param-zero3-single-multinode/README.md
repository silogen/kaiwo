# DeepSpeed Zero3: Single and Multi-Node full-parameter training

## Dependencies
- hf-token: Hugging Face API token for model download
- s3-secret: S3 secret for model upload or GCS secret for model upload - refactor `env` section in manifest and code accordingly

## Overview

- full-parameter-pretraining
- Supports single-node and multi-node scenarios
- DeepSpeed ZeRO stage 3 partitions LLM parameters, gradients, and optimizer states across multiple GPUs


Run example with:

`kubectl apply -f kaiwojob-llama-3.1-8b-instruct.yaml`

Or if you're using kaiwo-cli which can also set user email and clusterQueue to the correct one, run

`kaiwo submit -f kaiwojob-llama-3.1-8b-instruct.yaml`