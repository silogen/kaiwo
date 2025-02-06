# VLLM-based online inference

## Overview

Supports single-node and multi-node inference

Note! this workload expects existing secrets. Have a look at `env` file for the expected secrets. 

Note also that currently multi-node setup (`NUM_REPLICAS` > 1) requires setting `NCCL_P2P_DISABLE=1` which involves some performance penalty in addition to the penalty introduced by network latency/bandwidth between nodes. Do not set `NCCL_P2P_DISABLE=1` for single-node setup.
 
To run this workload on 16 GPUs in `kaiwo` namespace, you can let Kaiwo automatically set env variables `NUM_GPUS_PER_REPLICA` to `8` and `NUM_REPLICAS` to `2`. Kaiwo is able to set these by inspecting the number of requested GPUs (`-g`) and the number of GPUs available per node. See `__init__.py` for more details how the training script uses these env variables.

Run with:

`kaiwo serve -p workloads/inference/LLMs/online-inference/vllm-online-single-multinode -g 16 --ray --storage=100Gi,nameofyourstorageclass`

Or set these variables yourself with the following command:

`kaiwo serve -p workloads/inference/LLMs/online-inference/vllm-online-single-multinode --replicas 2 --gpus-per-replica 8 --ray --storage=100Gi,nameofyourstorageclass`

## Dependencies
- Secret `hf-token`: Hugging Face API token for model download


## Testing the workload

RayService typically starts a service called `yourworkloadname-serve-svc` at port 8000. For production, you should add an ingress to the service. For testing, you can use `kubectl port-forward` to access the service.

`kubectl port-forward svc/yourworkloadname-multi-serve-svc 8000:8000 -n yournamespace`

Then you can send a request to the service with curl:

```

curl http://localhost:8000/v1/chat/completions -H "Content-Type: application/json" -d '{
      "model": "meta-llama/Llama-3.1-8B-Instruct",
      "messages": [
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Provide a brief sentence describing the Ray open-source project."}
      ],
      "temperature": 0.7
    }'
```