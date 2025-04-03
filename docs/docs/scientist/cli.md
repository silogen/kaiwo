
# Kaiwo CLI 

## Installation

The installation of Kaiwo CLI tool is easy as it's a single binary. The only requirement is a kubeconfig file to access a Kubernetes cluster (see authentication below for authentication plugins). If you are unsure where to get a kubeconfig, speak to your infrastructure/platform administrator. Just like kubectl, Kaiwo will first look for a `KUBECONFIG=path` environment variable. If `KUBECONFIG` is not set, Kaiwo will then look for kubeconfig file in the default location `~/.kube/config`.

You can use the convenience script to install the CLI:

```bash
curl -sSL https://raw.githubusercontent.com/silogen/kaiwo/main/get-kaiwo-cli.sh | bash -s --
```

This will install the latest CLI. If you want to install a different version, you can run

```bash
KAIWO_VERSION=vX.X.X curl -sSL https://raw.githubusercontent.com/silogen/kaiwo/main/get-kaiwo-cli.sh | bash -s --
```

You're off to the races!

If you want to uninstall the Kaiwo CLI, you can run

```bash
curl -sSL https://raw.githubusercontent.com/silogen/kaiwo/main/get-kaiwo-cli.sh | bash -s --
```

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

**Command-Line Flags:**

Users configure the `kaiwo` CLI tool via a YAML file. If this is not set, kaiwo cli will prompt you to create a config file interactively. The config file can be specified via the `--config` flag or the `KAIWOCONFIG` environment variable. If neither is set, kaiwo will look for a config file in the default location.

**Location Precedence:**

1.  Path specified by `--config <path>` flag in `kaiwo submit`.
2.  Path specified by the `KAIWOCONFIG` environment variable.
3.  Default path: `~/.config/kaiwo/kaiwoconfig.yaml`.

**Fields:**

*   `user`: The user's identifier (typically email) to be associated with submitted workloads (sets `spec.user` and `kaiwo.silogen.ai/user` label).
*   `clusterQueue`: The default Kueue `ClusterQueue` to submit workloads to (sets `spec.clusterQueue` and `kueue.x-k8s.io/queue-name` label).

**Example:**

```yaml
# ~/.config/kaiwo/kaiwoconfig.yaml
user: scientist@example.com
clusterQueue: team-a-queue
```

The `kaiwo submit` command provides an interactive prompt to create this file if it's missing and user/queue information isn't provided via flags.

## Use