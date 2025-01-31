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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoServiceSpec defines the desired state of KaiwoService.
type KaiwoServiceSpec struct {
	CommonMetaSpec `json:",inline"`

	// RayService configuration
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`

	// Optional workload-specific configs (Pointers to avoid bloating CRD)
	RayClusterSpec *RayClusterSpec `json:"rayClusterConfig,omitempty"`
	DeploymentSpec *DeploymentSpec `json:"deploymentSpec,omitempty"`
}

// KaiwoServiceStatus defines the observed state of KaiwoService.
type KaiwoServiceStatus struct {
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
	ReplicaStatuses map[string]int32   `json:"replicaStatuses,omitempty"`
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
