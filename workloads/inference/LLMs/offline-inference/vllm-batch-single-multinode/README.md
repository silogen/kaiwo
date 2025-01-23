# VLLM-based batch inference

## Gated models

This workload example with Llama3 currently only supports single-node inference (one model instance per node), but the workload can be scaled to multiple instances by increasing `num_instances`

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

To run this workload on 16 GPUs in `kaiwo` namespace, set `NUM_GPUS_PER_REPLICA` to `8` and `NUM_REPLICAS` to `2` in the `env` file. 

You can then use the following command:

`kaiwo submit -p workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode/ --replicas 2 --gpus-per-replica 8 --ray`

### Dependencies
- Secret `hf-token`: Hugging Face API token for model download

### Models that do not require HF_TOKEN

Note! Not all models work with `NUM_REPLICAS` > 1

Replace `env` file with the following contents to use TinyLlama

```
envVars:
  - name: MODEL_ID
    value: "TinyLlama/TinyLlama-1.1B-Chat-v1.0"

```

Run with:

`kaiwo submit -p workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode/ --ray --replicas 1 --gpus-per-replica 8`