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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoServiceSpec defines the desired state of KaiwoService.
type KaiwoServiceSpec struct {
	CommonMetaSpec `json:",inline"`

	// User specifies the owner or creator of the workload.
	// If authentication is enabled, this must be email address which is checked against authenticated user for match.
	User string `json:"user"`

	// ClusterQueue is the Kueue ClusterQueue name.
	ClusterQueue string `json:"clusterQueue,omitempty"`

	// PriorityClass specifies the Kubernetes PriorityClass for scheduling.
	PriorityClass string `json:"priorityClass,omitempty"`

	// EntryPoint specifies the command or script executed in a Deployment.
	// Can also be defined inside Deployment struct as regular command in the form of string array
	EntryPoint string `json:"entrypoint,omitempty"`

	// Defines the applications and deployments to deploy, should be a YAML multi-line scalar string.
	// Can also be defined inside RayService struct
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`

	// Gpus specifies the total number of GPUs allocated to the workload.
	// Default is 0.
	// +kubebuilder:default=0
	Gpus int `json:"gpus,omitempty"`

	// GpuVendor specifies the GPU vendor (e.g., AMD, NVIDIA, etc.).
	// Default is AMD.
	// +kubebuilder:default=AMD
	GpuVendor string `json:"gpuVendor,omitempty"`

	// Version is an optional field specifying the version of the workload.
	Version string `json:"version,omitempty"`

	// Replicas specifies the number of replicas for the workload.
	// If greater than one, the workload must use Ray.
	// Default is 0.
	// +kubebuilder:default=0
	Replicas int `json:"replicas,omitempty"`

	// GpusPerReplica specifies the number of GPUs allocated per replica.
	GpusPerReplica int `json:"gpus-per-replica,omitempty"`

	// Image defines the container image used for the workload.
	Image string `json:"image,omitempty"`

	// ImagePullSecrets contains the list of secrets used to pull the container image.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// EnvVars specifies the environment variables to be passed to the container.
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Ray determines whether the operator should use RayCluster for workload execution.
	// Default is false.
	// +kubebuilder:default=false
	Ray bool `json:"ray"`

	// ConfigSource allows mounting from Git repository
	ConfigSource *ConfigSource `json:"configSource,omitempty"`

	// Storage configuration for the workload.
	Storage StorageSpec `json:"storage,omitempty"`

	// Optional workload-specific configs (Pointers to avoid bloating CRD)
	RayService *rayv1.RayService  `json:"rayService,omitempty"`
	Deployment *appsv1.Deployment `json:"deployment,omitempty"`
}

// KaiwoServiceStatus defines the observed state of KaiwoService.
type KaiwoServiceStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Status     Status             `json:"Status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoServiceSpec   `json:"spec,omitempty"`
	Status KaiwoServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type KaiwoServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoService{}, &KaiwoServiceList{})
}
