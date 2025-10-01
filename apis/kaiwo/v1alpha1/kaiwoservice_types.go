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
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KaiwoServiceSpec defines the desired state of KaiwoService.
type KaiwoServiceSpec struct {
	CommonMetaSpec `json:",inline"`

	// EntryPoint specifies the command or script executed in a Deployment.
	// Can also be defined inside Deployment struct as regular command in the form of string array.
	//
	// It is *not* used when `ray: true` (use `serveConfigV2` or the `rayService` spec instead for Ray entrypoints).
	EntryPoint string `json:"entrypoint,omitempty"`

	// Defines the applications and deployments to deploy, should be a YAML multi-line scalar string.
	// Can also be defined inside RayService struct
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`

	// RayService allows providing a full `rayv1.RayService` spec.
	//
	// If present (or `spec.ray` is `true`), Kaiwo creates a `RayService` (wrapped in an AppWrapper for Kueue integration) instead of a `Deployment`.
	//
	// Common fields are merged into the `RayClusterSpec` within this spec.
	//
	// Allows fine-grained control over the Ray cluster and Ray Serve configurations.
	// +kubebuilder:pruning:PreserveUnknownFields
	RayService *rayv1.RayService `json:"rayService,omitempty"`

	// Deployment allows providing a full `appsv1.Deployment` spec.
	//
	// If present and `spec.ray` is `false`, this is used as the base for the created `Deployment`.
	//
	// Common fields are merged into this spec.
	//
	// Allows fine-grained control over Kubernetes Deployment parameters (strategy, selectors, pod template, etc.).
	// +kubebuilder:pruning:PreserveUnknownFields
	Deployment *appsv1.Deployment `json:"deployment,omitempty"`
}

// KaiwoServiceStatus defines the observed state of KaiwoService.
type KaiwoServiceStatus struct {
	CommonStatusSpec `json:",inline"`
}

// KaiwoService represents a long-running service workload managed by Kaiwo. It encapsulates either a standard Kubernetes Deployment  or a RayService (via an AppWrapper), along with common metadata, storage configurations, and scheduling preferences. The Kaiwo controller reconciles this resource to create and manage the underlying workload objects.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="StartTime",type="string",JSONPath=".status.startTime"
// +kubebuilder:printcolumn:name="Duration(s)",type="integer",JSONPath=".status.duration"
type KaiwoService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KaiwoService, including workload type (Deployment/RayService), configuration, resources, and common metadata.
	Spec KaiwoServiceSpec `json:"spec,omitempty"`

	// Status reflects the most recently observed state of the KaiwoService, including its phase, start time, duration, and conditions.
	Status KaiwoServiceStatus `json:"status,omitempty"`
}

// KaiwoServiceList
// +kubebuilder:object:root=true
type KaiwoServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoService `json:"items"`
}

func (spec *KaiwoServiceSpec) IsRayService() bool {
	return spec.RayService != nil || spec.Ray
}

func (svc *KaiwoService) GetKaiwoWorkloadObject() client.Object {
	return svc
}

func (svc *KaiwoService) GetCommonStatusSpec() *CommonStatusSpec {
	return &svc.Status.CommonStatusSpec
}

func (svc *KaiwoService) GetCommonSpec() CommonMetaSpec {
	return svc.Spec.CommonMetaSpec
}

func init() {
	SchemeBuilder.Register(&KaiwoService{}, &KaiwoServiceList{})
}
