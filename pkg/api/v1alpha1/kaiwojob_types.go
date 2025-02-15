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
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoJobSpec defines the desired state of KaiwoJob.
type KaiwoJobSpec struct {
	CommonMetaSpec `json:",inline"`

	// User specifies the owner or creator of the workload.
	// If authentication is enabled, this must be email address which is checked against authenticated user for match.
	User string `json:"user"`

	// EntryPoint specifies the command or script executed in the job.
	EntryPoint string `json:"entryPoint,omitempty"`

	// ClusterQueue is the Kueue ClusterQueue name.
	ClusterQueue string `json:"clusterQueue,omitempty"`

	// PriorityClass specifies the Kubernetes PriorityClass for scheduling.
	PriorityClass string `json:"priorityClass,omitempty"`

	// Gpus specifies the total number of GPUs allocated to the workload.
	Gpus int `json:"gpus,omitempty"`

	// GpuVendor specifies the GPU vendor (e.g., AMD, NVIDIA, etc.).
	// Default is AMD.
	// +kubebuilder:default=AMD
	GpuVendor string `json:"gpuVendor,omitempty"`

	// Version is an optional field specifying the version of the workload.
	Version string `json:"version,omitempty"`

	// Replicas specifies the number of replicas for the workload.
	// If greater than one, the workload must use Ray.
	Replicas int `json:"replicas,omitempty"`

	// GpusPerReplica specifies the number of GPUs allocated per replica.
	GpusPerReplica int `json:"gpus-per-replica,omitempty"`

	// Image defines the container image used for the workload.
	Image string `json:"image,omitempty"`

	// ImagePullSecrets contains the list of secrets used to pull the container image.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// EnvVars specifies the environment variables to be passed to the container.
	EnvVars []corev1.EnvVar `json:"envVars,omitempty"`

	// Ray determines whether the operator should use RayCluster for workload execution.
	// Default is false.
	// +kubebuilder:default=false
	Ray bool `json:"ray"`

	// ConfigSource allows mounting from ConfigMap or S3
	ConfigSource *ConfigSource `json:"configSource,omitempty"`

	// Storage configuration for the workload.
	Storage StorageSpec `json:"storage,omitempty"`

	// RayClusterSpec defines the Ray cluster configuration.
	RayClusterSpec *rayv1.RayClusterSpec `json:"rayClusterSpec,omitempty"`

	// JobSpec defines the Kubernetes Job configuration.
	JobSpec *batchv1.JobSpec `json:"jobSpec,omitempty"`
}

// KaiwoJobStatus defines the observed state of KaiwoJob.
type KaiwoJobStatus struct {
	StartTime       *metav1.Time       `json:"startTime,omitempty"`
	CompletionTime  *metav1.Time       `json:"completionTime,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
	ReplicaStatuses map[string]int32   `json:"replicaStatuses,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoJobSpec   `json:"spec,omitempty"`
	Status KaiwoJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type KaiwoJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoJob{}, &KaiwoJobList{})
}
