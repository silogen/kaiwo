# KAIWO Observability Stack

This directory contains the complete observability stack for KAIWO testing and debugging in vCluster environments.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Host Cluster                             │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  test-observability namespace                              │ │
│  │  ├── Grafana Loki (logs storage)                           │ │
│  │  ├── Prometheus (metrics storage)                          │ │
│  │  ├── Grafana (visualization + dashboards)                  │ │
│  │  └── Alloy DaemonSet (collects from all vClusters)         │ │
│  │      ├── GPU metrics (AMD/NVIDIA exporters)                │ │
│  │      ├── Node metrics (kubelet, cAdvisor)                  │ │
│  │      ├── vCluster control plane logs (syncer container)    │ │
│  │      ├── vCluster workload pod logs (synced to host)       │ │
│  │      └── K8s audit logs (from syncer stdout)               │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  vCluster Namespace (kaiwo-test-{run_id}-{attempt}-{inst}) │ │
│  │  ├── vCluster Control Plane Pod (test-monitoring-0)        │ │
│  │  │   └── syncer container: K8s API server + audit logs     │ │
│  │  ├── Synced workload pods (from vCluster)                  │ │
│  │  │   └── Labels: vcluster.loft.sh/managed-by               │ │
│  │  └── Namespace labels: vcluster_namespace, github_run_id   │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Key Architecture Changes (2025-11-10):**
- ✅ **Host-side collection**: Alloy runs ONLY on host cluster (not in vCluster)
- ✅ **Direct log collection**: Collects from vCluster syncer container and synced workload pods
- ✅ **No webhook**: Audit logs scraped from syncer stdout (K8s API server writes to `-`)
- ✅ **Label propagation**: Namespace labels automatically propagate to all logs/metrics
- ✅ **K8s distro required**: vCluster must use K8s distro (not K3s) for audit logging to work

## Components

### Host Cluster Components

1. **Loki** - Log aggregation and storage
   - Stores logs from all vClusters with proper labeling
   - 30-day retention
   - 50Gi persistent storage
   - Direct access endpoint (no gateway needed)

2. **Prometheus** - Metrics storage and querying
   - Stores metrics from vClusters and host
   - 30-day retention
   - 50Gi persistent storage

3. **Grafana** - Visualization and dashboards
   - Pre-configured with Loki and Prometheus datasources
   - NodePort service on port 30300
   - Default credentials: admin/admin
   - Auto-loads dashboards from ConfigMap

4. **Alloy (Host DaemonSet)** - Unified telemetry collector
   - **GPU Metrics**: Scrapes AMD/NVIDIA GPU exporters
   - **Node Metrics**: Collects kubelet and cAdvisor metrics
   - **vCluster Logs**: Discovers and collects logs from:
     - vCluster control plane pods (syncer container)
     - Workload pods synced to host (vcluster.loft.sh/managed-by label)
   - **Audit Logs**: Parses K3s audit JSON from syncer stdout
   - **Label Extraction**: Propagates namespace labels (github_run_id, etc.)

### vCluster Configuration

1. **Audit Policy** - Kubernetes API audit trail
   - ConfigMap: `vcluster-audit-policy` (deployed to host namespace)
   - Configured in vCluster's K8s API server (via controlPlane.distro.k8s.apiServer.extraArgs)
   - Audit logs written to stdout (audit-log-path=-)
   - Levels:
     - **RequestResponse**: KAIWO CRDs, core resources (pods, services, etc.)
     - **Metadata**: Everything else
   - Captured fields: verb, user, resource, request/response bodies

2. **Namespace Labels** - Metadata propagation
   - `vcluster-namespace`: vCluster instance identifier
   - `github-run-id`: GitHub Actions run ID
   - `github-run-attempt`: GitHub Actions run attempt
   - `installer`: Installation method (helm/kustomization/manual)
   - Labels automatically extracted and added to all logs/metrics

## Setup Instructions

**IMPORTANT**: ConfigMaps must be applied BEFORE helmfile to avoid pod startup failures.

### 1. Deploy Prerequisites (ConfigMaps)

```bash
# From the project root directory
cd dependencies

# Deploy the Alloy host configuration ConfigMap (MUST be done first)
kubectl apply -f monitoring/alloy-host-config.yaml

# Deploy Grafana dashboards ConfigMap (MUST be done first)
kubectl apply -k monitoring/grafana-dashboards/
```

### 2. Deploy Monitoring Stack to Host Cluster

```bash
# Deploy Loki, Grafana, and host Alloy to test-observability namespace
# NOTE: This uses Prometheus from the existing 'monitoring' namespace
helmfile -f monitoring-helmfile.yaml apply

# Wait for deployments to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=loki -n test-observability --timeout=5m
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n test-observability --timeout=5m
kubectl wait --for=condition=ready pod -l app=alloy-host -n test-observability --timeout=5m
```

### 3. Access Grafana

```bash
# Via NodePort (if accessible)
echo "Grafana URL: http://<node-ip>:30300"

# Or via port-forward
kubectl port-forward -n test-observability svc/kube-prometheus-stack-grafana 3000:80

# Default credentials: admin/admin
```

### 4. Deploy vCluster with Monitoring

**Automated (CI/CD):**
The `deploy-vcluster-audit.sh` script automatically:
1. Labels the vCluster namespace with metadata
2. Deploys audit policy ConfigMap

**Manual deployment:**

```bash
# Set environment variables
export VCLUSTER_NAMESPACE="kaiwo-test-manual-1-local"
export GITHUB_RUN_ID="local"
export GITHUB_RUN_ATTEMPT="1"
export INSTALLER="manual"

# Create namespace
kubectl create namespace "$VCLUSTER_NAMESPACE"

# Deploy audit components (labels namespace + audit policy)
bash ./dependencies/deploy-vcluster-audit.sh

# Create vCluster with audit logging enabled
vcluster create test-monitoring \
  --namespace "$VCLUSTER_NAMESPACE" \
  --values dependencies/vcluster.yaml
```

**No additional steps needed!** The host Alloy DaemonSet automatically discovers and collects from the new vCluster.

## Label Strategy

All data is labeled with the following labels for filtering and correlation:

### Common Labels (All Logs/Metrics)

- `vcluster_namespace`: Host cluster namespace where vCluster runs (e.g., `kaiwo-test-123-1-helm`)
- `github_run_id`: GitHub Actions run ID (e.g., `12345`)
- `github_run_attempt`: GitHub Actions run attempt number (e.g., `1`)
- `installer`: Installation method (`kustomization`, `helm`, or `manual`)
- `cluster`: Always `vcluster` for vCluster data

### Log-Specific Labels

- `log_type`: Type of log entry
  - `pod_log`: Container logs from workload pods
  - `k8s_audit_log`: API server audit logs
- `namespace`: Kubernetes namespace within vCluster (for pod logs)
- `workload_namespace`: Original vCluster namespace (for synced workload pods)
- `pod`: Pod name
- `container`: Container name
- `app`: App label from pod

### Audit Log Labels (Parsed from JSON)

- `verb`: API operation (`create`, `update`, `delete`, `get`, `list`, `patch`, `watch`)
- `resource`: Kubernetes resource type (e.g., `pods`, `deployments`, `aims`)
- `namespace`: Namespace within vCluster
- `stage`: Audit stage (always `ResponseComplete`)

## Grafana Dashboards

### 1. KAIWO Namespace Debug Dashboard

**UID:** `kaiwo-namespace-debug`

Comprehensive debugging view for a specific namespace:

- **Pod Logs**: Live streaming logs with search
- **Kubernetes Events**: Pod lifecycle events, warnings, errors
- **Pod Status**: Timeline of pod phases (Pending, Running, Failed)
- **Resource Usage**: CPU and memory usage per pod
- **API Audit Log**: Kubernetes API mutations (create, update, delete)

**Usage:**
1. Select vCluster namespace from dropdown
2. Select test namespace from dropdown
3. Use search box to filter logs
4. Investigate timeline of events, pod status, and resource usage

### 2. KAIWO Cluster Health Dashboard

**UID:** `kaiwo-cluster-health`

Cluster-wide health and resource overview:

- **Node Statistics**: Total nodes, pods, failed/pending pods
- **Node Metrics**: CPU and memory usage per node
- **Pod Distribution**: Pods per node
- **GPU Utilization**: GPU usage (NVIDIA and AMD)

**Usage:**
1. Select vCluster namespace from dropdown
2. View overall cluster health
3. Identify resource bottlenecks
4. Correlate GPU usage with test failures

## LogQL Query Examples

### Find all errors in a namespace

```logql
{namespace="chainsaw-test-123", vcluster_namespace="kaiwo-test-456-1-helm", log_type="pod_log"} |~ "(?i)error|exception|fatal"
```

### View API operations on KAIWO CRDs

```logql
{vcluster_namespace="kaiwo-test-456-1-helm", log_type="k8s_audit_log", resource=~"aims|rayservices|appwrappers"}
```

### View full request/response for create operations

```logql
{vcluster_namespace="kaiwo-test-456-1-helm", log_type="k8s_audit_log", verb="create"} | json
```

### Filter audit logs by specific resource

```logql
{vcluster_namespace="kaiwo-test-456-1-helm", log_type="k8s_audit_log", resource="pods", verb="create"}
```

### Filter by GitHub run

```logql
{github_run_id="12345", github_run_attempt="1", log_type="pod_log"}
```

## PromQL Query Examples

### Pod restart count

```promql
sum(kube_pod_container_status_restarts_total{vcluster_namespace="kaiwo-test-456-1-helm"}) by (namespace, pod)
```

### Operator reconciliation rate

```promql
rate(controller_runtime_reconcile_total{vcluster_namespace="kaiwo-test-456-1-helm", controller="aim"}[5m])
```

### Failed reconciliations

```promql
sum(rate(controller_runtime_reconcile_errors_total{vcluster_namespace="kaiwo-test-456-1-helm"}[5m])) by (controller)
```

### Kueue workload admission latency

```promql
histogram_quantile(0.95,
  rate(kueue_admission_attempt_duration_seconds_bucket{vcluster_namespace="kaiwo-test-456-1-helm"}[5m])
)
```

## Audit Logging Details

### What Gets Audited?

See `vcluster-audit-policy.yaml` for the full policy:

**RequestResponse level (full request/response bodies):**
- **KAIWO CRDs**: `kaiwo.silogen.ai/*`, `aim.silogen.ai/*`
- **Core resources**: `pods`, `services`, `configmaps`, `secrets`, `persistentvolumeclaims`
- **Apps**: `deployments`, `statefulsets`, `daemonsets`, `replicasets`
- **KServe**: `serving.kserve.io/*`
- **Ray**: `ray.io/rayclusters`, `rayservices`, `rayjobs`
- **Kueue**: `kueue.x-k8s.io/*`
- **AppWrappers**: `workload.codeflare.dev/appwrappers`

**Metadata level (just metadata, no bodies):**
- Jobs, CronJobs
- Events

**Excluded (not logged):**
- System component read operations
- Lease operations (high volume)
- Health checks

### Example Audit Log

```json
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "RequestResponse",
  "verb": "create",
  "user": {"username": "system:admin"},
  "objectRef": {
    "resource": "configmaps",
    "namespace": "default",
    "name": "test-config"
  },
  "requestObject": {
    "kind": "ConfigMap",
    "data": {"key1": "value1"}
  },
  "responseObject": {
    "kind": "ConfigMap",
    "metadata": {"uid": "...", "resourceVersion": "123"},
    "data": {"key1": "value1"}
  }
}
```

## Troubleshooting

### No logs appearing in Loki

1. Check Alloy is running on host:
   ```bash
   kubectl get pods -n test-observability -l app=alloy-host
   kubectl logs -n test-observability -l app=alloy-host -f
   ```

2. Verify vCluster pods are labeled correctly:
   ```bash
   kubectl get pods -n "$VCLUSTER_NAMESPACE" --show-labels | grep vcluster
   ```

3. Check namespace labels:
   ```bash
   kubectl get namespace "$VCLUSTER_NAMESPACE" --show-labels
   ```

4. Query Loki directly:
   ```bash
   kubectl exec -n test-observability loki-0 -c loki -- \
     wget -q -O- 'http://localhost:3100/loki/api/v1/label/vcluster_namespace/values'
   ```

### Audit logs not appearing

1. Verify audit policy ConfigMap exists:
   ```bash
   kubectl get configmap vcluster-audit-policy -n "$VCLUSTER_NAMESPACE"
   ```

2. Check vCluster syncer logs for audit JSON:
   ```bash
   kubectl logs -n "$VCLUSTER_NAMESPACE" -l app=vcluster -c syncer | grep '{"kind":"Event"'
   ```

3. Check Alloy is parsing audit logs:
   ```bash
   kubectl logs -n test-observability -l app=alloy-host | grep audit
   ```

4. Query for audit log type:
   ```bash
   kubectl exec -n test-observability loki-0 -c loki -- \
     wget -q -O- 'http://localhost:3100/loki/api/v1/label/log_type/values'
   # Should include: k8s_audit_log
   ```

### Dashboard shows "No data"

1. Verify datasource configuration in Grafana:
   - Settings → Data Sources → Loki/Prometheus
   - Test connection

2. Check label filters match your vCluster:
   ```logql
   {vcluster_namespace=~".*"}  # Should show available vcluster_namespace labels
   ```

3. Adjust time range - data retention is 30 days

## Data Retention

- **Loki**: 720 hours (30 days)
- **Prometheus**: 30 days
- **Grafana dashboards**: Persistent

After vCluster deletion, historical data remains in Loki/Prometheus for 30 days, queryable by `vcluster_namespace`, `github_run_id`, etc.

## Resource Usage

**Host Cluster Resources:**
- Loki: 500m-2 CPU, 1-4Gi RAM, 50Gi storage
- Prometheus: 500m-2 CPU, 2-8Gi RAM, 50Gi storage
- Grafana: 250m-1 CPU, 512Mi-2Gi RAM, 10Gi storage
- Alloy (host): 100m-500m CPU, 256Mi-1Gi RAM per node

**vCluster Overhead:** None! (monitoring runs on host)

## Security Considerations

1. **Grafana Access**: Change default admin password in production
2. **Audit Logs**: Contain sensitive API request/response data including secrets
3. **Label Injection**: Labels are trusted input from CI environment
4. **Access Control**: Consider RBAC for Grafana datasource access

## Files in This Directory

### Active Files
- `README.md` - This file
- `QUICK_START.md` - Quick setup guide
- `DEPLOYMENT_NOTES.md` - Deployment history and troubleshooting
- `AUDIT_LOGGING_SETUP.md` - **OUTDATED** (describes webhook approach)
- `alloy-host-config.yaml` - Alloy configuration for host cluster ✅
- `vcluster-audit-policy.yaml` - K8s audit policy ✅
- `grafana-dashboards/` - Dashboard JSON files ✅
- `grafana-dashboards/kustomization.yaml` - Dashboard deployment config ✅

### Deprecated Files (No Longer Used)
- `alloy-audit-receiver.yaml` - ❌ Webhook receiver (replaced by stdout scraping)
- `alloy-vcluster-config.yaml` - ❌ Alloy config for inside vCluster (no longer deployed)
- `alloy-vcluster-daemonset.yaml` - ❌ Alloy DaemonSet for vCluster (no longer deployed)
- `vcluster-audit-webhook.yaml` - ❌ Audit webhook config (no longer used)
- `kube-state-metrics.yaml` - ❌ Separate deployment (using host Prometheus stack)
- `grafana-dashboards-configmap.yaml` - ❌ Manual ConfigMap (using kustomize)
- `test-workloads.yaml` - ❌ Testing only

## Further Reading

- [Grafana Loki Documentation](https://grafana.com/docs/loki/latest/)
- [Grafana Alloy Documentation](https://grafana.com/docs/alloy/latest/)
- [Kubernetes Audit Logging](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- [vCluster Documentation](https://www.vcluster.com/docs)
