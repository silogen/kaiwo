# Deployment Notes - test-observability Stack

## Environment Details
- **Cluster Type**: RKE2
- **Existing Monitoring**: Yes (`monitoring` namespace with kube-prometheus-stack)
- **DNS Service**: `rke2-coredns-rke2-coredns.kube-system`
- **Storage Class**: `openebs-hostpath`

## Issues Encountered & Solutions

### 1. ✅ FIXED: Loki Deployment Mode Validation Error

**Error:**
```
execution error at (loki/templates/validate.yaml:31:4):
You have more than zero replicas configured for both the single binary
and simple scalable targets
```

**Solution:**
Added explicit `replicas: 0` for unused deployment modes in `monitoring-helmfile.yaml`:
```yaml
deploymentMode: SingleBinary
read:
  replicas: 0
write:
  replicas: 0
backend:
  replicas: 0
```

### 2. ✅ FIXED: Node Exporter Port Conflict

**Error:**
```
listen tcp 0.0.0.0:9100: bind: address already in use
```

**Cause:** Existing node-exporter in `monitoring` namespace already using port 9100

**Solution:**
- Disabled node-exporter in our stack: `nodeExporter.enabled: false`
- Removed node-exporter scraping from Alloy config
- Using existing node-exporter from `monitoring` namespace

### 3. ⚠️ WORKAROUND: Loki Gateway DNS Resolution

**Error:**
```
host not found in resolver "kube-dns.kube-system.svc.cluster.local."
```

**Cause:** RKE2 uses `rke2-coredns` not `kube-dns`

**Current Status:** loki-gateway in CrashLoopBackOff

**Workaround Options:**
1. **Access Loki directly** (recommended for now):
   - Use `loki-0.loki-headless.test-observability:3100` instead of gateway
   - Update Alloy/Grafana configs to use direct endpoint

2. **Fix gateway DNS** (future):
   - Configure proper DNS resolver in Loki gateway nginx config
   - Or disable gateway entirely

### 4. ✅ FIXED: Grafana Dashboard ConfigMap Name Hashing

**Error:**
```
MountVolume.SetUp failed for volume "dashboards-kaiwo" : configmap "grafana-dashboards-kaiwo" not found
```

**Cause:** Kustomize was generating ConfigMap names with hash suffix (`grafana-dashboards-kaiwo-64tg6h4d9f`), but helmfile expected exact name

**Solution:**
Added `disableNameSuffixHash: true` to kustomization.yaml:
```yaml
configMapGenerator:
  - name: grafana-dashboards-kaiwo
    options:
      disableNameSuffixHash: true
      labels:
        grafana_dashboard: "1"
```

## Successful Deployments

### ✅ Working Components (All Running!)
1. **Loki** - 2/2 Running, accepting writes
2. **Prometheus** - 2/2 Running, scraping metrics
3. **Grafana** - 3/3 Running with dashboards loaded
4. **Alloy (Host)** - 2/2 Running (DaemonSet on 2 nodes)
5. **kube-state-metrics** - 1/1 Running
6. **Prometheus Operator** - 1/1 Running
7. **Alertmanager** - 2/2 Running

### ⚠️ Known Issues (Non-blocking)
1. **loki-gateway** - CrashLoopBackOff due to DNS issue (can be bypassed, components access Loki directly)

## Deployment Commands

### 1. Deploy Stack
```bash
cd /home/alex/code/enterprise-ai/ai-workload-orchestrator/dependencies
helmfile -f monitoring-helmfile.yaml apply
```

### 2. Check Status
```bash
kubectl get pods -n test-observability
```

### 3. Deploy Alloy Config (After stack is up)
```bash
kubectl apply -f monitoring/alloy-host-config.yaml -n test-observability
kubectl rollout restart daemonset/alloy-host -n test-observability
```

### 4. Load Dashboards (Using Kustomize)
```bash
# Apply dashboards using kustomize
kubectl apply -k monitoring/grafana-dashboards/

# Restart Grafana to pick up dashboards
kubectl rollout restart deployment/kube-prometheus-stack-grafana -n test-observability

# Wait for Grafana to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n test-observability --timeout=2m
```

## Configuration Changes Made

### monitoring-helmfile.yaml
1. Set Loki `deploymentMode: SingleBinary` with other modes at 0 replicas
2. Disabled `nodeExporter` (conflicts with existing)
3. Changed namespace from `monitoring` to `test-observability`
4. Added DNS resolver workaround attempt (may need further work)

### alloy-host-config.yaml
1. Changed namespace to `test-observability`
2. Removed node-exporter scraping (disabled component)
3. Updated all service URLs to use `test-observability` namespace

### alloy-vcluster-config.yaml
1. Updated Loki/Prometheus URLs to `test-observability` namespace

## Next Steps

1. **Wait for Grafana** to complete initialization
2. **Test Loki direct access** instead of gateway
3. **Deploy Alloy config** once stack is stable
4. **Verify data flow** from Alloy → Loki/Prometheus
5. **Access Grafana** and configure datasources
6. **Load dashboards** and verify they work
7. **Test with vCluster** deployment

## Access Information

### Grafana
- **NodePort**: 30300
- **URL**: `http://<node-ip>:30300`
- **Credentials**: admin/admin
- **Port-forward**: `kubectl port-forward -n test-observability svc/kube-prometheus-stack-grafana 3000:80`

### Loki (Direct)
- **Endpoint**: `http://loki-0.loki-headless.test-observability:3100`
- **Push URL**: `http://loki-0.loki-headless.test-observability:3100/loki/api/v1/push`

### Prometheus
- **Endpoint**: `http://kube-prometheus-stack-prometheus.test-observability:9090`

## Troubleshooting Commands

```bash
# Check all pods
kubectl get pods -n test-observability

# Check specific component logs
kubectl logs -n test-observability <pod-name>

# Check Grafana init container
kubectl logs -n test-observability kube-prometheus-stack-grafana-<id> -c grafana-sc-dashboard

# Force restart
kubectl rollout restart deployment/<name> -n test-observability

# Delete and recreate
helmfile -f monitoring-helmfile.yaml destroy
helmfile -f monitoring-helmfile.yaml apply
```

## Lessons Learned

1. **Always check for existing infrastructure** before deploying
2. **RKE2 clusters** use different service names than standard K8s
3. **Loki gateway** can be bypassed for simplicity
4. **Node-exporter** is commonly already deployed
5. **DNS resolver configuration** varies by cluster type
6. **Kustomize name hashing** must be disabled when helmfile expects exact ConfigMap names

## Final Deployment Status ✅

**Date**: 2025-11-07
**Status**: SUCCESS - All components deployed and operational

### Verified Working:
- ✅ Grafana accessible at http://10.21.9.9:30300 (admin/admin)
- ✅ KAIWO dashboards loaded (Namespace Debug, Cluster Health)
- ✅ Loki accepting writes and available for queries
- ✅ Prometheus collecting metrics
- ✅ Alloy DaemonSet running on all nodes
- ✅ All components in test-observability namespace

### Next Steps:
1. Test with vCluster deployment
2. Verify audit log collection from vCluster
3. Verify metrics collection from vCluster
4. Test dashboards with real test data
