# Authentication & Authorization

Kaiwo integrates with Kubernetes authentication and authorization mechanisms.

## Cluster queue namespaces

Since the **`KaiwoQueueConfig`** lists the namespaces for the cluster queues, you must make sure that users that use these queues have the correct RBAC rights for these namespaces.

## User Authentication (CLI)

The `kaiwo` CLI tool interacts with the Kubernetes API server using credentials configured in a `kubeconfig` file.

*   **Kubeconfig Location**: Like `kubectl`, Kaiwo searches for the kubeconfig file in this order:
    1.  Path specified by the `KUBECONFIG` environment variable.
    2.  Default path: `~/.kube/config`.
*   **Authentication Methods**: Kaiwo supports standard kubeconfig authentication methods, including:
    *   Tokens (Service Account tokens, OIDC tokens)
    *   Client Certificates
    *   Username/Password (generally discouraged)
    *   **Exec Plugins**: For integration with external identity providers (IdPs).

### Exec Plugins for External IdPs

If your cluster uses an external authentication provider (like OIDC with Keycloak, Dex, Okta, or cloud provider IAM like Azure AD, GKE Workload Identity), users will need an appropriate `exec` plugin configured in their kubeconfig. Kaiwo works with these standard plugins.

**Common Plugins:**

*   **OIDC:** [int128/kubelogin](https://github.com/int128/kubelogin) (provides `kubectl oidc-login`) is a popular choice.
*   **Azure:** [Azure Kubelogin](https://azure.github.io/kubelogin/index.html)
*   **GKE:** [gke-gcloud-auth-plugin](https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke) (often installed with `gcloud`)

**Example Kubeconfig with `kubelogin`:**

```yaml
apiVersion: v1
clusters:
-   cluster:
        server: https://your-kube-api-endpoint:6443
        # certificate-authority-data: LS0tLS... # Optional: CA cert
        # insecure-skip-tls-verify: true       # Use with caution!
    name: my-cluster
contexts:
-   context:
        cluster: my-cluster
        user: my-user@oidc
    name: my-context
current-context: my-context
kind: Config
preferences: {}
users:
-   name: my-user@oidc
    user:
        exec:
            apiVersion: client.authentication.k8s.io/v1beta1
            command: kubelogin # Assumes kubelogin binary is in PATH
            args:
            - get-token
            - --oidc-issuer-url=https://your.oidc.provider/auth/realms/myrealm
            - --oidc-client-id=kubernetes-client
            # - --oidc-client-secret=... # If required by provider
            # - --oidc-extra-scope=groups,profile # Request specific scopes
            # - --token-cache-dir=~/.kube/cache/oidc-login # Cache location
            # - --insecure-skip-tls-verify # Use with caution!
            interactiveMode: IfAvailable # Or Always / Never
            provideClusterInfo: false
```

Users need to install the relevant plugin (e.g., `kubelogin`) and potentially perform an initial login (`kubelogin login --oidc-...`) before using `kaiwo` or `kubectl`.

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
