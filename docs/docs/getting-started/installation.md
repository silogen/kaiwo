# Installing Kaiwo

## Pre-requisites (Ray and Kueue operators)

Kaiwo requires 4-5 components installed on Kubernetes:

1. Cert-Manager
2. AMD Operator (with AMD-Device-Config and Node Labeller) or Nvidia Operator and Feature Discovery
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

## Kaiwo operator

!!!warning
    This release is intended purely for internal testing of forth-coming Kaiwo-Operator. Not intended for general use

Install the operator by running:

```bash
kubectl apply -f https://github.com/silogen/kaiwo/releases/download/v.0.1-internal/install.yaml --server-side
```

## Kaiwo CLI tool

The installation of Kaiwo CLI tool is easy as it's a single binary. The only requirement is a kubeconfig file to access a Kubernetes cluster (see authentication below for authentication plugins). If you are unsure where to get a kubeconfig, speak to the engineers who set up your Kubernetes cluster. Just like kubectl, Kaiwo will first look for a `KUBECONFIG=path` environment variable. If `KUBECONFIG` is not set, Kaiwo will then look for kubeconfig file in the default location `~/.kube/config`.

1. To install Kaiwo, download the Kaiwo CLI binary from the [Releases Page](https://github.com/silogen/kaiwo/releases).
2. Make the binary executable and add it to your PATH

To do both steps in one command for Linux (AMD64), edit `v.x.x.x` in the following and run it

```bash
export KAIWO_VERSION=v.x.x.x && \
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

Although not required by Kaiwo, we also recommend that you [install kubectl](https://kubernetes.io/docs/tasks/tools/) just in case you need some functionality that Kaiwo can't provide.

### Authentication

If your cluster uses external authentication (OIDC, Azure, GKE), you will have to install a separate authentication plugin. These plugins should work with Kaiwo, so you won't need a separate installation of kubectl.

Links to authentication plugins:

- [int128/kubelogin: kubectl plugin for Kubernetes OpenID Connect authentication (kubectl oidc-login)](https://github.com/int128/kubelogin)) (tested)
- [Azure Kubelogin](https://azure.github.io/kubelogin/index.html) (untested)
- [GKE: gke-gcloud-auth-plugin](https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke) (tested)

For example, your kubeconfig that uses kubelogin should resemble the following format:

```yaml
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

### Enabling auto-completion

The instructions for setting up auto-completion differ slightly by type of terminal. See help with `kaiwo completion --help`

For bash, you can run the following

```
sudo apt update && sudo apt install bash-completion && \
kaiwo completion bash | sudo tee /etc/bash_completion.d/kaiwo > /dev/null
```

You have to restart your terminal for auto-completion to take effect.