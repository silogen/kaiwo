
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

## Usage

You can use the Kaiwo CLI to

* `kaiwo submit`: Quickly launch Kaiwo workloads (jobs and services)
* `kaiwo manage`: List and manage currently running workloads
* `kaiwo logs`: Fetch workload logs
* `kaiwo monitor`: Monitor (GPU) workloads
* `kaiwo exec`: Execute arbitrary commands inside the workload containers
* `kaiwo stats`: Check the status of your cluster

For a list of full functionality run `kaiwo --help`, or for a specific command, `kaiwo <command> --help`.

### Before running workloads with Kaiwo

Kaiwo uses Kueue to manage job queuing. Make sure your cluster-admin has created two necessary Kueue resources on the cluster: `ResourceFlavor` and `ClusterQueue`. Manifests for these can be found under `cluster-admins` directory. By default, kaiwo will always submit workloads to `kaiwo` ClusterQueue if no other queue is provided with `-q`or`--queue` option during `kaiwo submit`. Kaiwo will automatically create the namespaced `LocalQueue` resource if it doesn't exist. Speak to your cluster-admin if you are unsure which `ClusterQueue` is allocated to your team.

### Submitting workloads

Given a Kaiwo manifest, you can submit it via the following command:

```
kaiwo submit -f <manifest.yaml>
```

As you may want to leave the user and queue definitions empty to allow different users to run the workload, you can provide this information via other methods (see above).

!!!caution
    One important note about GPU requests: it is up to the user to ensure that the code can run on the requested number of GPUs. If the code is not written to run on the requested number of GPUs, the job will fail. Note that some parallelized code may only work on a specific number of GPUs such as 1, 2, 4, 8, 16, 32 but not 6, 10, 12 etc. If you are unsure, start with a single GPU and scale up as needed. For example, the total number of attention heads must be divisible by tensor parallel size.

### Managing workloads

You can list currently running workloads by running `kaiwo manage [flags]`. This displays a terminal application which you can use to:

* Select the workload type
* Delete the workload
* List the workload logs
* Port forward to the workload
* Run a command within a workload container
* Monitor a workload pod

By default, only workloads that you have submitted are shown. You can use the following flags:

* `-n / --namespace` to specify the namespace
* `-u / --user` to specify a different user
* `--all-users` to show workloads from all users

### Fetching workload logs

You can fetch workload logs by running

```
kaiwo logs <workloadType>/<workloadName> [flags]
```

where `<workloadType>` is either `job` or `service`.

The following flags are supported:

* `-f / --follow` to follow the output
* `-n / --namespace` to specify the namespace
* `--tail` to specify the number of lines to tail

### Monitoring workloads

You can monitor workload GPU usage by running

```
kaiwo monitor <workloadType>/<workloadName> [flags]
```

where `<workloadType>` is either `job` or `service`.

The following flags are supported:

* `-n / --namespace` to specify the namespace

### Executing commands

You can execute a command inside a container interactively by running

```
kaiwo exec <workloadType>/<workloadName> [flags]
```

where `<workloadType>` is either `job` or `service`.

The following flags are supported:

* `-i / --interactive` to enable interactive mode (default true)
* `-t / --tty` to enable TTY (default true)
* `--command` to specify the command to execute
* `-n / --namespace` to specify the namespace

### Checking cluster status

You can check the current resource availability (including GPUs) of your cluster by running: 

```
kaiwo stats nodes
```

You can check the current queue statuses for GPU jobs by running:

```
kaiwo stats queues
```