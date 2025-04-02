# Authentication & Authorization

Kaiwo integrates with Kubernetes authentication and authorization mechanisms.

## Cluster queue namespaces

Since the **`KaiwoQueueConfig`** lists the namespaces for the cluster queues, you must make sure that users that use these queues have the correct RBAC rights for these namespaces.

## User Authentication (CLI)

If you wish to set up authentication on the Kube API server, please follow the [official documentation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#configuring-the-api-server). For setting up the `kaiwo` CLI to use this authentication, please see the [CLI documentation](../scientist/cli.md).

## Authorization (RBAC)

Access control within Kubernetes, including permissions to create, view, and manage Kaiwo resources (`KaiwoJob`, `KaiwoService`, etc.) and underlying resources (Pods, Jobs, Deployments, PVCs), is managed through Kubernetes Role-Based Access Control (RBAC).

*   **Kaiwo Operator Permissions**: The `install.yaml` manifest includes `ClusterRole` and `ClusterRoleBinding` (or `Role`/`RoleBinding`) definitions granting the Kaiwo operator's Service Account the necessary permissions to manage resources across the cluster or within specific namespaces. These permissions include creating/updating/deleting Jobs, Deployments, Ray resources, Kueue resources, PVCs, ConfigMaps, etc. Review the RBAC rules in `config/rbac/` in the source repository for specifics.
*   **User Permissions**: As an administrator, you need to grant users appropriate RBAC permissions to interact with Kaiwo resources. Users typically need:
    *   `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` permissions on `kaiwojobs.kaiwo.silogen.ai` and `kaiwoservices.kaiwo.silogen.ai` within their target namespaces.
    *   `get`, `list`, `watch` permissions on Pods, Services, Events within their namespaces to allow `kaiwo manage`, `logs`, `monitor`, `exec`.
    *   Permissions to create `PersistentVolumeClaims` if they use the `storage` feature.
    *   Permissions to access `Secrets` referenced in their manifests (e.g., for image pulls, data download credentials).

**Example User Role:**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: ai-project-ns # Target namespace for the user
  name: kaiwo-scientist-role
rules:
- apiGroups: ["kaiwo.silogen.ai"]
  resources: ["kaiwojobs", "kaiwoservices"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""] # Core API group
  resources: ["pods", "pods/log", "services", "events", "persistentvolumeclaims", "configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods/exec"] # For kaiwo exec/monitor
  verbs: ["create"]
- apiGroups: ["batch"]
  resources: ["jobs"] # To view underlying batch jobs
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments"] # To view underlying deployments
  verbs: ["get", "list", "watch"]
- apiGroups: ["ray.io"] # If using Ray
  resources: ["rayjobs", "rayservices", "rayclusters"]
  verbs: ["get", "list", "watch"]
# Add other permissions as needed (e.g., create PVCs)
```

Bind this role to specific users or groups using a `RoleBinding`.
