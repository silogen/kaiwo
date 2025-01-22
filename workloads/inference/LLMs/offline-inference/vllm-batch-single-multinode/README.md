# VLLM-based batch inference

## Gated models

This workload example with Llama3 currently only supports single-node inference (one model instance per node), but the workload can be scaled to multiple instances by increasing `num_instances`

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

To run this workload on 16 GPUs in `kaiwo` namespace, set `TENSOR_PARALLELISM` to `8` and `NUM_INSTANCES` to `2` in the `env` file. 

You can then use the following command:

`kaiwo submit -p workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode/ -g 16 --ray`

### Dependencies
- Secret `hf-token`: Hugging Face API token for model download

### Models that do not require HF_TOKEN

Note! Not all models work with `num_instances` > 1

Replace `env` file with the following contents to use TinyLlama

```
envVars:
  - name: MODEL_ID
    value: "TinyLlama/TinyLlama-1.1B-Chat-v1.0"
  - name: TENSOR_PARALLELISM
    value: "2"
  - name: NUM_INSTANCES
    value: "1"

```

Run with:

`kaiwo submit -p workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode/ -g 2 --ray`