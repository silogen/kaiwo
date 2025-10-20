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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PartitioningProfileSpec defines the desired state of PartitioningProfile.
type PartitioningProfileSpec struct {
	// DisplayName is a human-readable description of this profile.
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// TargetSelector is an optional guardrail to ensure the profile is only
	// applied to compatible nodes. If specified, the controller will validate
	// that nodes match this selector before applying the profile.
	// +optional
	TargetSelector *metav1.LabelSelector `json:"targetSelector,omitempty"`

	// ExpectedResources defines the resources that should appear in
	// node.status.allocatable after partitioning succeeds.
	// +optional
	ExpectedResources map[string]resource.Quantity `json:"expectedResources,omitempty"`

	// ProfileName is the name of the profile to be applied.
	// This is the name of the profile under `gpu-config-profile` in DCM config.json.
	ProfileName string `json:"profileName"`
}

// PartitioningProfileStatus defines the observed state of PartitioningProfile.
type PartitioningProfileStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the profile's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// PartitioningProfile is the Schema for the partitioningprofiles API.
// It defines a reusable GPU partition configuration that can be referenced by PartitioningPlans.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=gpuprofile;pp,categories=infrastructure;gpu
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PartitioningProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PartitioningProfileSpec   `json:"spec,omitempty"`
	Status PartitioningProfileStatus `json:"status,omitempty"`
}

// PartitioningProfileList contains a list of PartitioningProfile.
// +kubebuilder:object:root=true
type PartitioningProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PartitioningProfile `json:"items"`
}

// GetStatus returns a pointer to the PartitioningProfile status.
func (p *PartitioningProfile) GetStatus() *PartitioningProfileStatus {
	return &p.Status
}

func init() {
	SchemeBuilder.Register(&PartitioningProfile{}, &PartitioningProfileList{})
}
