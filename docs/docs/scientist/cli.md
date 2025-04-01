
# Kaiwo CLI 

## Installation

The installation of Kaiwo CLI tool is easy as it's a single binary. The only requirement is a kubeconfig file to access a Kubernetes cluster (see authentication below for authentication plugins). If you are unsure where to get a kubeconfig, speak to your infrastructure/platform administrator. Just like kubectl, Kaiwo will first look for a `KUBECONFIG=path` environment variable. If `KUBECONFIG` is not set, Kaiwo will then look for kubeconfig file in the default location `~/.kube/config`.

First, look up the latest Kaiwo CLI binary under [Releases Page](https://github.com/silogen/kaiwo/releases) for your operating system. Then update KAIWO_VERSION in the following to the latest version and run it in your terminal:

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

Although not strictly required, we recommend that you also [install kubectl](https://kubernetes.io/docs/tasks/tools/) just in case you need some functionality that Kaiwo can't provide.

### Authentication

Speak to your infrastructure/platform administrator whether your cluster requires external authentication (OIDC, Azure, GKE). If authentication is required, you will have to install a separate authentication plugin. The following plugins should work with Kaiwo, so you won't necessarily need a separate installation of kubectl.

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

## Configuration

## Use