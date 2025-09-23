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
	"strings"

	corev1 "k8s.io/api/core/v1"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

	// Aim contains the AIM (AMD Inference Microservice) configuration
	Aim *KaiwoAimDeploymentSpec `json:"aim,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!(has(self.ray) && has(self.deployment))",message="only one of 'ray' or 'deployment' may be set"

// KaiwoServiceRaySpec groups Ray-specific configuration for services
type KaiwoServiceRaySpec struct {
	// Spec is the RayService spec used to configure the Ray cluster and Serve.
	Spec rayv1.RayServiceSpec `json:"spec,omitempty"`

	// ServeConfigV2 allows providing the Ray Serve config as YAML string.
	// If set, it overrides `spec.serveConfigV2`.
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`
}

// KaiwoServiceDeploymentSpec groups Deployment-specific configuration for services
type KaiwoServiceDeploymentSpec struct {
	// Spec is the Kubernetes Deployment spec used to configure the service pods.
	Spec appsv1.DeploymentSpec `json:"spec,omitempty"`

	// EntryPoint specifies the command or script executed in the primary container.
	EntryPoint string `json:"entrypoint,omitempty"`
}

type KaiwoAimDeploymentSpec struct {
	// AimWorkloadId is a unique user-provided ID that identifies an instance of an AIM
	AimWorkloadId string `json:"aimWorkloadId,omitempty"`

	// Model contains the model-specific configuration
	Model AimModelSpec `json:"model,omitempty"`

	// Routing contains the routing-specific configuration
	Routing AimRoutingSpec `json:"routing,omitempty"`
}

type AimModelSpec struct {
	// Name refers to the AIM model name.
	Name string `json:"name,omitempty"`

	// Caching contains the model caching specifications
	Caching AimModelCachingSpec `json:"caching,omitempty"`
}

type AimRoutingSpec struct {
	// Enabled, if true, will create an HTTPRoute for the InferenceService.
	Enabled bool `json:"enabled,omitempty"`
}

type AimModelCachingSpec struct {
	// Enabled, if true, turns on model caching
	Enabled bool `json:"enabled,omitempty"`

	// StorageClass specifies the storage class to use for the model cache, if a model cache does not already exist. If this is omitted, the default storage class from `KaiwoConfig.spec.models.caching.storageClass` is used.
	StorageClass string `json:"storageClass,omitempty"`

	// Env lists the environmental variables required to download the model.
	Env []corev1.EnvVar `json:"env,omitempty"`
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
