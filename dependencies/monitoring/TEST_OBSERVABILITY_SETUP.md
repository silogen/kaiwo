# Test Observability Setup Guide

Quick guide to deploying the Chainsaw test observability integration.

## Prerequisites

- Host cluster with admin access
- `kubectl`, `helm`, `helmfile` installed
- Existing monitoring stack deployed (or follow steps below)
- `jq`, `psql`, `curl` for test upload script

## Deployment Steps

### 1. Deploy Database Schema ConfigMap

**IMPORTANT**: This must be done BEFORE deploying the helmfile, as PostgreSQL needs the ConfigMap at startup.

```bash
cd dependencies

# Apply the schema ConfigMap
kubectl apply -f monitoring/test-results-schema.yaml
```

**Expected output:**
```
configmap/test-results-schema created
```

### 2. Deploy Monitoring Stack

```bash
# Deploy all components (includes PostgreSQL)
helmfile -f monitoring-helmfile.yaml apply

# This deploys:
# - PostgreSQL (test-results-db) with schema initialization
# - Loki (if not already deployed)
# - Prometheus (if not already deployed)
# - Grafana with PostgreSQL datasource
# - Alloy DaemonSet
```

**Wait for PostgreSQL to be ready:**
```bash
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=postgresql \
  -n test-observability \
  --timeout=5m
```

### 3. Deploy Grafana Dashboards

```bash
# Apply all dashboards (including test results)
kubectl apply -k monitoring/grafana-dashboards/

# Restart Grafana to pick up new dashboards
kubectl rollout restart deployment/grafana -n test-observability
```

### 4. Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n test-observability

# Expected pods:
# - test-results-db-postgresql-0 (1/1 Running)
# - loki-0 (2/2 Running)
# - grafana-xxxxx (3/3 Running)
# - alloy-host-xxxxx (2/2 Running per node)
```

**Test PostgreSQL connection:**
```bash
kubectl exec -it -n test-observability test-results-db-postgresql-0 -- \
  psql -U chainsaw -d testresults -c "SELECT COUNT(*) FROM test_runs;"
```

**Expected output:**
```
 count
-------
     0
(1 row)
```

**Test Grafana access:**
```bash
# Get Grafana URL
echo "Grafana: http://$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'):30300"

# Default credentials: admin/admin
```

### 5. Run Test with Observability

```bash
# Set environment variables
export VCLUSTER_NAMESPACE="kaiwo-test-local-1-helm"
export GITHUB_RUN_ID="test-$(date +%s)"
export GITHUB_RUN_ATTEMPT="1"
export INSTALLER="manual"

# Run tests
./test/chainsaw/run-tests.sh
```

**Expected output:**
```
======================================================================
  Chainsaw Test Runner Configuration
======================================================================

[INFO] Project Root:        /home/alex/code/enterprise-ai/ai-workload-orchestrator
[INFO] Test Directory:      test/chainsaw
[INFO] Run ID:              test-1699999999

...

======================================================================
  Uploading Test Results to Observability Stack
======================================================================

[INFO] Starting Chainsaw test results upload
[INFO] Run ID: test-1699999999
[INFO] Inserting test run: test-1699999999
[INFO] Test run inserted: 4 tests (4 passed, 0 failed)
[INFO] Successfully pushed 120 log entries to Loki
[INFO] Upload complete!
```

### 6. View Results in Grafana

1. Open Grafana: `http://<node-ip>:30300`
2. Login with `admin`/`admin`
3. Navigate to: **Dashboards → KAIWO Test Results**
4. Select your test run from the `Test Run` dropdown
5. Explore test results, logs, and correlated cluster data

## Verification Checklist

- [ ] PostgreSQL pod is running
- [ ] Schema ConfigMap exists: `kubectl get cm test-results-schema -n test-observability`
- [ ] Database tables created: `psql -U chainsaw -d testresults -c "\dt"`
- [ ] Grafana datasource configured: Check in Grafana → Configuration → Data Sources → PostgreSQL
- [ ] Test results dashboard visible: Grafana → Dashboards → KAIWO Test Results
- [ ] Test run completes and uploads successfully
- [ ] Test data appears in Grafana dashboard

## Troubleshooting

### PostgreSQL pod fails to start

**Check logs:**
```bash
kubectl logs -n test-observability test-results-db-postgresql-0
```

**Common issues:**
- Schema ConfigMap not found: Apply it before helmfile
- Storage class unavailable: Check `kubectl get sc`
- Insufficient resources: Check node capacity

**Fix:**
```bash
# Delete and recreate
helmfile -f monitoring-helmfile.yaml destroy --selector name=test-results-db
kubectl apply -f monitoring/test-results-schema.yaml
helmfile -f monitoring-helmfile.yaml apply --selector name=test-results-db
```

### Upload script fails

**Error: `psql: command not found`**

Install PostgreSQL client:
```bash
# Ubuntu/Debian
apt-get install postgresql-client

# macOS
brew install postgresql
```

**Error: `connection refused`**

Port-forward to PostgreSQL:
```bash
kubectl port-forward -n test-observability svc/test-results-db-postgresql 5432:5432

# In another terminal, set DB_HOST
export DB_HOST=localhost
./dependencies/upload-test-results.sh
```

**Error: `jq: command not found`**

Install jq:
```bash
# Ubuntu/Debian
apt-get install jq

# macOS
brew install jq
```

### Dashboard shows no data

**Check datasource:**
1. Grafana → Configuration → Data Sources → PostgreSQL
2. Click "Save & test"
3. Should show "Database Connection OK"

**Check data exists:**
```bash
kubectl exec -it -n test-observability test-results-db-postgresql-0 -- \
  psql -U chainsaw -d testresults -c "SELECT * FROM test_runs;"
```

**Refresh dashboard variables:**
- Click dropdown next to "Test Run"
- Click refresh icon
- Select your test run

## Uninstalling

```bash
# Remove test observability components
helmfile -f monitoring-helmfile.yaml destroy --selector name=test-results-db

# Remove ConfigMaps
kubectl delete -f monitoring/test-results-schema.yaml
kubectl delete -k monitoring/grafana-dashboards/

# Optional: Remove entire monitoring stack
helmfile -f monitoring-helmfile.yaml destroy
```

## Configuration Changes

### Change PostgreSQL Password

Edit `monitoring-helmfile.yaml`:
```yaml
auth:
  password: your-new-password  # Line 164
```

Update datasource in Grafana:
```yaml
secureJsonData:
  password: your-new-password  # Line 132
```

Redeploy:
```bash
helmfile -f monitoring-helmfile.yaml apply
```

### Increase Database Storage

Edit `monitoring-helmfile.yaml`:
```yaml
persistence:
  size: 20Gi  # Increase from 10Gi (Line 170)
```

**Note:** This requires recreating the PVC or expanding it (if storage class supports expansion).

### Add Custom Indexes

Edit `monitoring/test-results-schema.yaml` and add:
```sql
CREATE INDEX IF NOT EXISTS idx_custom ON tests(custom_field);
```

Recreate ConfigMap and pod:
```bash
kubectl delete cm test-results-schema -n test-observability
kubectl apply -f monitoring/test-results-schema.yaml
kubectl delete pod test-results-db-postgresql-0 -n test-observability
```

## Next Steps

- [Test Observability Documentation](../../test/chainsaw/README-observability.md) - Full usage guide
- [Main Monitoring README](./README.md) - Overall observability architecture
- [Quick Start Guide](./QUICK_START.md) - Monitoring stack setup

## Support

For issues or questions:
1. Check logs: `kubectl logs -n test-observability <pod-name>`
2. Review documentation above
3. File an issue with:
   - Error messages
   - Pod status: `kubectl get pods -n test-observability`
   - Recent events: `kubectl get events -n test-observability --sort-by='.lastTimestamp'`
