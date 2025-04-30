# vLLM-based batch inference

## Gated models

### Dependencies

- Secret `hf-token`: Hugging Face API token for model download

### Overview

This workload currently only supports single-node inference (one model instance per node), but the workload can be scaled up to multiple replicas (on multiple nodes) by increasing `replicas` and using `gpusPerReplica` instead of `gpus` field. However, note that not all models work with `NUM_REPLICAS` > 1. We have tested that this works with llama-3.1-8b-instruct. 

When using `gpus` field,  user is letting Kaiwo to automatically set env variables `NUM_GPUS_PER_REPLICA` to `8` and `NUM_REPLICAS` to `2`. Kaiwo is able to set these by inspecting the number of requested GPUs (`gpus` field) and the number of GPUs available per node. See `main.py` for more details how the training script uses these env variables. These env variables can also be controlled by the user when `replicas` and `gpusPerReplica` fields are used instead of `gpus` field.

Run example with:

`kubectl apply -f kaiwojob-llama-3.1-8b-instruct.yaml`

Or if you're using kaiwo-cli which can also set user email and clusterQueue to the correct one, run

`kaiwo submit -f kaiwojob-llama-3.1-8b-instruct.yaml`

## Models that do not require HF_TOKEN

For example, to run inference with Tiny Llama, run 

`kubectl apply -f kaiwojob-tiny-llama.yaml`

Or if you're using kaiwo-cli which can also set user email and clusterQueue to the correct one, run

`kaiwo submit -f kaiwojob-tiny-llama.yaml`