# GPU utilization / termination tests

To run these tests against a remote GPU cluster using a locally running operator, first set up port forwarding to the AMD GPU metrics exporter service:

```bash
kubectl port-forward -n kube-amd-gpu svc/amd-gpu-exporter 5000:5000
```

You can then run the operator with the following environment variables:

```
DISABLE_WEBHOOKS=true
RESOURCE_MONITORING_ENABLED=true
RESOURCE_MONITORING_METRICS_ENDPOINT=http://localhost:5000/metrics
RESOURCE_MONITORING_POLLING_INTERVAL=10s
```

Finally, you can run the tests (assuming you are in this folder):

```
chainsaw test .
```

Note that these tests will alter the default KaiwoConfig object in the cluster, so ensure you are able to restore it to the desired state later.
