# Custom Resource Definitions (CRDs)

The Kaiwo operator makes use of several Kubernetes Custom Resource Definitions (CRDs) to deploy and manage workloads. This document describes the most important fields and how they are interpreted by the Kaiwo operator.

!!!note
    Please see [here](../reference/crds/kaiwo.silogen.ai.md) for the full technical description of the available CRDs.

## Workload CRDs

### [Common fields](../reference/crds/kaiwo.silogen.ai.md#commonmetaspec)

These fields are shared between the `KaiwoJob` and `KaiwoService` CRDs, and are all available under the `spec` field.

#### `user`

The `user` field specifies the owner or creator of the workload. It should typically be the user's email address. This value is primarily used for labeling (`kaiwo.silogen.ai/user`) the generated resources (like Pods, Jobs, Deployments) for identification and filtering (e.g., with `kaiwo list --user <email>`).

#### `version`

The `version` field allows you to specify an optional version string for the workload. This can be useful for tracking different iterations or configurations of the same logical workload. It does not directly affect resource creation but serves as metadata.

#### `replicas`

The `replicas` field determines the desired number of instances for the workload.

*   For a `KaiwoService` using a standard Kubernetes `Deployment` (`ray: false`), this sets `deployment.spec.replicas`.
*   For a `KaiwoService` or `KaiwoJob` using Ray (`ray: true`), this sets the number of replicas for the Ray worker group (`rayClusterSpec.workerGroupSpecs[].replicas`). It also locks `minReplicas` and `maxReplicas` to this value.
*   This field is *ignored* for `KaiwoJob` using a standard Kubernetes `Job` (`ray: false`).
*   Default value is 1.

#### `gpus`

The `gpus` field specifies the *total* number of GPUs requested for the entire workload (across all replicas).

*   If `replicas` and `gpusPerReplica` are also specified, `gpus` is recalculated based on those values.
*   For non-Ray Jobs and Deployments, this value helps determine the GPU resources requested per pod if `gpusPerReplica` is not set or if there's only one replica.
*   If set and if `gpusPerReplica` is not set, Kaiwo automatically adjusts `gpusPerReplica` and `replicas` (for Ray workloads) based on available node capacity and scheduling constraints.
*   Default value is 0.

#### `gpuVendor`

The `gpuVendor` field specifies the vendor of the GPUs required (e.g., "AMD", "NVIDIA").

*   This determines which Kubernetes resource name is used for GPU requests (e.g., `amd.com/gpu`, `nvidia.com/gpu`).
*   It's also used in scheduling calculations to find appropriate nodes.
*   Default value is "AMD".

#### `gpusPerReplica`

The `gpusPerReplica` field specifies the number of GPUs allocated to *each* replica of the workload.

*   This directly translates to the GPU resource request/limit for the primary container in each pod.
*   Used in conjunction with `replicas` or `gpus` to determine the overall GPU allocation and scheduling strategy, particularly for Ray workloads.
*   If set, it overrides automatic calculation based on the `gpus` field alone.

#### `podTemplateSpecLabels`

The `podTemplateSpecLabels` field allows you to specify custom labels that will be added to the `template.metadata.labels` section of the generated Pods (within Jobs, Deployments, or RayCluster specs). Standard Kaiwo system labels (like `kaiwo.silogen.ai/user`, `kaiwo.silogen.ai/name`, etc.) are added automatically and take precedence if there are conflicts.

#### `resources`

The `resources` field defines default Kubernetes `ResourceRequirements` (requests and limits for CPU, memory, etc.) that are applied to *all* containers within the workload's pods (including init containers).

*   If a container within the underlying Job, Deployment, or Ray spec *already* defines resources, these defaults will *not* override them unless the specific resource (e.g., memory limit) is missing in the container's definition.
*   GPU resources are typically managed via the `gpus` and `gpusPerReplica` fields and might override GPU settings defined here, depending on the workload type and configuration.

#### `image`

The `image` field specifies the default container image to be used for the primary workload container(s).

*   If containers defined within the underlying Job, Deployment, or Ray spec do *not* specify an image, this image will be used.
*   If this field is also empty, a system default (e.g., `ghcr.io/silogen/rocm-ray:v0.8`) might be used.

#### `imagePullSecrets`

The `imagePullSecrets` field is a list of Kubernetes `LocalObjectReference` (containing just the secret `name`) referencing secrets needed to pull the container image(s). These are added to the `imagePullSecrets` field of the PodSpec for all generated pods.

#### `env`

The `env` field is a list of Kubernetes `EnvVar` structs. These environment variables are added to the primary workload container(s) in the generated pods. They are appended to any environment variables already defined in the underlying Job, Deployment, or Ray spec.

#### `secretVolumes`

The `secretVolumes` field allows you to mount specific keys from Kubernetes Secrets as files into the workload containers. Each item in the list defines:

*   `name`: The name for the volume.
*   `secretName`: The name of the Secret resource.
*   `key`: (Optional) The specific key within the Secret to mount. If omitted, the entire secret might be mounted (behavior depends on Kubernetes).
*   `subPath`: (Optional) The filename within the `mountPath` directory where the secret key's content will be placed.
*   `mountPath`: The path inside the container where the secret volume (or subPath file) will be mounted.

This adds corresponding `Volume` and `VolumeMount` entries to the PodSpec.

#### `ray`

The `ray` field is a boolean flag that determines whether the workload should be executed using Ray (RayJob for `KaiwoJob`, RayService for `KaiwoService`).

*   If `true`, Kaiwo will create Ray-specific resources.
*   If `false` (default), Kaiwo will create standard Kubernetes resources (BatchJob for `KaiwoJob`, Deployment for `KaiwoService`).
*   This setting dictates which underlying spec (`job`/`rayJob` or `deployment`/`rayService`) is primarily used.

#### `storage`

The `storage` field configures persistent storage using Kubernetes PersistentVolumeClaims (PVCs). It contains sub-fields:

*   `storageEnabled`: (boolean) Must be `true` to enable any storage creation.
*   `storageClassName`: Specifies the `StorageClass` to use for the created PVCs.
*   `accessMode`: Sets the `accessMode` for the PVCs (e.g., `ReadWriteOnce`, `ReadWriteMany`). Default is `ReadWriteMany`.
*   `data`: Configures the main data volume.
    *   `mountPath`: Path inside containers to mount the data PVC. Defaults to `/workload`.
    *   `storageSize`: Size of the data PVC (e.g., "100Gi"). Setting this enables the data PVC.
    *   `download`: Configures pre-downloading data from object storage or Git into the data volume using an init job. Contains lists (`s3`, `gcs`, `azureBlob`, `git`) specifying source details, credentials (via `ValueReference` pointing to Secrets), and target paths within the `mountPath`.
*   `huggingFace`: Configures a volume for Hugging Face model caching.
    *   `mountPath`: Path inside containers for the HF cache. Sets the `HF_HOME` environment variable. Defaults to `/.cache/huggingface`.
    *   `storageSize`: Size of the HF cache PVC (e.g., "50Gi"). Setting this enables the HF PVC.
    *   `preCacheRepos`: List of Hugging Face repository IDs and optional files to download into the cache volume using an init job.

Enabling `storage.data.download` or `storage.huggingFace.preCacheRepos` will cause Kaiwo to create a temporary Kubernetes Job (the "download job") before starting the main workload. This job runs a container (e.g., `ghcr.io/silogen/kaiwo-python:0.5`) that performs the downloads into the respective PVCs. The main workload only starts after the download job completes successfully.

#### `dangerous`

The `dangerous` field is a boolean flag. If set to `true`, Kaiwo will *not* add the default `PodSecurityContext` (which normally sets `runAsUser: 1000`, `runAsGroup: 1000`, `fsGroup: 1000`) to the generated pods. Use this only if you need to run containers as root or a different specific user and understand the security implications. Default is `false`.

### [KaiwoJob](../reference/crds/kaiwo.silogen.ai.md#kaiwojob)

A `KaiwoJob` represents a batch workload that runs to completion.

#### `clusterQueue`

The `clusterQueue` field specifies the name of the Kueue `ClusterQueue` that the job should be submitted to for scheduling and resource management.

*   This value is set as the `kueue.x-k8s.io/queue-name` label on the underlying Kubernetes Job or RayJob.
*   If omitted, it defaults to the value specified by the `DEFAULT_CLUSTER_QUEUE_NAME` environment variable in the Kaiwo controller (typically "kaiwo").
*   The `kaiwo submit` CLI command can override this using the `--queue` flag or the `clusterQueue` field in the `kaiwoconfig.yaml` file.

#### `priorityClass`

The `priorityClass` field specifies the name of a Kubernetes `PriorityClass` to be assigned to the job's pods. This influences the scheduling priority relative to other pods in the cluster.

#### `entrypoint`

The `entrypoint` field defines the command or script that the primary container in the job's pod(s) should execute.

*   It can be a multi-line string. Shell script shebangs (`#!/bin/bash`) are detected.
*   For standard Kubernetes Jobs (`ray: false`), this populates the `command` and `args` fields of the container spec (typically `["/bin/sh", "-c", "<entrypoint_script>"]`).
*   For RayJobs (`ray: true`), this populates the `rayJob.spec.entrypoint` field.
*   This overrides any default command specified in the container image or the underlying `job` or `rayJob` spec sections if they are also defined.

#### `rayJob`

The `rayJob` field allows you to provide a full `rayv1.RayJob` spec.

*   If this field is present (or if `spec.ray` is `true`), Kaiwo will create a `RayJob` resource instead of a standard `batchv1.Job`.
*   Common fields like `image`, `resources`, `gpus`, `replicas`, etc., will be merged into this spec, potentially overriding values defined here unless explicitly configured otherwise.
*   This provides fine-grained control over the Ray cluster configuration (head/worker groups) and Ray job submission parameters.

#### `job`

The `job` field allows you to provide a full `batchv1.Job` spec.

*   If this field is present and `spec.ray` is `false`, Kaiwo will use this as the base for the created `batchv1.Job`.
*   Common fields like `image`, `resources`, `gpus`, `entrypoint`, etc., will be merged into this spec, potentially overriding values defined here.
*   This provides fine-grained control over standard Kubernetes Job parameters like `backoffLimit`, `ttlSecondsAfterFinished`, pod template details, etc.

### [KaiwoService](../reference/crds/kaiwo.silogen.ai.md#kaiwoservice)

A `KaiwoService` represents a long-running service workload.

#### `clusterQueue`

Similar to `KaiwoJob`, this specifies the Kueue `ClusterQueue` name.

*   This value is set as the `kueue.x-k8s.io/queue-name` label on the underlying Kubernetes Deployment or the AppWrapper resource (which wraps the RayService).
*   If omitted, it defaults to the value specified by the `DEFAULT_CLUSTER_QUEUE_NAME` environment variable in the Kaiwo controller (typically "kaiwo").
*   The `kaiwo submit` CLI command can override this.

#### `priorityClass`

Specifies the Kubernetes `PriorityClass` for the service's pods, influencing scheduling priority.

#### `entrypoint`

Defines the command or script for the primary container in a standard Kubernetes `Deployment` (`ray: false`).

*   Similar to `KaiwoJob.spec.entrypoint`, it populates the container's `command`/`args`.
*   It is *not* used when `ray: true` (use `serveConfigV2` or the `rayService` spec instead for Ray entrypoints).

#### `serveConfigV2`

This field holds a Ray Serve configuration as a multi-line YAML string.

*   It is only used when `ray: true`.
*   The content is directly placed into the `rayService.spec.serveConfigV2` field of the generated RayService resource.
*   This is the standard way to define applications, deployments, and routing for Ray Serve.

#### `rayService`

The `rayService` field allows providing a full `rayv1.RayService` spec.

*   If present (or `spec.ray` is `true`), Kaiwo creates a `RayService` (wrapped in an AppWrapper for Kueue integration) instead of a `Deployment`.
*   Common fields are merged into the `RayClusterSpec` within this spec.
*   Allows fine-grained control over the Ray cluster and Ray Serve configurations.

#### `deployment`

The `deployment` field allows providing a full `appsv1.Deployment` spec.

*   If present and `spec.ray` is `false`, this is used as the base for the created `Deployment`.
*   Common fields are merged into this spec.
*   Allows fine-grained control over Kubernetes Deployment parameters (strategy, selectors, pod template, etc.).
