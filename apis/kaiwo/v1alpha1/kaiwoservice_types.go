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
	"strings"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:validation:XValidation:rule="!(has(self.ray) && has(self.deployment))",message="only one of 'ray' or 'deployment' may be set"

// KaiwoServiceSpec defines the desired state of KaiwoService.
type KaiwoServiceSpec struct {
	CommonMetaSpec `json:",inline"`

	// Type selects the underlying workload implementation. Valid values: "deployment", "ray".
	// +kubebuilder:validation:Enum=deployment;ray;aim
	Type string `json:"type,omitempty"`

	// Ray contains Ray-specific configuration. The primary underlying RayService
	// spec lives under `.ray.spec`, while additional Kaiwo-specific Ray options
	// can be added as siblings (e.g., `serveConfigV2`).
	Ray *KaiwoServiceRaySpec `json:"ray,omitempty"`

	// Deployment contains Deployment-specific configuration. The primary
	// underlying Deployment spec lives under `.deployment.spec`, while
	// additional Kaiwo-specific options can be added as siblings (e.g., `entrypoint`).
	Deployment *KaiwoServiceDeploymentSpec `json:"deployment,omitempty"`
}

// KaiwoServiceRaySpec groups Ray-specific configuration for services
type KaiwoServiceRaySpec struct {
	// Spec is the RayService spec used to configure the Ray cluster and Serve.
	Spec *rayv1.RayServiceSpec `json:"spec,omitempty"`

	// ServeConfigV2 allows providing the Ray Serve config as YAML string.
	// If set, it overrides `spec.serveConfigV2`.
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`
}

// KaiwoServiceDeploymentSpec groups Deployment-specific configuration for services
type KaiwoServiceDeploymentSpec struct {
	// Spec is the Kubernetes Deployment spec used to configure the service pods.
	Spec *appsv1.DeploymentSpec `json:"spec,omitempty"`

	// EntryPoint specifies the command or script executed in the primary container.
	EntryPoint string `json:"entrypoint,omitempty"`
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
	t := strings.ToLower(spec.Type)
	if t == "ray" {
		return true
	}
	if t == "" && spec.Ray != nil {
		return true
	}
	return false
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
