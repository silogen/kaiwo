# Monitoring Stack

Unified observability for AI workload orchestrator test environments using Grafana Alloy, Loki, and Prometheus.

## Quick Start

### 1. Setup Monitoring Infrastructure

Deploy the monitoring stack to the `test-observability` namespace:

```bash
# From dependencies/monitoring directory
cd dependencies/monitoring

# Create namespace
kubectl create namespace test-observability

# Apply base resources (RBAC, ConfigMaps)
kubectl apply -k host/

# Install monitoring components (Loki, Grafana, Prometheus, Alloy)
helmfile sync
```

This deploys:
- **Grafana Alloy**: Collects logs and metrics from vClusters
- **Loki**: Stores and queries logs with unified schema
- **Grafana**: Visualizes logs and metrics
- **Prometheus**: Metrics storage (optional)

### 2. Create a Test Environment

Set up a test namespace with log collection:

```bash
# Set environment variables
export INSTALLER="ci"                                      # or "manual"
export RUN_ID="550e8400-e29b-41d4-a89f-446657440000"      # UUID for this test run
export RUN_ATTEMPT="1"                                     # Attempt number

# Create test namespace with required format
export TEST_NAMESPACE="test-${INSTALLER}-${RUN_ID}-${RUN_ATTEMPT}"
kubectl create namespace ${TEST_NAMESPACE}

# Deploy audit policy ConfigMap - REQUIRED before vCluster
kubectl apply -f vcluster-audit-policy.yaml -n ${TEST_NAMESPACE}
```

Create vCluster with audit logging enabled:

```bash
vcluster create observability-test \
  --namespace ${TEST_NAMESPACE} \
  --values ../vcluster.yaml
```

Deploy event-exporter inside the vCluster:

```bash
# Connect to vCluster
vcluster connect observability-test -n ${TEST_NAMESPACE}

# Deploy event-exporter (creates kube-system and monitoring namespaces)
kubectl apply -k client/

# Disconnect
vcluster disconnect
```

### 3. (Optional) Deploy Audit Log Generator

For testing audit log collection inside the vCluster:

```bash
# Connect to vCluster
vcluster connect observability-test -n ${TEST_NAMESPACE}

# Deploy generator
kubectl apply -f client/audit-log-generator-rbac.yaml
kubectl apply -f client/audit-log-generator.yaml

# Disconnect
vcluster disconnect
```

This creates a pod that continuously performs CRUD operations on ConfigMaps and Pods to generate audit logs.

## Architecture

### Log Collection Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│ vCluster (test-{installer}-{run_id}-{run_attempt})              │
│                                                                 │
│  ┌──────────────────┐    ┌──────────────────┐                   │
│  │ Control Plane    │    │ Workload Pods    │                   │
│  │ - syncer         │    │ - event-exporter │                   │
│  │   (audit logs)   │    │ - application    │                   │
│  └──────────────────┘    └──────────────────┘                   │
│         │                         │                             │
│         │ K8s Audit Logs          │ K8s Events + Pod Logs       │
└─────────┼─────────────────────────┼─────────────────────────────┘
          │                         │
          ▼                         ▼
    ┌─────────────────────────────────────┐
    │ Grafana Alloy (DaemonSet)           │
    │ - Discovers vCluster pods           │
    │ - Collects logs from syncer/pods    │
    │ - Processes into unified schema     │
    │ - Extracts labels from namespace    │
    └─────────────────────────────────────┘
                    │
                    ▼
    ┌─────────────────────────────────────┐
    │ Loki                                │
    │ Stores logs with labels:            │
    │ - source: k8s_audit_log,            │
    │           k8s_event, pod_log        │
    │ - level: info, warning, error       │
    │ - installer, run_id, run_attempt    │
    └─────────────────────────────────────┘
                    │
                    ▼
    ┌─────────────────────────────────────┐
    │ Grafana                             │
    │ Query and visualize unified logs    │
    └─────────────────────────────────────┘
```

### Unified Log Schema

All logs are transformed into a consistent JSON format:

```json
{
  "source": "k8s_audit_log|k8s_event|pod_log",
  "level": "info|warning|error",
  "action": "create|update|delete|...",
  "message": "Human-readable description",
  "object": {
    "name": "resource-name",
    "namespace": "namespace",
    "kind": "Pod|ConfigMap|...",
    "apiVersion": "v1",
    "apiGroup": "..."
  },
  "pod": "pod-name",
  "container": "container-name",
  "timestamp": "2025-11-12T14:00:00Z",
  "details": { /* Original log payload */ }
}
```

### Label Strategy

**Low-cardinality labels** (for efficient querying):
- `source`: Log source type (k8s_audit_log, k8s_event, pod_log)
- `level`: Log severity (info, warning, error)
- `installer`: Test installer type (ci, manual)
- `run_id`: Test run identifier
- `run_attempt`: Attempt number
- `app`: Application name (for filtering, e.g., event-exporter)
- `job`: Alloy source component
- `container`: Container name (syncer, kubernetes - very low cardinality)

**High-cardinality fields** (in JSON only, not labels):
- `pod`, `object.name`, `object.namespace`: In unified JSON payload
- `vcluster_namespace`: Can be reconstructed from `test-{installer}-{run_id}-{run_attempt}`

This design minimizes Loki cardinality while maintaining full queryability via LogQL JSON parsing.

## Accessing Grafana

```bash
# Port-forward to Grafana
kubectl port-forward -n test-observability svc/grafana 3000:3000

# Open browser to http://localhost:3000
# Default credentials: admin/admin
```

## Querying Logs

### LogQL Examples

```logql
# All logs from a specific test run
{installer="ci", run_id="550e8400-e29b-41d4-a89f-446657440000"}

# Audit logs for a specific test
{source="k8s_audit_log", installer="ci"} | json

# Errors from any source
{level="error"}

# Events for a specific object (using JSON parsing)
{source="k8s_event"} | json | object_name="my-pod"

# Audit actions on ConfigMaps
{source="k8s_audit_log"} | json | object_kind="configmaps"

# Pod logs from a specific namespace (using JSON parsing)
{source="pod_log"} | json | object_namespace="default"
```

## Components

### Host Components (test-observability namespace)

- **host/kustomization.yaml**: Base Kubernetes resources (namespaces, RBAC, ConfigMaps)
- **host/helmfile.yaml**: Helm chart deployments (Loki, Grafana, Prometheus)
- **host/alloy-config/**: Grafana Alloy configuration for log collection and processing

### vCluster Components (deployed inside vCluster)

- **client/kustomization.yaml**: Event-exporter deployment
- **client/vcluster-values.yaml**: vCluster configuration with audit logging enabled
- **client/audit-log-generator.yaml**: Optional test workload for generating audit logs
- **client/audit-log-generator-rbac.yaml**: RBAC for audit log generator

## Key Features

### Automatic Discovery

Alloy automatically discovers and collects from:
- All vCluster pods matching label `app=vcluster`
- All workload pods synced to host with label `vcluster.loft.sh/managed-by`

### Namespace Format

Test namespaces MUST follow the format: `test-{installer}-{run_id}-{run_attempt}`

Example: `test-ci-550e8400-e29b-41d4-a89f-446657440000-1`

This enables automatic extraction of:
- `installer`: "ci"
- `run_id`: "550e8400-e29b-41d4-a89f-446657440000"
- `run_attempt`: "1"

### Audit Logging

vCluster's K8s API server writes audit logs to stdout, which are collected by Alloy from the syncer container. Audit logs include:
- API verb (create, update, delete, get, patch)
- User and source IP
- Object reference (resource, namespace, name)
- Request/response bodies (for configured resources)
- Status codes

See `client/vcluster-values.yaml` for the audit policy configuration.

## Troubleshooting

### No logs appearing

1. Check vCluster namespace name matches format: `test-{installer}-{run_id}-{run_attempt}`
2. Verify Alloy is running: `kubectl get pods -n test-observability -l app.kubernetes.io/name=alloy`
3. Check Alloy logs: `kubectl logs -n test-observability -l app.kubernetes.io/name=alloy`
4. Query Loki labels: `kubectl exec -n test-observability deploy/grafana -- curl -s http://loki-0.loki-headless.test-observability.svc.cluster.local:3100/loki/api/v1/labels`

### Audit logs not appearing

1. Verify vCluster is using K8s distro (not K3s) - check `vcluster-values.yaml`
2. Check syncer logs for audit JSON: `kubectl logs -n {namespace} -l app=vcluster -c syncer | grep '"kind":"Event"'`
3. Verify audit policy is configured in vCluster values

### Events not appearing

1. Verify event-exporter is running inside vCluster:
   ```bash
   vcluster connect observability-test -n {namespace}
   kubectl get pods -n kube-system -l app=event-exporter
   vcluster disconnect
   ```
2. Check event-exporter logs for errors

## Data Retention

- **Loki**: 30 days
- **Prometheus**: 30 days (if deployed)

Historical data remains queryable by `installer`, `run_id`, `run_attempt` labels after test cleanup.
