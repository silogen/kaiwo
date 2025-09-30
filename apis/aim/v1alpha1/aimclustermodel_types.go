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

// AIMClusterModelSpec defines the desired state of AIMClusterModel.
type AIMClusterModelSpec struct {
	// Name is the canonical model name (includes version/revision).
	// Example: `meta/llama-3-8b:1.1+20240915`.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Image is the container image URI for this AIM model.
	// This image is inspected by the operator to select runtime profiles used by templates.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
}

// AIMClusterModelStatus defines the observed state of AIMClusterModel.
type AIMClusterModelStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the model's state
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimclm;aimclmodel,categories=kaiwo;all
// +kubebuilder:printcolumn:name="Model Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMClusterModel is the Schema for cluster-scoped AIM model catalog entries.
type AIMClusterModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMClusterModelSpec   `json:"spec,omitempty"`
	Status AIMClusterModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMClusterModelList contains a list of AIMClusterModel.
type AIMClusterModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterModel{}, &AIMClusterModelList{})
}
