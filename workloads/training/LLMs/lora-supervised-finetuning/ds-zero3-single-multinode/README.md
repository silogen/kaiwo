# DeepSpeed Zero3: Single and Multi-Node finetuning

- LORA finetuning: if you use a different model architecture, you may need to adjust LORA configuration and `target_modules` in particular.
- Supports single-node and multi-node scenarios
- DeepSpeed ZeRO partitions LLM parameters, gradients, and optimizer states across multiple GPUs
- set `num_devices` to total number of GPUs.

