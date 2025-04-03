# Installing Kaiwo

## Kaiwo operator

### Dependencies

Kaiwo requires several components installed on Kubernetes:

1. Cert-Manager
2. GPU Operator: AMD Operator (with AMD-Device-Config and Node Labeller) or Nvidia Operator and Feature Discovery
3. Kueue Operator
4. KubeRay Operator
5. AppWrapper
6. Prometheus (not strictly necessary but recommended)

These and the Kaiwo operator itself can be installed in a few different ways.

### Using our convenience scripts

You can install Kaiwo and its dependencies into your currently active Kubernetes cluster via the convenience script

```
# From the remote script
curl -sSL https://raw.githubusercontent.com/silogen/kaiwo/main/get-kaiwo.sh | bash

# Or if you have cloned the repostiory
bash get-kaiwo.sh
```

This method assumes you have installed a GPU operator separately.

### Using Cluster Forge

Another option is using [Cluster-Forge](https://github.com/silogen/cluster-forge) to install all necessary components for Kubernetes. There is a README in the Cluster-Forge repo, but the steps are simple:

1. Clone the repository: `git clone https://github.com/silogen/cluster-forge.git`
2. Make sure you have Go installed
3. Run `go run . forge -s kaiwo` and select `kaiwo-all`, in addition to any other components you may want. Specifically, you may want to also include the `amd-gpu-operator` and `amd-device-config`.
4. Run the deploy script `bash stacks/kaiwo/deploy.sh`

This will deploy a working Kaiwo stack into your target Kubernetes cluster.

### Manually

If you prefer to manage the dependencies yourself, you can inspect the `/dependencies` folder to see what is required, and install Kaiwo yourself by using the `install.yaml` release from the [releases page](https://github.com/silogen/kaiwo/releases).

### Enabling auto-completion

The instructions for setting up auto-completion differ slightly by type of terminal. See help with `kaiwo completion --help`

For bash, you can run the following

```
sudo apt update && sudo apt install bash-completion && \
kaiwo completion bash | sudo tee /etc/bash_completion.d/kaiwo > /dev/null
```

You have to restart your terminal for auto-completion to take effect.