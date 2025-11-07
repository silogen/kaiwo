# KAIWO Observability Stack

This directory contains the complete observability stack for KAIWO testing and debugging in vCluster environments.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Host Cluster                             │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  monitoring namespace                                      │ │
│  │  ├── Grafana Loki (logs storage)                           │ │
│  │  ├── Prometheus (metrics storage)                          │ │
│  │  ├── Grafana (visualization)                               │ │
│  │  └── Alloy DaemonSet (GPU & node metrics collection)       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  vCluster Namespace (kaiwo-test-{run_id}-{attempt}-{inst} )│ │
│  │  └── vCluster Control Plane ─────────────────────────────-─┼─┼─┐
│  └────────────────────────────────────────────────────────────┘ │ │
└───────────────────────────────────────────────────────────────────┘ │
                                                                      │
    ┌─────────────────────────────────────────────────────────────────┘
    │
    │  ┌────────────────────────────────────────────────────────────┐
    └──│                    vCluster (Virtual)                      │
       │  ┌──────────────────────────────────────────────────────┐ │
       │  │  Alloy DaemonSet                                     │ │
       │  │  ├── Pod logs collection (all namespaces)           │ │
       │  │  ├── K8s events collection                          │ │
       │  │  ├── API audit log webhook receiver                 │ │
       │  │  ├── Operator metrics scraping                      │ │
       │  │  ├── Kube-state-metrics scraping                    │ │
       │  │  ├── KubeRay metrics scraping                       │ │
       │  │  ├── Kueue metrics scraping                         │ │
       │  │  └── Ray cluster metrics scraping                   │ │
       │  │                                                        │ │
       │  │  All data → Remote write to host Loki/Prometheus     │ │
       │  └──────────────────────────────────────────────────────┘ │
       │                                                            │
       │  ┌──────────────────────────────────────────────────────┐ │
       │  │  kube-state-metrics (in kube-system)                 │ │
       │  │  Exposes K8s resource metrics (pods, deployments...)  │ │
       │  └──────────────────────────────────────────────────────┘ │
       └────────────────────────────────────────────────────────────┘
```

## Components

### Host Cluster Components

1. **Loki** - Log aggregation and storage
   - Stores logs from all vClusters with proper labeling
   - 30-day retention
   - 50Gi persistent storage

2. **Prometheus** - Metrics storage and querying
   - Stores metrics from vClusters and host
   - 30-day retention
   - 50Gi persistent storage

3. **Grafana** - Visualization and dashboards
   - Pre-configured with Loki and Prometheus datasources
   - NodePort service on port 30300
   - Default credentials: admin/admin

4. **Alloy (Host)** - Telemetry collector for host
   - Scrapes GPU exporters (AMD OpenTelemetry, NVIDIA DCGM)
   - Collects host node metrics (kubelet, cAdvisor)
   - Writes to local Prometheus

### vCluster Components

1. **Alloy (vCluster)** - Telemetry collector for vCluster
   - Collects pod logs from all namespaces
   - Collects Kubernetes events
   - Receives audit logs from API server webhook
   - Scrapes operator, Kueue, KubeRay, Ray cluster metrics
   - Remote writes to host Loki/Prometheus with labels

2. **kube-state-metrics** - Kubernetes resource metrics
   - Deployed in kube-system namespace
   - Exposes metrics about pods, deployments, CRDs, etc.

3. **Audit Logging** - Kubernetes API audit trail
   - Configured in vCluster API server
   - Sends audit events to Alloy via webhook
   - Captures KAIWO CRD operations, pod lifecycle, etc.

## Setup Instructions

### 1. Deploy Monitoring Stack to Host Cluster

```bash
# From the project root directory
cd dependencies

# Deploy Loki, Prometheus, Grafana, and host Alloy to the host cluster
helmfile -f monitoring-helmfile.yaml apply

# Deploy the Alloy host configuration ConfigMap
kubectl apply -f monitoring/alloy-host-config.yaml -n monitoring

# Wait for deployments to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=loki -n monitoring --timeout=5m
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=prometheus -n monitoring --timeout=5m
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n monitoring --timeout=5m
```

### 2. Deploy Grafana Dashboards

```bash
# Create the dashboard ConfigMap
kubectl create configmap grafana-dashboards-kaiwo \
  --from-file=monitoring/grafana-dashboards/ \
  -n monitoring \
  --dry-run=client -o yaml | kubectl apply -f -

# Label it for Grafana to discover
kubectl label configmap grafana-dashboards-kaiwo grafana_dashboard=1 -n monitoring
```

### 3. Access Grafana

```bash
# Via NodePort (if accessible)
echo "Grafana URL: http://<node-ip>:30300"

# Or via port-forward
kubectl port-forward -n monitoring svc/kube-prometheus-stack-grafana 3000:80

# Default credentials: admin/admin
```

### 4. Deploy vCluster Monitoring (Automatic via CI)

When vCluster is created via GitHub Actions, monitoring is automatically deployed via the `deploy-vcluster-monitoring.sh` script.

**Manual deployment:**

```bash
# Ensure you're in the vCluster context
kubectl config use-context vcluster_<vcluster-name>

# Set environment variables
export VCLUSTER_NAMESPACE="kaiwo-test-123-1-helm"  # Host namespace where vCluster runs
export GITHUB_RUN_ID="local"
export GITHUB_RUN_ATTEMPT="1"
export INSTALLER="manual"

# Deploy monitoring
bash ./dependencies/deploy-vcluster-monitoring.sh
```

### 5. Deploy Audit Policy to vCluster Namespace (Host)

The audit policy ConfigMaps must be deployed to the **host cluster** in the vCluster namespace before vCluster starts:

```bash
# Switch to host kubeconfig
export KUBECONFIG=/path/to/host-kubeconfig

# Deploy audit ConfigMaps to vCluster namespace
export VCLUSTER_NAMESPACE="kaiwo-test-123-1-helm"
kubectl apply -f monitoring/vcluster-audit-policy.yaml -n $VCLUSTER_NAMESPACE
kubectl apply -f monitoring/vcluster-audit-webhook.yaml -n $VCLUSTER_NAMESPACE

# Update the webhook config with correct namespace
cat monitoring/vcluster-audit-webhook.yaml | \
  sed "s/\${NAMESPACE}/$VCLUSTER_NAMESPACE/g" | \
  kubectl apply -f - -n $VCLUSTER_NAMESPACE
```

## Label Strategy

All data is labeled with the following labels for filtering and correlation:

### Common Labels

- `vcluster_namespace`: Host cluster namespace where vCluster runs (e.g., `kaiwo-test-123-1-helm`)
- `github_run_id`: GitHub Actions run ID
- `github_run_attempt`: GitHub Actions run attempt number
- `installer`: Installation method (`kustomization` or `helm`)

### Log-Specific Labels

- `log_type`: Type of log entry
  - `pod_log`: Container logs from pods
  - `k8s_event`: Kubernetes events
  - `k8s_audit_log`: API server audit logs
- `namespace`: Kubernetes namespace within vCluster
- `pod`: Pod name
- `container`: Container name

### Metric-Specific Labels

- `job`: Scrape job name (e.g., `kaiwo-operator`, `kueue`, `kuberay`)
- `namespace`: Kubernetes namespace within vCluster
- `pod`: Pod name

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

### Find pod failures

```logql
{vcluster_namespace="kaiwo-test-456-1-helm", log_type="k8s_event"} |~ "(?i)failed|error|crash|oom"
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

## Troubleshooting

### No logs appearing in Loki

1. Check Alloy is running in vCluster:
   ```bash
   kubectl get pods -l app=alloy-vcluster
   kubectl logs -l app=alloy-vcluster -f
   ```

2. Verify remote write endpoint is accessible:
   ```bash
   kubectl exec -it <alloy-pod> -- wget -O- http://loki-gateway.monitoring.svc.cluster.local/ready
   ```

3. Check Alloy configuration:
   ```bash
   kubectl get configmap alloy-vcluster-config -o yaml
   ```

### No metrics appearing in Prometheus

1. Check ServiceMonitor is deployed:
   ```bash
   kubectl get servicemonitor -A
   ```

2. Verify operator metrics endpoint:
   ```bash
   kubectl port-forward -n kaiwo-system svc/controller-manager-metrics-service 8443:8443
   curl -k https://localhost:8443/metrics
   ```

3. Check Alloy scrape targets:
   ```bash
   kubectl logs -l app=alloy-vcluster | grep -i "scrape"
   ```

### Audit logs not appearing

1. Verify audit policy ConfigMap exists in vCluster namespace (host):
   ```bash
   kubectl get configmap vcluster-audit-policy -n <vcluster-namespace>
   ```

2. Check vCluster API server logs (host):
   ```bash
   kubectl logs -n <vcluster-namespace> -l app=vcluster -c syncer | grep audit
   ```

3. Verify Alloy webhook endpoint is accessible from vCluster:
   ```bash
   # From within vCluster
   kubectl run -it --rm test --image=curlimages/curl -- \
     curl -v http://alloy-vcluster.<namespace>.svc.cluster.local:8080/audit
   ```

### Dashboard shows "No data"

1. Verify datasource configuration in Grafana:
   - Settings → Data Sources → Loki/Prometheus
   - Test connection

2. Check label filters match your vCluster:
   ```bash
   # In Grafana Explore
   {vcluster_namespace=~".*"}  # Should show available vcluster_namespace labels
   ```

3. Adjust time range - data retention is 30 days

## Data Retention

- **Loki**: 720 hours (30 days)
- **Prometheus**: 30 days
- **Grafana dashboards**: Persistent

After vCluster deletion, historical data remains in Loki/Prometheus for 30 days, queryable by `vcluster_namespace`, `github_run_id`, etc.

## Cost and Resource Usage

**Host Cluster Resources:**
- Loki: 500m-2 CPU, 1-4Gi RAM, 50Gi storage
- Prometheus: 500m-2 CPU, 2-8Gi RAM, 50Gi storage
- Grafana: 250m-1 CPU, 512Mi-2Gi RAM, 10Gi storage
- Alloy (host): 100m-500m CPU, 128-512Mi RAM per node

**vCluster Resources:**
- Alloy: 100m-500m CPU, 256Mi-1Gi RAM per node
- kube-state-metrics: 100m-500m CPU, 256-512Mi RAM

**Total (per vCluster test run):** ~200-600m CPU, ~512Mi-1.5Gi RAM

## Security Considerations

1. **Grafana Access**: Change default admin password in production
2. **Metrics Authentication**: Consider enabling mTLS for remote write
3. **Audit Logs**: Contain sensitive API request/response data
4. **Label Injection**: Labels are trusted input from CI environment

## Further Reading

- [Grafana Loki Documentation](https://grafana.com/docs/loki/latest/)
- [Grafana Alloy Documentation](https://grafana.com/docs/alloy/latest/)
- [Kubernetes Audit Logging](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- [vCluster Documentation](https://www.vcluster.com/docs)
