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

Let's take a look at how to deploy a multi-GPU training workload with Kaiwo. For this, we need to use a KaiwoJob. KaiwoJobs handle regular Batch Jobs and RayJobs. If `image` has a sensible entrypoint, our submission can be as simple as

```
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: hello-kaiwojob
spec:
  gpus: 4
  image: ghcr.io/my-image-with-entrypoint

```

You can submit this to kubernetes either via `kubectl apply -f filename.yaml` or by running `kaiwo submit -f filename.yaml`. Here are reasons why you may want to use `kaiwo submit`

1. kaiwo will automatically add `user` and `clusterQueue` to the manifest upon submission. If it's your first submission, kaiwo cli will ask you for this information.
2. if your kaiwojobs include your name, you can manage and monitor your workloads easily later with `kaiwo manage`. For example, you can list your submissions, check their GPU utilization, port-forward to a pod or execute commands.

After receiving this workload, Kaiwo Operator does the following

1. It adds your workload to the queue you provided, assuming you were granted permission to use the queue. This depends on which namespace you have access to. Queueing policies are set by administrators in [KaiwoQueueConfig](../admin/configuration.md)
2. Kaiwo will use binpacking and try find a node that is already partially reserved before looking for new nodes. If you didn't provide `gpuVendor` field, the operator will look for nodes with AMD GPUs. You can change this by providing `gpuVendor: nvidia`.
3. 

## Distributed training


## Inference

## Distributed inference