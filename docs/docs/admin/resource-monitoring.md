# Resource Monitoring

The Kaiwo Operator includes a resource monitoring utility which continuously watches your Kaiwo workloads (Jobs or Services) and checks their GPU utilization via metrics endpoints. If any pod of a workload that reserves GPUs is underutilizing the GPU, the operator marks the workload as **Underutilized** and emits an event. If the workload does not utilize the GPU for a given amount of time, it is automatically terminated. This termination feature is enabled by default if resource monitoring is enabled, but it can be disabled in case you want to implement your own termination logic.

In order for workloads to be monitored, they must be deployed via Kaiwo CRDs (`KaiwoJob` or `KaiwoService`). This ensures that the created resources have the correct labels and are inspected by the resource monitor.

## Configuration

### Operator Environmental Variables

Resource monitoring is enabled via environmental variables given to the Kaiwo operator: 

| Parameter                                       | Description                                                                                                                              | Default      |
|-------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------| ------------ |
| `RESOURCE_MONITORING_ENABLED`                   | Enable or disable monitoring (`true`/`false`)                                                                                            | `false`      |
| `RESOURCE_MONITORING_METRICS_ENDPOINT`          | URL of your metrics endpoint                                                                                                             | _(required)_ |
| `RESOURCE_MONITORING_POLLING_INTERVAL`          | How often to check metrics (e.g. `30s`, `1m`)                                                                                            | _(required)_ |

!!!note
  Setting the polling interval very long with workloads that only use GPUs occasionally may end up causing false early terminations, if the GPU is not in use during the polling check. Ensure that your polling interval is low enough to catch GPU usage based on your workload.

These options are set as the operator environment variables and cannot be changed during runtime.

### `resourceMonitoring` field in KaiwoConfig

Please see the [CRD documentation](../reference/crds/config.kaiwo.silogen.ai.md#kaiworesourcemonitoringconfig) for the available options for setting the runtime configuration for the resource monitoring. Changing these fields takes effect immediately. 

## Terminating Underutilizing Workloads

If the KaiwoConfig field `spec.resourceMonitoring.terminateUnderutilizing` is `true` (the default), once a workload has been underutilizing one or more GPUs **continuously** for the time specified in the field `spec.resourceMonitoring.terminateUnderutilizingAfter`, it is flagged for termination by setting the early termination condition and setting the status to `TERMINATING`. The Kaiwo operator will then take care of deleting the dependent resources, but keeps the Kaiwo workload object available to provide a way to inspect the reason for termination.

## Status Conditions

Once monitoring begins, each KaiwoWorkload will have a condition under `.status.conditions`:

```yaml
- type: ResourceUnderutilization
  status: "False"    # “True” means Underutilized, “False” means Normal
  reason: GpuUtilizationNormal    # or GpuUtilizationLow
  message: "GPU utilization normal"
```

If a workload is flagged for early termination, it will have an additional condition:

```yaml
- type: WorkloadTerminatedEarly
  status: "True"
  reason: GpuUtilizationLow    # or GpuUtilizationLow
  message: "Early termination due to low GPU usage"
```

## Best Practices

- **Right-size your thresholds**  
  Choose a sensible cutoff (e.g. 10–30%) so you catch idle pods without false positives
- **Namespace filtering**  
  Use the KaiwoConfig field `spec.resourceMonitoring.targetNamespaces` to restrict monitoring to critical workloads only.

## Troubleshooting

- **No status updates?**
    - Ensure `ENABLED=true` and `METRICS_ENDPOINT` is reachable.
    - Check operator logs for query errors.
- **Excessive events?**
    - Increase `lowUtilizationThreshold` to reduce sensitivity.
