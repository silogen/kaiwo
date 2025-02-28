# Label propagation

In order to facilitate logical label propagation, the Kaiwo controller propagates labels from the Kaiwo objects to the target workload objects.

## From created Kaiwo resource

If you directly create a Kaiwo resource, such as the following job, the labels are taken from two locations, `metadata.labels`, which are propagated to the downstream resource's metadata field, and `spec.podTemplateLabels`, which are propagated to the downstream resources' pod template label field(s).

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: my-job
  labels:
    key1: value1
spec:
  user: my-user
  queue: my-queue
  podTemplateLabels:
    key2: value2
```

The following batch job would get created

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-job
  labels:
    key1: value1
    kueue.x-k8s.io/queue-name: my-queue
    kaiwo.silogen.ai/type: job
    kaiwo.silogen.ai/run-id: "kaiwo-job-UUID"
    kaiwo.silogen.ai/user: my-user
    kaiwo.silogen.ai/name: my-job
spec:
  template:
    metadata:
      labels:
        key2: value2
        kaiwo.silogen.ai/type: job
        kaiwo.silogen.ai/run-id: "kaiwo-job-UUID"
        kaiwo.silogen.ai/user: my-user
        kaiwo.silogen.ai/name: my-job
```

Which in turn would create the following Pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-job-<hash>
  labels:
    key2: value2
    kaiwo.silogen.ai/type: job
    kaiwo.silogen.ai/run-id: "kaiwo-job-UUID"
    kaiwo.silogen.ai/user: my-user
    kaiwo.silogen.ai/name: my-job
```

!!!info
    If you define your job or service spec inline inside the Kaiwo resource definition, these are preserved. However, any label that begins with `kaiwo.silogen.ai/` may be overwritten, if it clashes with a kaiwo system label such as the ones define above.

## From directly created resource with a Kaiwo label

If you create a resource such as a batch Job directly and assign the `kaiwo.silogen.ai/managed: true` label, similar logic is applied.

If you create a Job such as 

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-job
  labels:
    key1: value1
    kaiwo.silogen.ai/managed: "true"
    kueue.x-k8s.io/queue-name: my-queue
spec:
  template:
    metadata:
      labels:
        key2: value2
```

This will update the job with the `kaiwo.silogen.ai/` system labels:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-job
  labels:
    key1: value1
    kaiwo.silogen.ai/managed: "true"
    kueue.x-k8s.io/queue-name: my-queue
    kaiwo.silogen.ai/type: job
    kaiwo.silogen.ai/run-id: "kaiwo-job-UUID"
    kaiwo.silogen.ai/user: my-user
    kaiwo.silogen.ai/name: my-job
spec:
  template:
    metadata:
      labels:
        key2: value2
        kaiwo.silogen.ai/type: job
        kaiwo.silogen.ai/run-id: "kaiwo-job-UUID"
        kaiwo.silogen.ai/user: my-user
        kaiwo.silogen.ai/name: my-job
```

and create a KaiwoJob:

```yaml
apiVersion: kaiwo.silogen.ai/v1alpha1
kind: KaiwoJob
metadata:
  name: my-job
  labels:
    key1: value1
    kaiwo.silogen.ai/managed: "true"
spec:
  queue: my-queue
```

Again, if you try to include any protected `kaiwo.silogen.ai/` labels, these will be overwritten.
