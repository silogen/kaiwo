# Getting started with Kaiwo

## Kaiwo operator and CRDs

At its core, Kaiwo is composed of the Kaiwo Custom Resource Definitions (CRDs) and the Kaiwo operator, which contains controllers that react to changes in the Kaiwo CRDs to manage workloads.

## Kaiwo CLI

!!!info
    Please see the [installation instructions](./installation.md#kaiwo-cli-tool) on how to install the CLI.

You can use the Kaiwo CLI to

* `kaiwo submit`: Quickly launch Kaiwo workloads (jobs and services)
* `kaiwo manage`: List and manage currently running workloads
* `kaiwo logs`: Fetch workload logs
* `kaiwo monitor`: Monitor (GPU) workloads
* `kaiwo exec`: Execute arbitrary commands inside the workload containers

For a list of full functionality run `kaiwo --help`, or for a specific command, `kaiwo <command> --help`.

### Before running workloads with Kaiwo

Kaiwo uses Kueue to manage job queuing. Make sure your cluster-admin has created two necessary Kueue resources on the cluster: `ResourceFlavor` and `ClusterQueue`. Manifests for these can be found under `cluster-admins` directory. By default, kaiwo will always submit workloads to `kaiwo` ClusterQueue if no other queue is provided with `-q`or`--queue` option during `kaiwo submit`. Kaiwo will automatically create the namespaced `LocalQueue` resource if it doesn't exist. Speak to your cluster-admin if you are unsure which `ClusterQueue` is allocated to your team.

### Submitting workloads

Given a Kaiwo manifest, you can submit it via the following command:

```
kaiwo submit -f <manifest.yaml>
```

As you may want to leave the user and queue definitions empty to allow different users to run the workload, you can provide this information via

* **CLI flags** by specifying `--user` and `--queue`
* **A YAML config file**, which contains the `user` and `queue` fields. This file can be provided via
  * The CLI flag `--config`
  * The environmental variable `KAIWOCONFIG`, which points to the path where the config exists
  * Placing the config file in `~/.config/kaiwo/kaiwoconfig.yaml`

!!!note
    The `user` field should be the user's email address

If you try to submit a workload without providing these values, you will be prompted to interactively create the Kaiwo config file.

!!!caution
    One important note about GPU requests: it is up to the user to ensure that the code can run on the requested number of GPUs. If the code is not written to run on the requested number of GPUs, the job will fail. Note that some parallelized code may only work on a specific number of GPUs such as 1, 2, 4, 8, 16, 32 but not 6, 10, 12 etc. If you are unsure, start with a single GPU and scale up as needed. For example, the total number of attention heads must be divisible by tensor parallel size.

### Managing workloads

You can list currently running workloads by running `kaiwo manage [flags]`. This displays a terminal application which you can use to:

* Select the workload type
* Delete the workload
* List the workload logs
* Port forward to the workload

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

