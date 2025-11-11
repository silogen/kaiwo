# Audit Logging Setup Guide

**⚠️ OUTDATED DOCUMENTATION ⚠️**

This document describes a webhook-based audit logging architecture that is **no longer used**.

**Current approach (as of 2025-11-11):**
- Audit logs are written to stdout by the vCluster K8s API server (--audit-log-path=-)
- Alloy DaemonSet on the host cluster scrapes audit logs from syncer container stdout
- Audit logs are parsed and labeled with `log_type=k8s_audit_log`
- No webhook, no separate Alloy deployment needed
- vCluster must use K8s distro (not K3s) for audit logging configuration to work

**See:** [Main Monitoring README](./README.md) for current setup instructions.

---

## OLD Architecture (No Longer Used)

Audit logging uses a different architecture than other monitoring components:

```
┌─────────────────────────────────────────────────────────────┐
│  Host Cluster - vCluster Namespace (e.g., alex-test)       │
│                                                             │
│  ┌──────────────────┐         ┌──────────────────────────┐ │
│  │ vCluster Pod     │         │ Alloy Audit Receiver     │ │
│  │                  │  HTTP   │                          │ │
│  │ ┌──────────────┐ │ ──────> │ Webhook receiver :8080   │ │
│  │ │ API Server   │ │         │ Forwards to Loki         │ │
│  │ │ Audit webhook│ │         │ (monitoring namespace)   │ │
│  │ └──────────────┘ │         └──────────────────────────┘ │
│  └──────────────────┘                                       │
└─────────────────────────────────────────────────────────────┘
```

**Key Points:**
- Alloy runs in the **host cluster** (same namespace as vCluster pod)
- vCluster API server sends audit logs via HTTP webhook
- Alloy forwards logs to Loki in `monitoring` namespace
- Audit logs are labeled with `log_type=k8s_audit_log`

## Why This Architecture?

The vCluster API server runs in a pod on the **host cluster**, not inside the vCluster virtual cluster. It can only communicate with services in the host cluster, not with services inside the vCluster.

Therefore, Alloy must run in the host cluster namespace to receive audit webhooks.

## Deployment Steps

### Option 1: Automated (GitHub Actions)

The workflow automatically:
1. Creates vCluster namespace
2. Deploys audit components (`deploy-vcluster-audit.sh`)
3. Creates vCluster with audit logging enabled

### Option 2: Manual Deployment

**Step 1: Set environment variables**

```bash
export VCLUSTER_NAMESPACE="alex-test"  # Your vCluster namespace on host
export GITHUB_RUN_ID="local"
export GITHUB_RUN_ATTEMPT="1"
export INSTALLER="manual"
export KUBECONFIG=/path/to/host-kubeconfig  # Host cluster kubeconfig!
```

**Step 2: Create namespace**

```bash
kubectl create namespace "$VCLUSTER_NAMESPACE"
```

**Step 3: Deploy audit components**

```bash
bash ./dependencies/deploy-vcluster-audit.sh
```

This deploys:
- `vcluster-audit-policy` ConfigMap (audit policy)
- `vcluster-audit-webhook` ConfigMap (webhook config)
- `alloy-audit-receiver` Deployment (receives webhooks)
- `alloy-audit-receiver` Service (ClusterIP on port 8080)

**Step 4: Create vCluster**

```bash
vcluster create alex-test \
  --namespace "$VCLUSTER_NAMESPACE" \
  --values dependencies/vcluster.yaml
```

The vCluster will now send audit logs to Alloy!

## Verification

### Check Alloy is running

```bash
kubectl get pods -n $VCLUSTER_NAMESPACE -l app=alloy-audit-receiver
kubectl logs -n $VCLUSTER_NAMESPACE -l app=alloy-audit-receiver -f
```

### Check vCluster API server logs

```bash
kubectl logs -n $VCLUSTER_NAMESPACE -l app=vcluster | grep -i audit
```

You should see:
```
--kube-apiserver-arg=audit-policy-file=/etc/kubernetes/audit/policy.yaml
--kube-apiserver-arg=audit-webhook-config-file=/etc/kubernetes/audit-webhook/webhook-config.yaml
```

### Query audit logs in Grafana

```logql
{vcluster_namespace="alex-test", log_type="k8s_audit_log"}
```

## Troubleshooting

### Error: "host must be a URL or a host:port pair"

This means the webhook ConfigMap has a variable placeholder instead of the actual service name.

**Fix:** Redeploy the webhook ConfigMap:
```bash
kubectl apply -f dependencies/monitoring/vcluster-audit-webhook.yaml -n $VCLUSTER_NAMESPACE
```

The current version uses `http://alloy-audit-receiver:8080/audit` (no variables).

### vCluster pod keeps restarting

Check API server error:
```bash
kubectl logs -n $VCLUSTER_NAMESPACE -l app=vcluster -c syncer | grep -i error
```

Common issues:
- Alloy service not ready: Wait for `alloy-audit-receiver` deployment
- ConfigMaps not found: Ensure audit policy/webhook ConfigMaps exist
- Webhook unreachable: Check Alloy pod logs

### No audit logs in Loki

1. Check Alloy is receiving webhooks:
   ```bash
   kubectl logs -n $VCLUSTER_NAMESPACE -l app=alloy-audit-receiver
   ```

2. Test webhook endpoint:
   ```bash
   kubectl run -it --rm debug --image=curlimages/curl -n $VCLUSTER_NAMESPACE -- \
     curl -v http://alloy-audit-receiver:8080/audit
   ```

3. Check Loki is receiving data:
   ```bash
   kubectl logs -n monitoring -l app.kubernetes.io/name=loki | grep -i write
   ```

## What Gets Audited?

See `vcluster-audit-policy.yaml` for the full policy. Key events:

**RequestResponse level (full request/response bodies):**
- KAIWO CRDs (`kaiwo.silogen.ai`, `aim.silogen.ai`)
- KServe resources (`serving.kserve.io`)
- Ray resources (`ray.io`)
- AppWrappers (`workload.codeflare.dev`)
- Kueue resources (`kueue.x-k8s.io`)

**Metadata level (just metadata, no bodies):**
- Pod, Service, ConfigMap, Secret operations
- Deployments, StatefulSets, DaemonSets
- Jobs, CronJobs
- Events

**Excluded (not logged):**
- System component read operations (kube-proxy, kube-scheduler)
- Lease operations (high volume, low value)
- Prometheus health checks

## Performance Impact

**Resource usage:**
- Alloy audit receiver: ~50-200m CPU, ~128-256Mi RAM
- Audit log volume: ~100-500 KB/minute (varies by activity)
- Loki storage: Included in 30-day retention budget

**Latency impact:**
- Webhook adds <10ms to API requests
- Batch processing reduces overhead (100 events, 5s max wait)

## Security Considerations

⚠️ **Audit logs contain sensitive information:**
- Request/response bodies for CRD operations
- May include secrets, credentials, tokens in request bodies
- Access should be restricted to authorized personnel

**Current setup:**
- No authentication on webhook (Alloy and API server in same namespace)
- No TLS (internal cluster communication)
- For production: Consider mTLS and authentication

## Advanced Configuration

### Adjust Audit Policy

Edit `dependencies/monitoring/vcluster-audit-policy.yaml`:

```yaml
# Example: Add more detailed logging for specific resources
- level: RequestResponse
  resources:
    - group: "custom.io"
      resources: ["myresources"]
```

Redeploy ConfigMap and restart vCluster.

### Increase Batch Size

Edit `dependencies/vcluster.yaml`:

```yaml
controlPlane:
  distro:
    k3s:
      extraArgs:
        - --kube-apiserver-arg=audit-webhook-batch-max-size=200
        - --kube-apiserver-arg=audit-webhook-batch-max-wait=10s
```

### Disable Audit Logging

Remove the audit-related sections from `vcluster.yaml`:

```yaml
controlPlane:
  statefulSet:
    persistence:
      # Remove addVolumes and addVolumeMounts
  distro:
    k3s:
      # Remove audit-related extraArgs
```

Don't deploy audit components.

## Related Documentation

- [Main Monitoring README](./README.md)
- [Quick Start Guide](./QUICK_START.md)
- [Kubernetes Audit Logging](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
