# VLLM-based batch inference

## Overview

This workload currently only supports single-node inference (one model instance per node), but the workload can be scaled to multiple instances by increasing `num_instances`

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

To run this workload on 16 GPUs in `kaiwo` namespace, set `TENSOR_PARALLELISM` to `8` and `NUM_INSTANCES` to `2` in the `env` file. 

You can then use the following command:

`kaiwo submit -p workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode/ -g 16 --ray`

## Dependencies
- Secret `hf-token`: Hugging Face API token for model download
