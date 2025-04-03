# Maintenance Guide

This section outlines common maintenance tasks for the Kaiwo system.

## Upgrading Kaiwo

1.  **Check Release Notes**: Before upgrading, review the release notes for the target version on the [GitHub Releases page](https://github.com/silogen/kaiwo/releases). Pay attention to any breaking changes, dependency updates, or manual migration steps required.
2.  **Backup (Optional but Recommended)**: Consider backing up relevant configurations, especially your `KaiwoQueueConfig` CRD (`kubectl get kaiwoqueueconfig kaiwo -o yaml > kaiwoqueueconfig_backup.yaml`).
3.  **Apply New Manifest**: Apply the `install.yaml` manifest for the new version using `kubectl apply --server-side`.

    ```bash
    export KAIWO_NEW_VERSION=vX.Y.Z # Set to the target version
    kubectl apply -f https://github.com/silogen/kaiwo/releases/download/${KAIWO_NEW_VERSION}/install.yaml --server-side
    ```
4.  **Verify Upgrade**:

    *   Check that the Kaiwo operator pod restarts and uses the new image version:
        ```bash
        kubectl get pods -n kaiwo-system -l control-plane=kaiwo-controller-manager
        kubectl describe pod -n kaiwo-system -l control-plane=kaiwo-controller-manager | grep Image:
        ```
    *   Monitor operator logs for any errors during startup:
        ```bash
        kubectl logs -n kaiwo-system -l control-plane=kaiwo-controller-manager -f
        ```
    *   Ensure Kaiwo CRDs remain functional and workloads continue to be processed.
5.  **Upgrade Dependencies**: If the Kaiwo release notes indicate required upgrades for dependencies (Kueue, Ray, Cert-Manager, etc.), perform those upgrades according to their respective documentation.

## Certificate Rotation

*   **Cert-Manager (Default)**: If using the default setup with Cert-Manager, certificate rotation for webhooks is typically handled automatically based on the `Certificate` resources created during installation. Monitor Cert-Manager logs and certificate expiry (`kubectl get certificates -n kaiwo-system`) if issues arise.

## Operator Pod Management

*   **Restarting**: If the operator becomes unresponsive, you can restart it by deleting the pod:
    ```bash
    kubectl delete pod -n kaiwo-system -l control-plane=kaiwo-controller-manager
    ```
    The Deployment will automatically create a new pod.
*   **Scaling**: The Kaiwo operator deployment typically runs with a single replica due to leader election (`--leader-elect=true`). Scaling is generally not required unless leader election is disabled (not recommended for production).

## Cleaning Up Resources

*   **Workloads**: Users can delete their workloads using `kaiwo manage` or `kubectl delete kaiwojob <name>` / `kubectl delete kaiwoservice <name>`. The Kaiwo operator's finalizers ensure associated resources (like underlying Jobs/Deployments, PVCs created by Kaiwo download jobs) are cleaned up.
*   **Kueue Resources**: Resources managed by `KaiwoQueueConfig` (Flavors, ClusterQueues, PriorityClasses) are typically deleted if removed from the `kaiwo` `KaiwoQueueConfig` spec.
*   **Uninstalling Kaiwo**: To completely remove Kaiwo, delete the installation manifest and clean up CRDs and namespaces:
    ```bash
    # Replace vX.Y.Z with the installed version
    export KAIWO_VERSION=vX.Y.Z
    kubectl delete -f https://github.com/silogen/kaiwo/releases/download/${KAIWO_VERSION}/install.yaml

    # Delete CRDs (use with caution - this will delete ALL KaiwoJob/Service/QueueConfig resources)
    kubectl delete crd kaiwojobs.kaiwo.silogen.ai
    kubectl delete crd kaiwoservices.kaiwo.silogen.ai
    kubectl delete crd kaiwoqueueconfigs.kaiwo.silogen.ai
    # ... delete other Kaiwo CRDs if any

    # Delete namespace
    kubectl delete namespace kaiwo-system
    ```
    Remember to also uninstall dependencies if they are no longer needed.
