# DeepSpeed Zero3: Single and Multi-Node full-parameter training

## Overview

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. If you find both S3 and GCS secrets, you can choose to use either one. Remember to refactor your code accordingly.

- full-parameter-pretraining
- Supports single-node and multi-node scenarios
- DeepSpeed ZeRO stage 3 partitions LLM parameters, gradients, and optimizer states across multiple GPUs
- set `num_devices` to total number of GPUs.

To run this workload on 16 GPUs in `kaiwo` namespace, set `num_devices` in `entrypoint` to `16` and use the following command:

`kaiwo submit -p workloads/training/LLMs/full-parameter-pretraining/full-param-zero3-single-multinode -g 16 --ray --storage=100Gi,nameofyourstorageclass`

## Dependencies
- hf-token: Hugging Face API token for model download
- s3-secret: S3 secret for model upload or GCS secret for model upload - refactor `env` file and code accordingly