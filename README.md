```plaintext
 _  __     _
| |/ /__ _(_)_      _____
| ' // _' | \ \ /\ / / _ \
| . \ (_| | |\ V  V / (_) |
|_|\_\__,_|_| \_/\_/ \___/
Kubernetes-native AI Workload Orchestrator
```

# Kaiwo - Kubernetes-native AI Workload Orchestrator to accelerate GPU workloads

🚀️🚀️ Kaiwo supports ***AMD*** GPUs! 🚀️🚀️

⚠️ **Caveat: Heavy Development in Progress** ⚠️

This repository is under active and ****heavy development****, and the codebase is subject to frequent changes, including potentially some breaking updates. While we strive to maintain stability, please be aware that the **`main`** branch may not always be in a functional or stable state.

To ensure a smooth experience, we strongly recommend that users:

**1.** ****Stick to Stable Releases****

- Use the tagged [releases on GitHub](https://github.com/silogen/ai-workload-orchestrator/releases) for the most stable and tested versions.
- Avoid building directly from the **`main`** branch unless you are comfortable with potential instability or are contributing to the project.

**2.** ****Monitor Changes****

- Keep an eye on the [Changelog](https://github.com/silogen/ai-workload-orchestrator/CHANGELOG.md) for updates and breaking changes.

**3.** ****Provide Feedback****

- If you encounter any issues or have suggestions, feel free to open an issue in the [Issues section](https://github.com/silogen/ai-workload-orchestrator/issues).

## Description

**Kaiwo** is a Kubernetes-native tool designed to optimize GPU resource utilization for AI workloads. The project is built primarily for AMD GPUs. Built on top of **Ray** and **Kueue** , Kaiwo minimizes GPU idleness and increases resource efficiency through intelligent job queueing, fair sharing of resources, guaranteed quotas and opportunistic batch job scheduling.

Kaiwo supports a wide range of AI workloads, including distributed multi-node pretraining, fine-tuning, online inference, and batch inference, with seamless integration into Kubernetes environments.

## Main Features

* **GPU Utilization Optimization** :
  * Dynamically queues workloads to reduce GPU idle time and maximize resource utilization.
* **CLI Tool** :
  * Simplified workload submission using the kaiwo CLI tool
* **Distributed Workload Scheduling** :
  * Effortlessly schedule distributed workloads across multiple Kubernetes nodes.
* **Broad Workload Support** with pre-built templates:
  * Supports running **Kubernetes Jobs**, **RayJobs** and **RayServices**.
* **Integration with Ray and Kueue** :
  * Leverages the power of Ray for distributed computing and Kueue for efficient job queueing.

## Installation

### Installation of Ray and Kueue Operators on Kubernetes

Kaiwo requires 4-5 components installed on Kubernetes:

1. Cert-Manager
2. AMD Operator (with AMD-Device-Config) or Nvidia Operator
3. Kueue Operator
4. KubeRay Operator
5. Prometheus (not strictly necessary but recommended)

We recommend using [Cluster-Forge](https://github.com/silogen/cluster-forge) to install all necessary components for Kubernetes. There is a README in the Cluster-Forge repo, but the steps are simple:

1. Clone the repository: `git clone https://github.com/silogen/cluster-forge.git`
2. Make sure you have Go installed
3. Run `./scripts/clean.sh` to make sure you're starting off clean slate
4. Run `go run . smelt` and select your components (above)
5. Make sure docker is using multiarch-builder `docker buildx create --name multiarch-builder --use`
6. Run `go run . cast`
7. Run `go run . forge`

### Installation of Kaiwo CLI tool

The installation of Kaiwo CLI tool is easy as it's a single binary. The only requirement is a kubeconfig file to access a Kubernetes cluster. If you are unsure where to get a kubeconfig, speak to the engineers who set up your Kubernetes cluster. Just like kubectl, Kaiwo will first look for a `KUBECONFIG=path` environment variable. If `KUBECONFIG` is not set, Kaiwo will then look for kubeconfig file in the default location `~/.kube/config`.

1. Download the Kaiwo CLI binary from the [Releases Page](https://github.com/silogen/ai-workload-orchestrator/releases).
2. Make the binary executable and add it to your PATH:

```bash
chmod +x kaiwo
sudo mv kaiwo /usr/local/bin/
```

## Usage

Run `kaiwo --help` for an overview of currently available commands.

### Before running workloads with Kaiwo

Kaiwo uses Kueue to manage job queuing. Make sure your cluster-admin has created two necessary Kueue resources on the cluster: `ResourceFlavor` and `ClusterQueue`. Manifests for these can be found under `cluster-admins` directory. By default, kaiwo will always submit workloads to `kaiwo` ClusterQueue if no other queue is provided with `-q`or`--queue` option during `kaiwo submit`. Kaiwo will automatically create the namespaced `LocalQueue` resource if it doesn't exist. Speak to your cluster-admin if you are unsure which `ClusterQueue` is allocated to your team.

### kaiwo submit

`workloads` directory includes examples with code for different types workloads that you can submit with `kaiwo submit`. At the moment, Kaiwo can submit three types of workloads to Kubernetes

- Standard Kubernetes Jobs
- RayJobs
- RayServices

RayServices are intended for online inference. They bypass job queues. We recommend running them in a separate cluster as services are constantly running and therefore reserve compute resources 24/7.

RayJobs and RayServices require using `-p` or `--path` option. Kaiwo will look for `rayjob-entrypoint` or `rayservice-serviceconfig` files in these paths to determine which one is in question. Kubernetes Job is the default workload type if no `-p` option is provided or if `rayjob-entrypoint` or `rayservice-serviceconfig` files are not found in the path.

Run `kaiwo submit --help` for an overview of available options. To get started with a workload, first make sure that your code (e.g. finetuning script) works with the number of GPUs that you request via `kaiwo submit`.  For example, the following command will run the code found in `path` as a RayJob on 16 GPUs.

`kaiwo submit -p workloads/training/LLMs/lora-supervised-finetuning/ds-zero3-single-multinode -g 16`

By default, this will run in `kaiwo` namespace unless another namespace is provided with `-n` or `--namespace` option. If the provided namespace doesn't exist, use `--create-namespace` flag.

You can also run a workload by just passing `--image` or `-i` flag.

`kaiwo submit -i my-registry/my_image -g 8`

Or, you may want to mount code from a github repo at runtime and only modify the entrypoint for the running container. In such a case and when submitting a standard Kubernetes Job, add `entrypoint` file to `--path` directory and submit your workload like so

`kaiwo submit -i my-registry/my_image -p path_to_entrypoint_directory -g 8`

TODO, describe 

- Note about typical secrets and environment variables (s3 keys, HF TOKEN, etc)
- Note about how secrets are managed (ExternalSecrets, etc)

## Contributing to Kaiwo

TODO

We welcome contributions to Kaiwo! Please refer to the [Contributing Guidelines]() for more information on how to contribute to the project.
