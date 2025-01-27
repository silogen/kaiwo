package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RayServiceSpec defines fields for RayService
type RayServiceSpec struct {
	HeadNodeSpec   runtime.RawExtension `json:"headNodeSpec,omitempty"`
	WorkerNodeSpec runtime.RawExtension `json:"workerNodeSpec,omitempty"`
}

// DeploymentSpec defines fields for a Kubernetes Deployment
type DeploymentSpec struct {
	Replicas int32                 `json:"replicas,omitempty"`
	Template corev1.PodTemplateSpec `json:"template,omitempty"`
}

// KaiwoServiceSpec defines the desired state of KaiwoService
type KaiwoServiceSpec struct {
	RayServiceSpec *RayServiceSpec `json:"rayServiceSpec,omitempty"`
	DeploymentSpec *DeploymentSpec `json:"deploymentSpec,omitempty"`
	Kueue          KueueSpec       `json:"kueue,omitempty"`
}

// KaiwoServiceStatus defines the observed state of KaiwoService
type KaiwoServiceStatus struct {
	Conditions []v1.Condition `json:"conditions,omitempty"`
}

// KaiwoService is the Schema for the kaiwoservices API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoService struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoServiceSpec   `json:"spec,omitempty"`
	Status KaiwoServiceStatus `json:"status,omitempty"`
}

// KaiwoServiceList contains a list of KaiwoService
// +kubebuilder:object:root=true
type KaiwoServiceList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []KaiwoService `json:"items"`
}
