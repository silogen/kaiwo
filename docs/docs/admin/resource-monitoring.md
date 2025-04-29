# Resource Monitoring

The Kaiwo Operator includes a resource monitoring utility which continuously watches your Kaiwo workloads (Jobs or Services) and checks their CPU or GPU utilization via Prometheus. If a workload’s average utilization over a configurable time window falls below a threshold, the operator marks it **Underutilized** and emits an event, letting you identify idle resources in your cluster.

!!! info "Future feature"
    A feature will soon be available which can be used to automatically terminate Kaiwo workloads that are marked as Underutilized

In order for workloads to be monitored, they must be deployed via Kaiwo CRDs (`KaiwoJob` or `KaiwoService`). This ensures that the created resources have the correct labels and are inspected by the resource monitor.

## Monitoring Profiles

You can choose one of two metrics profiles:

**GPU Profile**

The GPU profile tracks GPU utilization, and looks at all GPUs individually. This means that if even one GPU is underutilized, the workload is flagged.

**CPU Profile**

The CPU profile tracks CPU utilization, and looks at the percentage ratio between what has been requested for the workload and what the workload is actually using. This calculation is done based on the total that all the pods in the workload are using.


## Configuration Parameters

Resource monitoring is enabled and configured via the following environmental variables. 

| Parameter                                        | Description                                                                                                                              | Default      |
|--------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------| ------------ |
| `RESOURCE_MONITORING_ENABLED`                    | Enable or disable monitoring (`true`/`false`)                                                                                            | `false`      |
| `RESOURCE_MONITORING_PROFILE`                    | `gpu` or `cpu`                                                                                                                           | `gpu`        |
| `RESOURCE_MONITORING_PROMETHEUS_ENDPOINT`        | URL of your Prometheus server                                                                                                            | _(required)_ |
| `RESOURCE_MONITORING_POLLING_INTERVAL`           | How often to check metrics (e.g. `30s`, `1m`)                                                                                            | _(required)_ |
| `RESOURCE_MONITORING_AVERAGING_INTERVAL`         | Window over which to compute average utilization (e.g. `5m`)                                                                             | _(required)_ |
| `RESOURCE_MONITORING_MIN_ALIVE_TIME`             | Minimum pod age before monitoring starts (e.g. `1m`)                                                                                     | _(required)_ |
| `RESOURCE_MONITORING_LOW_UTILIZATION_THRESHOLD`  | Utilization % below which a workload is underutilized (e.g. `20`)                                                                        | _(required)_ |
| `RESOURCE_MONITORING_TARGET_NAMESPACES`          | Comma-separated list of namespaces to include (e.g. `ml,ai`). If not provided or empty, all namespaces are included (except `kube-system`) | _(optional)_ |

---

## Status Conditions

Once monitoring begins, each KaiwoWorkload will have a condition under `.status.conditions`:

```yaml
- type: ResourceUnderutilization
  status: "True"    # “True” means Underutilized, “False” means Normal
  reason: GpuUtilizationNormal    # or GpuUtilizationLow, CpuUtilizationNormal, CpuUtilizationLow
  message: "GPU utilization normal"
```

---

## Viewing Events

The operator also emits Events on the workload object:

- **Warning** when utilization drops below threshold
- **Normal** when it returns above threshold

You can inspect them with:

```bash
kubectl describe <kaiwojob|kaiwoservice> my-workload
```

## Best Practices

- **Right-size your thresholds**  
  Choose a sensible cutoff (e.g. 10–30%) so you catch idle pods without false positives.
- **Tune polling & averaging**
    - Short windows (5–10 min) react faster but can be noisy.
    - Longer windows (30–120 min) smooth out spikes.
- **Namespace filtering**  
  Use `TARGET_NAMESPACES` to restrict monitoring to critical workloads only.

## Troubleshooting

- **No status updates?**
    - Ensure `ENABLED=true` and `PROMETHEUS_ENDPOINT` is reachable.
    - Check operator logs for query errors.
- **Threshold never crossed?**
    - Verify your query window (`AVERAGING_INTERVAL`) aligns with real workload patterns.
    - Inspect raw Prometheus metrics to confirm values.
- **Excessive events?**
    - Increase `LOW_UTILIZATION_THRESHOLD` to reduce sensitivity.
    - Lengthen `POLLING_INTERVAL` to batch state changes.
