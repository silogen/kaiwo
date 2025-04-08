# Quickstart Guides

This section provides quickstart guides for deploying AI workloads with Kaiwo. It covers the following topics:

- [CLI quickstart](#installation)
- [Training](#training)
- [Distributed Training](#distributed-training)
- [Inference](#inference)
- [Distributed Inference](#distributed-inference)

## CLI Quickstart

This quickstart assumes Kaiwo Operator has been installed. If you suspect this is not the case, see [here](../admin/installation.md).

Workloads can be submitted to Kaiwo Operator via Kaiwo CLI or Kubectl. Kaiwo CLI is a command-line interface that simplifies the process of submitting and managing workloads on Kubernetes. It provides a user-friendly interface for interacting with the Kaiwo Operator, making it easier to deploy and manage AI workloads.

This is the TL;DR version of the installation instructions. For more details, see the [installation guide](./cli.md).

Make sure you have a working KUBECONFIG file. Then, install Kaiwo CLI by running the following commands in your terminal:

```bash
export KAIWO_VERSION=v.0.1 && \
wget https://github.com/silogen/kaiwo/releases/download/$KAIWO_VERSION/kaiwo_linux_amd64 && \
mv kaiwo_linux_amd64 kaiwo && \
chmod +x kaiwo && \
sudo mv kaiwo /usr/local/bin/ && \
wget https://github.com/silogen/kaiwo/releases/download/$KAIWO_VERSION/workloads.zip && \
unzip workloads.zip && \
kaiwo version && \
kaiwo help
```

You're off to the races!

## Training

### Simple multi-GPU training workload

Let's take a look at how to deploy a multi-GPU training workload with Kaiwo. For training, we need to use a KaiwoJob. KaiwoJobs handle regular Batch Jobs and RayJobs. For example, if [Kaiwo's default image](https://github.com/orgs/silogen/packages/container/package/rocm-ray) is sufficient for your workload (note, the image assumes AMD GPUs), you can use the following which will run Direct Preference Optimization (DPO) on a single node with 4 GPUs:

```
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: dpo-singlenode
spec:
  user: test@amd.com
  gpus: 4
  entrypoint: |
    accelerate launch code/dpo.py \
    --dataset_name trl-lib/ultrafeedback_binarized \
    --model_name_or_path Qwen/Qwen2-0.5B-Instruct \
    --learning_rate 5.0e-6 \
    --num_train_epochs 1 \
    --per_device_train_batch_size 8 \
    --logging_steps 25 \
    --eval_strategy steps \
    --eval_steps 50 \
    --output_dir Qwen2-0.5B-DPO \
    --no_remove_unused_columns \
    --use_peft \
    --lora_r 32 \
    --lora_alpha 16 \
    --bf16 \
    --optim="adamw_torch"  
  storage:
    storageEnabled: true
    storageClassName: multinode
    data:
      storageSize: 10Mi
      mountPath: /workload
      download:
        git:
        - repository: https://github.com/silogen/kaiwo.git
          path: workloads/training/LLMs/dpo-singlenode
          targetPath: code
    huggingFace:
      storageSize: "30Gi"
      # automatically sets HF_HOME env variable for all containers
      preCacheRepos:
      - repoId: Qwen/Qwen2-0.5B-Instruct

```

You can submit this to kubernetes either via `kubectl apply -f filename.yaml` or by running `kaiwo submit -f filename.yaml`. Here are reasons why you may want to use `kaiwo submit`

1. Kaiwo will automatically add `user` (your email) and `clusterQueue` to the manifest upon submission. If it's your first submission, kaiwo cli will ask you for this information. You only have to do this once.
2. If your kaiwojobs include `user`, you can manage and monitor your workloads easily later with `kaiwo manage`. For example, you can list your submissions, check their GPU utilization, port-forward to a pod or execute commands.

After receiving this simple workload submission, Kaiwo Operator does the following at minimum:

1. It adds your workload to the queue you provided, assuming you were granted permission to use the queue. This depends on which namespace you have access to. Queueing policies are set by administrators in [KaiwoQueueConfig](../admin/configuration.md)
2. It runs your storage task (if any) and waits for it to finish before reserving GPUs. This is done by creating a separate Job that will run before your AI workload. 
3. Kaiwo will use binpacking and try find a node that is already partially reserved before looking for new nodes. If you didn't provide `gpuVendor` field, the operator will look for nodes with AMD GPUs. You can change this by providing `gpuVendor: nvidia`.
4. Kaiwo Operator creates environment variable `NUM_GPUS` in the container, which is set to the number of GPUs requested. This means you don't have to hardcode the number of GPUs in your code. You can use this variable in your entrypoint or in your code.

#### Using storage task with KaiwoJobs/Services

As our example above shows, Kaiwo makes it possible to download artifacts (model weights, data, code, etc.) into a persistent volume before starting the training workload. This is done by filling in `data` or `huggingFace` sections of KaiwoJob/KaiwoService. We must specify a mountPath under `data`. All subsequent targetPaths will be relative to this mountPath. The storage task will create separate persistent volume claims for `data` and `huggingFace` with the specified storageClassName and storageSize. We specify a mountPath for `huggingFace` as well, which will be used to set the HF_HOME environment variable in each container. This is useful if you want to use the HuggingFace cache in your training workload.  For more details on storage tasks, see [here](../reference/crds/kaiwo.silogen.ai.md#datastoragespec).

## Distributed training


## Batch inference (single- and multi-node)

## Online inference (single- and multi-node)