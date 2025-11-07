# Quick Start Guide - KAIWO Observability Stack

## TL;DR

Set up complete observability for KAIWO vCluster testing with Grafana, Loki, and Prometheus.

## Prerequisites

- Access to host cluster with admin permissions
- `kubectl`, `helm`, `helmfile` installed
- At least 10Gi available storage on host cluster
- Working directory: `/home/alex/code/enterprise-ai/ai-workload-orchestrator/dependencies`

## Tested Deployment Steps (RKE2 Cluster)

### Step 1: Deploy Monitoring Stack to Host Cluster

```bash
# From dependencies directory
cd dependencies

# Deploy Loki, Prometheus, Grafana to test-observability namespace
helmfile -f monitoring-helmfile.yaml apply

# This will take 3-5 minutes
# Some components may show errors initially (loki-gateway) - this is expected
```

### Step 2: Deploy Grafana Dashboards

```bash
# Apply dashboards using kustomize
kubectl apply -k monitoring/grafana-dashboards/

# Wait for Grafana to start (it needs the dashboard ConfigMap)
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n test-observability --timeout=5m
```

### Step 3: Deploy Alloy Configuration

```bash
# Apply Alloy config
kubectl apply -f monitoring/alloy-host-config.yaml

# Restart Alloy to pick up config
kubectl rollout restart daemonset/alloy-host -n test-observability
```

### Step 4: Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n test-observability

# You should see:
# - kube-prometheus-stack-grafana: 3/3 Running
# - loki-0: 2/2 Running
# - kube-prometheus-stack-prometheus-0: 2/2 Running (may take a few minutes)
# - alloy-host-*: 2/2 Running (DaemonSet pods)
# - kube-state-metrics: 1/1 Running
# Note: loki-gateway may be in CrashLoopBackOff - this is OK, we access Loki directly
```

### Step 5: Access Grafana

```bash
# Get the NodePort
kubectl get svc -n test-observability kube-prometheus-stack-grafana

# Access Grafana at http://<node-ip>:30300
# Default credentials: admin/admin
```

### Step 6: Create vCluster with Monitoring (Automatic in CI)

When you run tests via GitHub Actions, monitoring is automatically deployed!

**Manual vCluster creation:**

```bash
# Create vCluster
vcluster create test-cluster \
  --namespace kaiwo-test-local-1-manual \
  --values dependencies/vcluster.yaml

# Connect to vCluster
vcluster connect test-cluster --namespace kaiwo-test-local-1-manual

# Deploy dependencies
export KUBECONFIG=~/.kube/config  # vCluster kubeconfig
bash ./dependencies/deploy.sh vcluster up

# Deploy monitoring in vCluster
export VCLUSTER_NAMESPACE="kaiwo-test-local-1-manual"
export GITHUB_RUN_ID="local"
export GITHUB_RUN_ATTEMPT="1"
export INSTALLER="manual"
bash ./dependencies/deploy-vcluster-monitoring.sh
```

## Using the Dashboards

### 1. Namespace Debug Dashboard

**Use case:** Debugging why a specific test failed

1. Go to Grafana → Dashboards → KAIWO Namespace Debug Dashboard
2. Select your vCluster namespace (e.g., `kaiwo-test-123-1-helm`)
3. Select the test namespace (e.g., `chainsaw-basic-test-xyz`)
4. View:
   - Pod logs with search
   - Kubernetes events (pod failures, scheduling issues)
   - Pod status timeline
   - Resource usage (CPU/memory)
   - API audit log (what operations happened)

### 2. Cluster Health Dashboard

**Use case:** Understanding cluster-wide resource constraints

1. Go to Grafana → Dashboards → KAIWO Cluster Health
2. Select your vCluster namespace
3. View:
   - Total nodes, pods, failures
   - Node CPU and memory usage
   - Pods per node
   - GPU utilization

## Common Debugging Workflows

### Workflow 1: "Test timed out, but I don't know why"

1. Open **Namespace Debug Dashboard**
2. Select vCluster namespace and test namespace
3. Look at **Kubernetes Events** panel for:
   - `Failed to schedule` → Node capacity issues
   - `ImagePullBackOff` → Image not available
   - `OOMKilled` → Memory limit too low
4. Check **Pod Logs** panel for application errors
5. View **Pod Status** timeline to see when pod got stuck
6. Check **Cluster Health Dashboard** → GPU Utilization if GPU test

### Workflow 2: "Test passed locally but failed in CI"

1. Compare two test runs side-by-side:
   - Filter by `github_run_id` in Grafana Explore
2. Compare:
   - Resource usage patterns
   - Timing of pod lifecycle events
   - API operations in audit log
3. Check **Cluster Health** for node-level differences

### Workflow 3: "Multiple tests failing randomly"

1. Open **Cluster Health Dashboard**
2. Look for:
   - High node CPU/memory usage
   - Many pending pods
   - GPU saturation
3. Correlate with test failure times
4. Check **API Audit Log** for resource quota rejections

## Quick LogQL Queries (Grafana Explore)

**All errors in namespace:**
```logql
{namespace="my-test-ns", vcluster_namespace="kaiwo-test-123-1-helm"} |~ "(?i)error|fatal"
```

**Pod scheduling failures:**
```logql
{log_type="k8s_event", vcluster_namespace="kaiwo-test-123-1-helm"} |~ "FailedScheduling"
```

**KAIWO operator logs:**
```logql
{namespace="kaiwo-system", vcluster_namespace="kaiwo-test-123-1-helm", container="manager"}
```

**Audit log for CRD creates:**
```logql
{log_type="k8s_audit_log", verb="create", resource=~"aims|rayservices"}
```

## Quick PromQL Queries (Grafana Explore)

**Pod restart rate:**
```promql
rate(kube_pod_container_status_restarts_total{vcluster_namespace="kaiwo-test-123-1-helm"}[5m])
```

**Operator reconciliation errors:**
```promql
rate(controller_runtime_reconcile_errors_total{vcluster_namespace="kaiwo-test-123-1-helm"}[5m])
```

**Pending workloads in Kueue:**
```promql
kueue_pending_workloads{vcluster_namespace="kaiwo-test-123-1-helm"}
```

## Troubleshooting

### "No data in Grafana"

```bash
# Check if Alloy is running in vCluster
kubectl get pods -l app=alloy-vcluster
kubectl logs -l app=alloy-vcluster --tail=50

# Check if data is being written to Loki
kubectl logs -n monitoring -l app.kubernetes.io/name=loki --tail=50 | grep -i write
```

### "Can't access Grafana"

```bash
# Use port-forward instead of NodePort
kubectl port-forward -n monitoring svc/kube-prometheus-stack-grafana 3000:80

# Access at http://localhost:3000
```

### "Audit logs not showing"

Audit logs require ConfigMaps to be deployed to the **host cluster** vCluster namespace:

```bash
# Switch to host kubeconfig
export KUBECONFIG=/path/to/host-kubeconfig

# Deploy audit config to vCluster namespace
kubectl apply -f monitoring/vcluster-audit-policy.yaml -n kaiwo-test-123-1-helm
kubectl apply -f monitoring/vcluster-audit-webhook.yaml -n kaiwo-test-123-1-helm
```

## Next Steps

- Read full documentation: [README.md](./README.md)
- Customize dashboards in Grafana
- Set up alerting for test failures
- Export dashboards for version control

## Getting Help

- Check logs: `kubectl logs -n monitoring -l app.kubernetes.io/name=<component>`
- Grafana docs: https://grafana.com/docs/
- Loki docs: https://grafana.com/docs/loki/latest/
- File issues: https://github.com/your-org/ai-workload-orchestrator/issues
