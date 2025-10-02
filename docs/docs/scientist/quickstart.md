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

Let's take a look at how to deploy a multi-GPU training workload with Kaiwo. For training, we need to use a KaiwoJob. KaiwoJobs handle regular Batch Jobs and RayJobs. For example, if [Kaiwo's default image](https://github.com/orgs/silogen/packages/container/package/rocm-ray) is sufficient for your workload (note, the image assumes AMD GPUs), you can use the following which will run Direct Preference Optimization (DPO) on a single node with 4 GPUs. Source code and manifest can be found [here](https://github.com/silogen/kaiwo/tree/main/workloads/training/LLMs/dpo-singlenode):

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

Our example of distributed training uses Ray and DeepSpeed's Zero-3 optimization which partitions optimizer states, gradients, and parameters across GPUs. This allows us to train models that are larger than the memory of a single GPU. ZeRO-3 also includes the infinity offload engine, which can offload model states to RAM or NVME disks for significant GPU memory savings. Source code and manifest can be found [here](https://github.com/silogen/kaiwo/tree/main/workloads/training/LLMs/full-parameter-pretraining/full-param-zero3-single-multinode).

The training example requires object storage for saving model checkpoints. If you don't have an object storage solution, you can use MinIO, which is a self-hosted S3-compatible object storage solution. The following manifest will deploy MinIO on your cluster and create a bucket called `silogen-dev` for you. You can change the bucket name in the manifest if you want.

```
apiVersion: v1
kind: Secret
metadata:
  name: minio-secret
  namespace: kaiwo
data:
  access_key_id: bWluaW8= # minio
  secret_key: bWluaW8xMjM= # minio123
---

apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: minio-pvc
  namespace: kaiwo
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 500Gi
  storageClassName: multinode
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio-deployment
  namespace: kaiwo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
      - name: minio
        image: minio/minio
        args: ["server", "/data"]
        resources:
          limits:
            cpu: "1"
            memory: "1Gi"
          requests:
            cpu: "1"
            memory: "1Gi"
        env:
        - name: MINIO_ROOT_USER
          valueFrom:
            secretKeyRef:
              name: minio-secret
              key: access_key_id
        - name: MINIO_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: minio-secret
              key: secret_key
        ports:
        - containerPort: 9000
        volumeMounts:
        - name: minio-data
          mountPath: /data
      - name: bucket-init
        image: minio/mc
        resources:
          limits:
            cpu: "1"
            memory: "1Gi"
          requests:
            cpu: "1"
            memory: "1Gi"
        env:
        - name: MINIO_ROOT_USER
          valueFrom:
            secretKeyRef:
              name: minio-secret
              key: access_key_id
        - name: MINIO_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: minio-secret
              key: secret_key
        command:
        - sh
        - -c
        - |
          until /usr/bin/mc alias set local http://minio-service:9000 $MINIO_ROOT_USER $MINIO_ROOT_PASSWORD; do
            echo "Waiting for MinIO to be available..."
            sleep 5
          done
          /usr/bin/mc mb -p local/silogen-dev || echo "Bucket already exists"
          echo "Bucket init done, sleeping forever..."
          tail -f /dev/null
      volumes:
      - name: minio-data
        persistentVolumeClaim:
          claimName: minio-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: minio-service
  namespace: kaiwo
spec:
  selector:
    app: minio
  ports:
  - protocol: TCP
    port: 9000
    targetPort: 9000
    name: minio-endpoint
  type: ClusterIP


```

Once you have storage available, you can run the following KaiwoJob manifest. This will run a distributed training workload with 16 GPUs automatically using as many nodes as required. For example, if you have 4 x 8 GPU nodes where 50% of capacity is reserved on each node, Kaiwo will create 4 replicas each using 4 GPUs. Notice that this scenario of 50% GPU reservation on every node is unlikely. Kaiwo automatically uses binpacking, ensuring that each GPU node is used to maximum capacity before another GPU node is requested. 

The following manifest will also create a persistent volume claim for the model weights. The model weights will be downloaded from HuggingFace and cached in the persistent volume before any GPUs are reserved. Notice that you will need a secret that holds your Huggingface token if you are downloading a gated model.

```

apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: multinode-stage-zero3-pretraining-example
spec:
  user: test@amd.com
  gpus: 16
  ray: true
  entrypoint: |
    python code/main.py \
    --model-name=meta-llama/Llama-3.1-8B-Instruct \
    --ds-config=./code/zero_3_offload_optim_param.json \
    --bucket=silogen-dev \
    --num-epochs=1 \
    --num-devices=$NUM_GPUS \
    --batch-size-per-device=32 \
    --eval-batch-size-per-device=32 \
    --ctx-len=1024
  env:
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: minio-secret
        key: access_key_id
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: minio-secret
        key: secret_key
  - name: HF_TOKEN
    valueFrom:
      secretKeyRef:
        name: hf-token
        key: hf-token
  - name: MODEL_ID
    value: meta-llama/Llama-3.1-8B-Instruct
  storage:
    storageEnabled: true
    storageClassName: multinode
    data:
      storageSize: 20Mi
      mountPath: /workload
      download:
        git:
        - repository: https://github.com/silogen/kaiwo.git
          path: workloads/training/LLMs/full-parameter-pretraining/full-param-zero3-single-multinode
          targetPath: code
    huggingFace:
      storageSize: "100Gi"
      mountPath: "/hf_cache" # Also sets the env var HF_HOME to this value in each container
      preCacheRepos:
      - repoId: meta-llama/Llama-3.1-8B-Instruct
        files: []
```

Notice that our finetuning example is almost identical to this pre-training example except the former uses Lora for parameter-efficient finetuning. The only difference is the entrypoint and Lora config. You can find the manifest and source code [here](https://github.com/silogen/kaiwo/tree/main/workloads/training/LLMs/lora-supervised-finetuning/lora-sft-zero3-single-multinode)

## Batch inference (single- and multi-node)

Batch inference is a common use case for AI workloads, where large amounts of data are processed in batches to generate predictions or insights. Kaiwo supports batch inference workloads using KaiwoJobs. The following example demonstrates how to deploy a batch inference workload with Kaiwo. Our example uses vLLM and Ray to scale up inference to multiple replicas on multiple nodes (one model replica per node). 

Once again, model weights will be downloaded from HuggingFace and cached in the persistent volume before any GPUs are reserved. Notice that you will need a secret that holds your Huggingface token if you are downloading a gated model.

Run offline inference with the following manifest. Source code and manifest can be found [here](https://github.com/silogen/kaiwo/tree/main/workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode)

```
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: batch-inference-vllm-example
spec:
  user: test@amd.com
  gpusPerReplica: 4
  replicas: 1
  ray: true
  entrypoint: python code/main.py
  env:
  - name: HF_TOKEN
    valueFrom:
      secretKeyRef:
        name: hf-token
        key: hf-token
  - name: MODEL_ID
    value: meta-llama/Llama-3.1-8B-Instruct
  storage:
    storageEnabled: true
    storageClassName: multinode
    data:
      storageSize: 20Mi
      mountPath: /workload
      download:
        git:
        - repository: https://github.com/silogen/kaiwo.git
          path: workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode
          targetPath: code
    huggingFace:
      storageSize: "100Gi"
      mountPath: "/hf_cache" # Also sets the env var HF_HOME to this value in each container
      preCacheRepos:
      - repoId: meta-llama/Llama-3.1-8B-Instruct
        files: []
```


## Online inference (single- and multi-node)

Similar to our offline example, our online inference example also uses vLLM and Ray. The online inference example supports multi-node inference (one model partitioned across multiple nodes). However, we recommend sticking to a single node when possible as there are likely to be performance penalties from network bottlenecks.

Run online inference with the following manifest. Source code and manifest can be found [here](https://github.com/silogen/kaiwo/tree/main/workloads/inference/LLMs/online-inference/vllm-online-single-multinode)

```
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoService
metadata:
  name: online-inference-vllm-example
spec:
  user: test@amd.com
  gpus: 4
  ray:
    serveConfigV2: |
      applications:
      - name: llm
        route_prefix: /
        import_path: workloads.inference.LLMs.online-inference.vllm-online-single-multinode:deployment
        runtime_env:
            working_dir: "https://github.com/silogen/kaiwo/archive/b66c14303f3beadc08e11a89fd53d7f71b2a15cd.zip"
        deployments:
        - name: VLLMDeployment
          autoscaling_config:
            metrics_interval_s: 0.2
            look_back_period_s: 2
            downscale_delay_s: 600
            upscale_delay_s: 30
            target_num_ongoing_requests_per_replica: 20
          graceful_shutdown_timeout_s: 5
          max_concurrent_queries: 100
  env:
  - name: NCCL_P2P_DISABLE
    value: "1"
  - name: MODEL_ID
    value: "meta-llama/Llama-3.1-8B-Instruct"
  - name: GPU_MEMORY_UTILIZATION
    value: "0.9"
  - name: PLACEMENT_STRATEGY
    value: "PACK"
  - name: MAX_MODEL_LEN
    value: "8192"
  - name: MAX_NUM_SEQ
    value: "4"
  - name: MAX_NUM_BATCHED_TOKENS
    value: "32768"
  - name: HF_TOKEN
    valueFrom:
      secretKeyRef:
        name: hf-token
        key: hf-token
  storage:
    storageEnabled: true
    storageClassName: multinode
    huggingFace:
      storageSize: "100Gi"
      mountPath: "/hf_cache" # Also sets the env var HF_HOME to this value in each container
      preCacheRepos:
      - repoId: meta-llama/Llama-3.1-8B-Instruct
        files: []
```