# Fluent Bit Configuration for Test Log Collection

This directory contains custom Fluent Bit configurations for collecting logs from test environments.

## Files

- **kubernetes-filter-patch.yaml**: Patches the default kubernetes filter to enable pod labels (required for log classification)
- **test-simple.yaml**: Main configuration for test namespace log collection
  - Namespace filtering by pattern `test-{installer}-{run-id}-{run-attempt}`
  - Field extraction (installer, run_id, run_attempt)
  - Log type classification (audit, event, pod)

## Installation

The configuration is automatically applied via helmfile hooks. When you run:

```bash
helmfile sync
```

The following happens:
1. Fluent Operator is installed/upgraded
2. Post-sync hook applies `kubernetes-filter-patch.yaml` to enable labels
3. Apply test configuration:
   ```bash
   kubectl apply -f test-simple.yaml
   ```

## Manual Application

If you need to apply the configuration manually:

```bash
# Apply kubernetes filter patch (enables labels)
kubectl apply -f kubernetes-filter-patch.yaml

# Apply test log collection configuration
kubectl apply -f test-simple.yaml
```

## Log Classification

Logs are classified into three types based on pod labels:

1. **audit**: Logs from vCluster pods (`app=vcluster` label)
   - Contains API server audit logs
   - Example: `/healthz` requests, API operations

2. **event**: Logs from event-exporter pods (`app=event-exporter` label)
   - Contains Kubernetes events
   - Example: Pod scheduled, Container started

3. **pod**: All other pod logs
   - Application logs from other containers

## Extracted Fields

Each log record includes:
- `installer`: Test installer name (e.g., "ci")
- `run_id`: Test run UUID
- `run_attempt`: Test run attempt number
- `log_type`: Classification (audit/event/pod)
- `kubernetes.*`: Pod metadata (name, namespace, labels, etc.)

## Troubleshooting

### Logs not appearing
Check Fluent Bit pod logs:
```bash
kubectl logs -n test-observability -l app.kubernetes.io/name=fluent-bit --tail=50
```

### Classification not working
Verify labels are enabled in kubernetes filter:
```bash
kubectl get clusterfilter kubernetes -o yaml | grep labels:
```
Should show `labels: true`

### Verify log types
Check distribution of log types:
```bash
kubectl logs -n test-observability -l app.kubernetes.io/name=fluent-bit --tail=200 | \
  grep -E '^\[[0-9]+\]' | grep -o '"log_type"=>"[^"]*"' | sort | uniq -c
```

Expected output:
```
    181 "log_type"=>"audit"
      3 "log_type"=>"event"
     12 "log_type"=>"pod"
```
