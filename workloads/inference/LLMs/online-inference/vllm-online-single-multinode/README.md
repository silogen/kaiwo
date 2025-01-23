# VLLM-based online inference

## Overview

Supports single-node and multi-node inference

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

Note also that currently multi-node setup (`NUM_REPLICAS` > 1) requires setting `NCCL_P2P_DISABLE=1` which involves some performance penalty in addition to the penalty introduced by network latency/bandwidth between nodes. Do not set `NCCL_P2P_DISABLE=1` for single-node setup.

To run this workload on 16 GPUs in `kaiwo` namespace, you can use the following command:	 

`kaiwo serve -p workloads/inference/LLMs/online-inference/vllm-online-single-multinode --replicas 2 --gpus-per-replica 8 --ray`

## Dependencies
- Secret `hf-token`: Hugging Face API token for model download
