// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

// KaiwoJobSpec defines the desired state of KaiwoJob.
type KaiwoJobSpec struct {
	CommonMetaSpec `json:",inline"`

	// ClusterQueue specifies the name of the Kueue `ClusterQueue` that the job should be submitted to for scheduling and resource management.
	//
	// This value is set as the `kueue.x-k8s.io/queue-name` label on the underlying Kubernetes Job or RayJob.
	//
	// If omitted, it defaults to the value specified by the `DEFAULT_CLUSTER_QUEUE_NAME` environment variable in the Kaiwo controller (typically "kaiwo"), which is set during installation.
	//
	// Note! If the applied KaiwoQueueConfig includes no quota for the default queue, no workload will run that tries to fall back on it.
	//
	// The `kaiwo submit` CLI command can override this using the `--queue` flag or the `clusterQueue` field in the `kaiwoconfig.yaml` file.
	ClusterQueue string `json:"clusterQueue,omitempty"`

	// PriorityClass specifies the name of a Kubernetes `PriorityClass` to be assigned to the job's pods. This influences the scheduling priority relative to other pods in the cluster.
	PriorityClass string `json:"priorityClass,omitempty"`

	// EntryPoint defines the command or script that the primary container in the job's pod(s) should execute.
	//
	// It can be a multi-line string. Shell script shebangs (`#!/bin/bash`) are detected.
	//
	// For standard Kubernetes Jobs (`ray: false`), this populates the `command` and `args` fields of the container spec (typically `["/bin/sh", "-c", "<entrypoint_script>"]`).
	//
	// For RayJobs (`ray: true`), this populates the `rayJob.spec.entrypoint` field. For RayJobs, this must reference a Python script.
	//
	// This overrides any default command specified in the container image or the underlying `job` or `rayJob` spec sections if they are also defined.
	EntryPoint string `json:"entrypoint,omitempty"`

	// RayJob defines the RayJob configuration.
	//
	// If this field is present (or if `spec.ray` is `true`), Kaiwo will create a `RayJob` resource instead of a standard `batchv1.Job`.
	//
	// Common fields like `image`, `resources`, `gpus`, `replicas`, etc., will be merged into this spec, potentially overriding values defined here unless explicitly configured otherwise.
	//
	// This provides fine-grained control over the Ray cluster configuration (head/worker groups) and Ray job submission parameters.
	// +kubebuilder:pruning:PreserveUnknownFields
	RayJob *rayv1.RayJob `json:"rayJob,omitempty"`

	// Job defines the Kubernetes Job configuration.
	//
	// If this field is present and `spec.ray` is `false`, Kaiwo will use this as the base for the created `batchv1.Job`.
	//
	// Common fields like `image`, `resources`, `gpus`, `entrypoint`, etc., will be merged into this spec, potentially overriding values defined here.
	//
	// This provides fine-grained control over standard Kubernetes Job parameters like `backoffLimit`, `ttlSecondsAfterFinished`, pod template details, etc.
	// +kubebuilder:pruning:PreserveUnknownFields
	Job *batchv1.Job `json:"job,omitempty"`
}

func (spec *KaiwoJobSpec) IsBatchJob() bool {
	return !spec.IsRayJob()
}

func (spec *KaiwoJobSpec) IsRayJob() bool {
	return spec.RayJob != nil || spec.Ray
}

// KaiwoJobStatus defines the observed state of KaiwoJob.
type KaiwoJobStatus struct {
	// StartTime records the timestamp when the first pod associated with the KaiwoJob started running.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime records the timestamp when the KaiwoJob finished execution (either successfully or with failure).
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Conditions lists the observed conditions of the KaiwoJob resource, following standard Kubernetes conventions.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status reflects the current high-level phase of the KaiwoJob lifecycle (e.g., PENDING, RUNNING, COMPLETE, FAILED).
	Status Status `json:"status,omitempty"`

	// Duration indicates the total time the job ran, calculated from StartTime to CompletionTime, in seconds.
	Duration int64 `json:"duration,omitempty"`

	// ObservedGeneration records the `.metadata.generation` of the KaiwoJob resource that was last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// KaiwoJob represents a batch workload managed by Kaiwo. It encapsulates either a standard Kubernetes Job or a RayJob, along with common metadata, storage configurations, and scheduling preferences. The Kaiwo controller reconciles this resource to create and manage the underlying workload objects.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="StartTime",type="string",JSONPath=".status.startTime"
// +kubebuilder:printcolumn:name="CompletionTime",type="string",JSONPath=".status.completionTime"
// +kubebuilder:printcolumn:name="Duration(s)",type="integer",JSONPath=".status.duration"
type KaiwoJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KaiwoJob, including workload type (Job/RayJob), configuration, resources, and common metadata.
	Spec KaiwoJobSpec `json:"spec,omitempty"`

	// Status reflects the most recently observed state of the KaiwoJob, including its phase, start/completion times, and conditions.
	Status KaiwoJobStatus `json:"status,omitempty"`
}

func (job *KaiwoJob) GetUser() string {
	return job.Spec.CommonMetaSpec.User
}

func (job *KaiwoJob) GetObjectMeta() *metav1.ObjectMeta {
	return &job.ObjectMeta
}

func (job *KaiwoJob) GetStatus() string {
	return string(job.Status.Status)
}

func (job *KaiwoJob) GetType() string {
	return "job"
}

func (job *KaiwoJob) GetPods(ctx context.Context, k8sClient client.Client) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace(job.Namespace), client.MatchingLabels{
		common.KaiwoRunIdLabel: string(job.UID),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

func (job *KaiwoJob) GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

// KaiwoJobList
// +kubebuilder:object:root=true
type KaiwoJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoJob{}, &KaiwoJobList{})
}
