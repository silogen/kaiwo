/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoJobSpec defines the desired state of KaiwoJob.
type KaiwoJobSpec struct {
	CommonMetaSpec `json:",inline"`

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
	CommonStatusSpec `json:",inline"`

	// CompletionTime records the timestamp when the KaiwoJob finished execution (either successfully or with failure).
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
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

func (job *KaiwoJob) GetKaiwoWorkloadObject() client.Object {
	return job
}

func (job *KaiwoJob) GetCommonStatusSpec() *CommonStatusSpec {
	return &job.Status.CommonStatusSpec
}

func (job *KaiwoJob) GetCommonSpec() CommonMetaSpec {
	return job.Spec.CommonMetaSpec
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
