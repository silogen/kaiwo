# Basic Tests for GPU Orchestration

## Basic Workload Submission

**Rationale**: Kaiwo Operator must orchestrate GPUs intelligently.

### Given
- The user has developed code and pushed it to a remote Git repository.
- The user wants to achieve one of the following:
  1. Single node job submission
  2. Single/Multi-node RayJob submission
  3. Single node Deployment submission
  4. Single/Multi-node RayService submission

---

## Single Node Job Submission

### Submission of KaiwoJob via Kaiwo CLI

**When** the user submits a non-Ray KaiwoJob via Kaiwo CLI that requests GPUs:
- **Then** they will use at least some Kaiwo CLI arguments for KaiwoJobs, including `--gpus`, `--gpu-models`, `--vendor`.
- **Then** they will possibly specify at least the code location and possibly a `clusterQueue`.
- **Then** KaiwoCLI turns the CLI arguments into a KaiwoJob CRD.
- **Then** Kaiwo CLI ensures that all provided arguments are valid.
- **Then** the CRD is submitted to the Kubernetes API, where it is accepted and picked up by the KaiwoJob Controller.
- **Then** the Controller selects nodes with GPUs matching the provided GPU vendor.
- **Then** the Controller uses bin-packing to select nodes that can fulfill the requested number of GPUs.
- **Then** the Controller uses `kaiwo-nodepool` labels to select nodes matching the user's GPU model requests (if provided).
- **Then** the Controller submits the Job to Kueue with the provided `clusterQueue` label.
- **Then** Job resource becomes pending with desired number of GPUs and 1 replica.
- **Then** Kueue admits the Job when resources become available.
- **Then** the user-provided code runs successfully in the Job container.

### Submission of Job via kubectl apply

**When** the user submits a standard Job manifest via `kubectl apply` that requests GPUs:
- **Then** they will use the label: `"kaiwo.silogen.ai/managed": "true"`.
- **Then** the Kaiwo webhook for Jobs validates the manifest and converts it into a KaiwoJob.
- **Then** the Kaiwo webhook submits the KaiwoJob to the Kubernetes API, where it is picked up by the KaiwoJob Controller.
- **Then** the rest of the process follows the same GPU orchestration steps as a KaiwoJob submission via CLI.

### Submission of KaiwoJob via kubectl apply

**When** the user submits a KaiwoJob manifest via `kubectl apply` that requests GPUs:
- **Then** they will use some KaiwoJob CRD fields for GPU orchestration, including `gpus`, `gpu-models`, `vendor`.
- **Then** the process follows the same GPU orchestration steps as a CLI or standard Job submission.

---

## Single/Multi-Node RayJob Submission

### Submission of KaiwoJob via Kaiwo CLI

**When** the user submits a Ray KaiwoJob via Kaiwo CLI that requests GPUs:
- **Then** they will use at least some Kaiwo CLI arguments for Ray KaiwoJobs, including `--gpus`, `--gpus-per-replica`, `--replicas`, `--gpu-models`, `--vendor`.
- **Then** the Controller selects the number of replicas and GPUs per replica based on availability, ensuring optimal GPU allocation (e.g., a request for 8 GPUs may be distributed as 4+2+2 across nodes).
- **Then** the rest of the process follows the same GPU orchestration steps as a standard KaiwoJob submission except that there may be more replicas (Ray worker pods).

### Submission of RayJob via kubectl apply

**When** the user submits a RayJob manifest via `kubectl apply` that requests GPUs:
- **Then** the label `"kaiwo.silogen.ai/managed": "true"` is used.
- **Then** the Kaiwo webhook validates and converts it into a KaiwoJob before proceeding with GPU orchestration.

### Submission of KaiwoJob via kubectl apply

**When** the user submits a Ray-enabled KaiwoJob via `kubectl apply` that requests GPUs:
- **Then** the user will use some fields on the KaiwoJob CRD for GPU orchestration in Ray scenarios.
- **Then** the rest of the process follows the same GPU orchestration steps as a standard Ray KaiwoJob submission.

---

## Single Node Deployment Submission

### Submission of KaiwoService via Kaiwo CLI

**When** the user submits a KaiwoService via Kaiwo CLI that requests GPUs:
- **Then** the request follows GPU orchestration steps similar to a KaiwoJob submission.

### Submission of Deployment via kubectl apply

**When** the user submits a Deployment via `kubectl apply` that requests GPUs:
- **Then** the request follows GPU orchestration steps similar to a standard Job submission.

### Submission of KaiwoService via kubectl apply

**When** the user submits a KaiwoService via `kubectl apply` that requests GPUs:
- **Then** the request follows GPU orchestration steps similar to a KaiwoJob submission.

---

## Single/Multi-Node RayService Submission

### Submission of RayService via kubectl apply

**When** the user submits a RayService via `kubectl apply` that requests GPUs:
- **Then** the label `"kaiwo.silogen.ai/managed": "true"` is used.
- **Then** the request follows GPU orchestration steps similar to a RayJob submission.

### Submission of KaiwoService via Kaiwo CLI

**When** the user submits a KaiwoService via Kaiwo CLI that requests GPUs:
- **Then** the request follows GPU orchestration steps similar to a KaiwoJob submission.

### Submission of KaiwoService via kubectl apply

**When** the user submits a KaiwoService via `kubectl apply` that requests GPUs:
- **Then** the request follows GPU orchestration steps similar to a KaiwoJob submission.

