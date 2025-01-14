# VLLM-based online inference

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

- Supports single-node and multi-node inference
- for best results, set `TENSOR_PARALLELISM` to number of GPUs per node and `PIPELINE_PARALLELISM` to number of nodes

