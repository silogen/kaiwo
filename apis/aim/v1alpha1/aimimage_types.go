// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIMImageSpec defines the desired state of AIMImage.
type AIMImageSpec struct {
	// ModelID is the ID name (includes version/revision).
	// Example: `meta/llama-3-8b:1.1+20240915`.
	// +kubebuilder:validation:MinLength=1
	ModelID string `json:"modelId"`

	// Image is the container image URI for this AIM model.
	// This image is inspected by the operator to select runtime profiles used by templates.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// DefaultServiceTemplate is the name of the default service template to use, if an
	// AIMService is created without specifying a template name.
	DefaultServiceTemplate string `json:"defaultServiceTemplate"`
}

// AIMImageStatus defines the observed state of AIMImage.
type AIMImageStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the model's state
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimclimg,categories=aim;all
// +kubebuilder:printcolumn:name="Model ID",type=string,JSONPath=`.spec.modelId`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMClusterImage is the Schema for cluster-scoped AIM image catalog entries.
type AIMClusterImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMImageSpec   `json:"spec,omitempty"`
	Status AIMImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMClusterImageList contains a list of AIMClusterImage.
type AIMClusterImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterImage `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimimg,categories=aim;all
// +kubebuilder:printcolumn:name="Model ID",type=string,JSONPath=`.spec.modelId`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMImage is the Schema for namespace-scoped AIM image catalog entries.
type AIMImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMImageSpec   `json:"spec,omitempty"`
	Status AIMImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMImageList contains a list of AIMImage.
type AIMImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterImage{}, &AIMClusterImageList{}, &AIMImage{}, &AIMImageList{})
}
