# Troubleshooting Guide

This guide provides steps for diagnosing common issues with Kaiwo.

## Operator Issues

### Operator Pod Not Running or Crashing

1.  **Check Pod Status**:
    ```bash
    kubectl get pods -n kaiwo-system -l control-plane=kaiwo-controller-manager
    ```
    Look for pods in `CrashLoopBackOff`, `Error`, or `Pending` states.

2.  **Examine Pod Logs**:
    ```bash
    kubectl logs -n kaiwo-system -l control-plane=kaiwo-controller-manager
    # Or for a specific crashing pod:
    kubectl logs -n kaiwo-system <pod-name> -p # Check previous container logs
    ```
    Look for error messages related to startup, configuration, API connectivity, or reconciliation loops.

3.  **Describe Pod**:
    ```bash
    kubectl describe pod -n kaiwo-system <pod-name>
    ```
    Check for events related to scheduling failures (resource constraints, taints/tolerations), image pull errors, readiness/liveness probe failures, or volume mount issues.

4.  **Check Dependencies**: Ensure all dependencies (Cert-Manager, Kueue, Ray Operator, GPU Operator, AppWrapper) are running correctly in their respective namespaces. Check their logs if necessary.

5.  **RBAC Permissions**: Verify the Kaiwo operator's `ServiceAccount`, `ClusterRole`, and `ClusterRoleBinding` grant sufficient permissions. Errors related to "forbidden" access often point to RBAC issues.

6.  **Webhook Issues**: If webhooks are enabled, check Cert-Manager status and webhook service connectivity. Invalid certificates or network policies blocking webhook calls can prevent resource creation/updates.
    *   Check webhook configurations: `kubectl get mutatingwebhookconfigurations`, `kubectl get validatingwebhookconfigurations`
    *   Check certificate status: `kubectl get certificates -n kaiwo-system`
    *   Test webhook service endpoint.

### Default `KaiwoQueueConfig` Not Created

*   Check operator logs (`kubectl logs -n kaiwo-system -l control-plane=kaiwo-controller-manager`) for errors during the startup routine that creates the default configuration.
*   Common causes include inability to list Nodes (RBAC issue) or errors during node labeling/tainting if enabled.

### Kueue Resources Not Syncing

*   Ensure the `kaiwo` `KaiwoQueueConfig` resource exists (`kubectl get kaiwoqueueconfig kaiwo`).
*   Check operator logs for errors related to creating/updating Kueue `ResourceFlavors`, `ClusterQueues`, or `WorkloadPriorityClasses`.
*   Verify the operator has RBAC permissions to manage these Kueue resources.
*   Check Kueue controller logs (`kubectl logs -n kueue-system -l control-plane=controller-manager -f`) for related errors.

## Workload Issues

### Workload Stuck in `PENDING`

This usually means Kueue has not admitted the workload yet.

1.  **Check Kueue Workload Status**: Find the Kueue `Workload` resource corresponding to your `KaiwoJob`/`KaiwoService`.
    ```bash
    # Find the workload (often named after the Kaiwo resource)
    kubectl get workloads -n <namespace>
    # Describe the workload to see admission status and reasons for pending
    kubectl describe workload -n <namespace> <workload-name>
    ```
    Look for conditions like `Admitted` being `False` and check the `Message` for reasons (e.g., quota exhaustion, no matching `ResourceFlavor`).

2.  **Check ClusterQueue Status**:
    ```bash
    kubectl describe clusterqueue <queue-name>
    ```
    Look at usage vs. quota (`nominalQuota`) for relevant resource flavors.

3.  **Check ResourceFlavor Definitions**: Ensure `ResourceFlavors` defined in `KaiwoQueueConfig` correctly match node labels in your cluster.

4.  **Check LocalQueue**: Ensure a `LocalQueue` pointing to the correct `ClusterQueue` exists in the workload's namespace (`kubectl get localqueue -n <namespace> <queue-name>`). Kaiwo operator should create these if specified in `KaiwoQueueConfig.spec.clusterQueues[].namespaces`.

### Workload Fails Immediately (Status `FAILED`)

1.  **Check Kaiwo Resource Events**:
    ```bash
    kubectl describe kaiwojob -n <namespace> <job-name>
    # or
    kubectl describe kaiwoservice -n <namespace> <service-name>
    ```
    Look for events indicating failures during dependency creation (e.g., PVC, download job) or underlying resource creation.

2.  **Check Download Job Logs (if applicable)**: If using `spec.storage` with downloads, check the logs of the downloader job pod.
    ```bash
    # Find the downloader pod (usually name ends with '-download-<hash>')
    kubectl get pods -n <namespace> | grep <job-name>-download
    # Get logs
    kubectl logs -n <namespace> <downloader-pod-name>
    ```
    Look for errors related to accessing storage secrets, connecting to S3/GCS/Git, or filesystem permissions.

3.  **Check Underlying Resource Events/Logs**:
    *   For `KaiwoJob` -> `BatchJob`: `kubectl describe job -n <namespace> <job-name>` and check pod events/logs.
    *   For `KaiwoJob` -> `RayJob`: `kubectl describe rayjob -n <namespace> <job-name>` and check Ray cluster/pod events/logs.
    *   For `KaiwoService` -> `Deployment`: `kubectl describe deployment -n <namespace> <service-name>` and check pod events/logs.
    *   For `KaiwoService` -> `RayService`: `kubectl describe rayservice -n <namespace> <service-name>` and check Ray cluster/pod events/logs.

### Pods Not Scheduling / Stuck in `Pending`

This occurs after Kueue admits the workload but before Kubernetes schedules the pod(s).

1.  **Describe Pod**:
    ```bash
    kubectl describe pod -n <namespace> <pod-name>
    ```
    Check the `Events` section for messages from the scheduler (e.g., `FailedScheduling`). Common reasons include:
    *   **Insufficient Resources**: Not enough CPU, memory, or GPUs available on any node.
    *   **Node Affinity/Selector Mismatch**: Pod requires labels that no node possesses (often related to `ResourceFlavor` `nodeLabels`).
    *   **Taint/Toleration Mismatch**: Pod lacks tolerations for taints present on suitable nodes (e.g., GPU taint). Kaiwo should add GPU tolerations automatically if GPUs are requested.
    *   **PVC Binding Issues**: If using `storage`, check if the `PersistentVolumeClaim` is stuck in `Pending` (`kubectl get pvc -n <namespace>`). This could be due to no available `PersistentVolume` or StorageClass issues.

### Pods Crashing / `CrashLoopBackOff`

1.  **Check Pod Logs**: This is the most important step.
    ```bash
    kubectl logs -n <namespace> <pod-name>
    kubectl logs -n <namespace> <pod-name> -p # Previous container instance logs
    ```
    Look for application errors, missing files, permission issues, OOMKilled errors, GPU driver/runtime errors.

2.  **Describe Pod**: Check events for reasons like OOMKilled.

3.  **Exec into Pod (if possible)**: Use `kaiwo exec` or `kubectl exec` to inspect the container environment.
    ```bash
    kaiwo exec job/<job-name> --command "/bin/bash" -n <namespace>
    # or
    kubectl exec -it -n <namespace> <pod-name> -- /bin/bash
    ```

## Developer Debugging (`kaiwo-dev`)

!!! info
    This feature is only intended for contributors

The `kaiwo-dev` tool (built separately from the main CLI/operator) provides debugging utilities.

*   **`kaiwo-dev debug chainsaw`**: Helps debug Kyverno Chainsaw E2E tests by collecting and correlating logs and events from a specific test namespace.

    ```bash
    # Build the tool: go build -o bin/kaiwo-dev ./pkg/cli/dev/main.go
    ./bin/kaiwo-dev debug chainsaw -n <test-namespace> [--print-level <debug|info|warn|error>]
    ```
    This command gathers Kaiwo controller logs relevant to the namespace, pod logs within the namespace, and Kubernetes events, sorts them chronologically, and prints them with color-coding. Useful for understanding the sequence of events during a failed test run.
