# DeepSpeed Zero3: Single and Multi-Node Training

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. If you find both S3 and GCS secrets, you can choose to use either one. Remember to refactor your code accordingly.

- full-parameter-pretraining
- Supports single-node and multi-node scenarios
- DeepSpeed ZeRO partitions LLM parameters, gradients, and optimizer states across multiple GPUs
- set `num_devices` to total number of GPUs.

