# VLLM-based batch inference

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

- Currently only supports single-node inference (one model instance per node), but can be scaled to multiple instances by increasing `num_instances`
