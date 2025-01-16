# VLLM-based online inference

Supports single-node and multi-node inference

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

For best performance, set `TENSOR_PARALLELISM` to number of GPUs per node and `PIPELINE_PARALLELISM` to number of nodes in `env` file. The product of these two should be your total GPU request. In the following example, `TENSOR_PARALLELISM` should be set to `8` and `PIPELINE_PARALLELISM` to `2`.

Note also that currently multi-node setup (`PIPELINE_PARALLELISM` > 1) requires setting `NCCL_P2P_DISABLE=1` which involves some performance penalty in addition to the penalty introduced by network latency/bandwidth between nodes.

To run this workload on 16 GPUs in `kaiwo` namespace, you can use the following command:	 

`kaiwo submit -p workloads/inference/LLMs/online-inference/vllm-single-multinode -g 16 -t rayservice`

