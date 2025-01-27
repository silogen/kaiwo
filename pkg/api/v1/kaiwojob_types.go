package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RayJobSpec defines fields for RayJob
type RayJobSpec struct {
	EntryPoint string            `json:"entryPoint,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
}

// KueueSpec defines Kueue-related configuration
type KueueSpec struct {
	ClusterQueueName string            `json:"clusterQueueName"`
	Quotas           map[string]string `json:"quotas,omitempty"`
}

// KubernetesJobSpec defines fields for a Kubernetes Job
type KubernetesJobSpec struct {
	Containers []corev1.Container `json:"containers"`
	Volumes    []corev1.Volume    `json:"volumes,omitempty"`
}

// KaiwoJobSpec defines the desired state of KaiwoJob
type KaiwoJobSpec struct {
	RayJobSpec *RayJobSpec        `json:"rayJobSpec,omitempty"`
	JobSpec    *KubernetesJobSpec `json:"jobSpec,omitempty"`
	Kueue      KueueSpec          `json:"kueue,omitempty"`
}

// KaiwoJobStatus defines the observed state of KaiwoJob
type KaiwoJobStatus struct {
	Conditions []v1.Condition `json:"conditions,omitempty"`
}

// KaiwoJob is the Schema for the kaiwojobs API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoJob struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoJobSpec   `json:"spec,omitempty"`
	Status KaiwoJobStatus `json:"status,omitempty"`
}

// KaiwoJobList contains a list of KaiwoJob
// +kubebuilder:object:root=true
type KaiwoJobList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []KaiwoJob `json:"items"`
}


