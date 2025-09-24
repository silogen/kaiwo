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

// ClusterModelSpec defines the desired state of ClusterModel
type ClusterModelSpec struct {
	// Aim is the
	Aim AimClusterModelSpec `json:"aim,omitempty"`
}

type AimClusterModelSpec struct {
	// Name is the identifying name of the AIM model
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Image is the URI of the image that is used when deploying this AIM
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ImagePullSecret is the name of the secret that is used to pull the Image, if required
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
}

// ClusterModelStatus defines the observed state of ClusterModel
type ClusterModelStatus struct {
	// Conditions represent the latest available observations of the cluster model's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Model Name",type=string,JSONPath=`.spec.name`

// ClusterModel is the Schema for the clustermodels API
type ClusterModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterModelSpec   `json:"spec,omitempty"`
	Status ClusterModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterModelList contains a list of ClusterModel
type ClusterModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterModel{}, &ClusterModelList{})
}
