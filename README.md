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

- Use the tagged [releases on GitHub](https://github.com/silogen/kaiwo/releases) for the most stable and tested versions.
- Avoid building directly from the **`main`** branch unless you are comfortable with potential instability or are contributing to the project.

**2.** ****Monitor Changes****

- Keep an eye on the [Changelog](https://github.com/silogen/kaiwo/CHANGELOG.md) for updates and breaking changes.

**3.** ****Provide Feedback****

- If you encounter any issues or have suggestions, feel free to open an issue in the [Issues section](https://github.com/silogen/kaiwo/issues).

## Description

**Kaiwo** (pronunciation *"ky-voh"*) is a Kubernetes-native tool designed to optimize GPU resource utilization for AI workloads. The project is built primarily for AMD GPUs. Built on top of **Ray** and **Kueue** , Kaiwo minimizes GPU idleness and increases resource efficiency through intelligent job queueing, fair sharing of resources, guaranteed quotas and opportunistic gang scheduling.

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

The installation of Kaiwo CLI tool is easy as it's a single binary. The only requirement is a kubeconfig file to access a Kubernetes cluster (see authentication below for authentication plugins). If you are unsure where to get a kubeconfig, speak to the engineers who set up your Kubernetes cluster. Just like kubectl, Kaiwo will first look for a `KUBECONFIG=path` environment variable. If `KUBECONFIG` is not set, Kaiwo will then look for kubeconfig file in the default location `~/.kube/config`.

1. To install Kaiwo, download the Kaiwo CLI binary from the [Releases Page](https://github.com/silogen/kaiwo/releases).
2. Make the binary executable and add it to your PATH

To do both steps in one command for Linux (AMD64), edit `v.x.x.x` in the following and run it

```bash
wget https://github.com/silogen/kaiwo/releases/download/v.x.x.x/kaiwo_linux_amd64 && \
mv kaiwo_linux_amd64 kaiwo && \
chmod +x kaiwo && \
sudo mv kaiwo /usr/local/bin/
```

3. You're off to the races!

Although not required by Kaiwo, we also recommend that you [install kubectl](https://kubernetes.io/docs/tasks/tools/) just in case you need some functionality that Kaiwo can't provide.

#### Authentication

If your cluster uses external authentication (OIDC, Azure, GKE), you will have to install a separate authentication plugin. These plugins should work with Kaiwo, so you won't need a separate installation of kubectl.

Links to authentication plugins:

- [int128/kubelogin: kubectl plugin for Kubernetes OpenID Connect authentication (kubectl oidc-login)](https://github.com/int128/kubelogin))(tested)
- [Azure Kubelogin](https://azure.github.io/kubelogin/index.html)(untested)
- [GKE: gke-gcloud-auth-plugin](https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke)(tested)

For example, your kubeconfig that uses kubelogin should resemble the following format:


```
apiVersion: v1
clusters:
-   cluster:
        server: https://kube-api-endpoint:6443
    name: default
contexts:
-   context:
        cluster: default
        user: default
    name: default
current-context: default
kind: Config
preferences: {}
users:
-   name: default
    user:
        exec:
            apiVersion: client.authentication.k8s.io/v1beta1
            args:
            - get-token
            - --oidc-issuer-url=https://my.issuer.address.com/realms/realm_name
            - --oidc-client-id=my_client_id_name
            - --oidc-client-secret=my_client_id_secret
            command: kubelogin
            interactiveMode: IfAvailable
            provideClusterInfo: false
```

In case your certificate trust store gives "untrusted" certificate errors, you can use `insecure-skip-tls-verify: true` under cluster and `--insecure-skip-tls-verify` in `kubelogin get-token` as a temporary workaround. As usual, we don't recommend this in production.  


## Usage

Run `kaiwo --help` for an overview of currently available commands.

### Before running workloads with Kaiwo

Kaiwo uses Kueue to manage job queuing. Make sure your cluster-admin has created two necessary Kueue resources on the cluster: `ResourceFlavor` and `ClusterQueue`. Manifests for these can be found under `cluster-admins` directory. By default, kaiwo will always submit workloads to `kaiwo` ClusterQueue if no other queue is provided with `-q`or`--queue` option during `kaiwo submit`. Kaiwo will automatically create the namespaced `LocalQueue` resource if it doesn't exist. Speak to your cluster-admin if you are unsure which `ClusterQueue` is allocated to your team.

### Deploying with kaiwo

Kaiwo allows you to both `submit` jobs and `serve` deployments. It's important to note that `kaiwo serve` does not use Kueue or any other form of job queueing.  The assumption is that immediate serving of models is required when using `serve`. We encourage users to use separate clusters for `submit` and `serve`. Currently, the following types are supported:

* Standard Kubernetes jobs `batch/v1 Job` via `kaiwo submit`
* Ray jobs `ray.io/v1 RayJob` via `kaiwo submit --ray`
* Ray services `ray.io/v1 RayService` via `kaiwo serve --ray`
* Standard Kubernetes deployments `apps/v1 Deployment` via `kaiwo serve`

The `workloads` directory includes examples with code for different types workloads that you can create.

RayServices are intended for online inference. They bypass job queues. We recommend running them in a separate cluster as services are constantly running and therefore reserve compute resources 24/7.

RayJobs and RayServices require using the `-p`/`--path` option. Kaiwo will look for `entrypoint` or `serveconfig` files in `path` which are required for RayJobs and RayServices, respectively.

Run `kaiwo submit --help` and `kaiwo serve --help` for an overview of available options. To get started with a workload, first make sure that your code (e.g. finetuning script) works with the number of GPUs that you request via `kaiwo submit/serve`.  For example, the following command will run the code found in `path` as a RayJob on 16 GPUs.

`kaiwo submit -p path/to/workload/directory -g 16 --ray`

By default, this will run in `kaiwo` namespace unless another namespace is provided with `-n` or `--namespace` option. If the provided namespace doesn't exist, use `--create-namespace` flag.

You can also run a workload by just passing `--image` or `-i` flag.

`kaiwo submit -i my-registry/my_image -g 8`

Or, you may want to mount code from a github repo at runtime and only modify the entrypoint for the running container. In such a case and when submitting a standard Kubernetes Job, add `entrypoint` file to `--path` directory and submit your workload like so

`kaiwo submit -i my-registry/my_image -p path_to_entrypoint_directory -g 8`

One important note about GPU requests: it is up to the user to ensure that the code can run on the requested number of GPUs. If the code is not written to run on the requested number of GPUs, the job will fail. Note that some parallelized code may only work on a specific number of GPUs such as 1, 2, 4, 8, 16, 32 but not 6, 10, 12 etc. If you are unsure, start with a single GPU and scale up as needed. For example, the total number of attention heads must be divisible by tensor parallel size.

When passing custom images, please be mindful that kaiwo mounts local files to `/workload` for all jobs and to `/workload/app` for services (RayService, Deployment) to adhere to `RayService` semantics

#### Note about environment variables

Kaiwo cannot assume how secret management has been set up on your cluster (permissions to create/get secrets, backend for ExternalSecrets, etc.). Therefore, Kaiwo does not create secrets for you. If your workload requires secrets, you must create them yourself. You can create secrets in the namespace where you are running your workload. If you are using ExternalSecrets, make sure that the ExternalSecrets are created in the same namespace where you are running your workload.

To pass environment variables (from secrets or otherwise) into your workload, you can add `env` file to the `--path` directory. The file format follows YAML syntax and looks something like this:

```yaml
envVars:
  - name: MY_VAR
    value: "my_value"
  - fromSecret:
      name: "AWS_ACCESS_KEY_ID"
      secret: "gcs-credentials"
      key: "access_key"
  - fromSecret:
      name: "AWS_SECRET_ACCESS_KEY"
      secret: "gcs-credentials"
      key: "secret_key"
  - fromSecret:
      name: "HF_TOKEN"
      secret: "hf-token"
      key: "hf-token"
  - mountSecret:
      name: "GOOGLE_APPLICATION_CREDENTIALS"
      secret: "gcs-credentials"
      key: "gcs-credentials-json"
      path: "/etc/gcp/credentials.json"
```

#### Enabling auto-completion for kaiwo

The instructions for setting up auto-completion differ slightly by type of terminal. See help with `kaiwo completion --help`

For bash, you can run the following

```bash
sudo apt update && sudo apt install bash-completion && \
kaiwo completion bash | sudo tee /etc/bash_completion.d/kaiwo > /dev/null
```

You have to restart your terminal for auto-completion to take effect.

### Workload templates

Kaiwo manages Kubernetes workloads through templates. These templates are YAML files that use go template syntax. If you do not provide a template when submitting or serving a workload by using the `--template` flag, a default template is used. The context available for the template is defined by the [WorkloadTemplateConfig struct](./pkg/workloads/config.go), and you can refer to the default templates for [Ray](pkg/workloads/ray) and [Kueue](./pkg/workloads/jobs).

If you want to provide custom configuration for the templates, you can do so via the `-c / --custom-config` flag, which should point to a YAML file. The contents of this file will then be available under the `.Custom` key in the templates. For example, if you provide a YAML file with the following structure

```yaml
parent:
  child: "value"
```

You can access this in the template via

```gotemplate
{{ .Custom.parent.child }}
```

## Contributing to Kaiwo

TODO

We welcome contributions to Kaiwo! Please refer to the [Contributing Guidelines](contributing-guidelines.md) for more information on how to contribute to the project.
