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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlanReference references the parent PartitioningPlan.
type PlanReference struct {
	// Name of the PartitioningPlan.
	Name string `json:"name"`

	// UID of the PartitioningPlan (for strong ownership).
	// +optional
	UID string `json:"uid,omitempty"`
}

// NodePartitioningSpec defines the desired state of NodePartitioning.
type NodePartitioningSpec struct {
	// PlanRef references the parent PartitioningPlan.
	PlanRef PlanReference `json:"planRef"`

	// DryRun indicates whether this is a dry-run operation.
	// When true, the controller will skip all actual operations and set phase to Skipped.
	// +optional
	DryRun bool `json:"dryRun,omitempty"`

	// NodeName is the name of the target node.
	// +kubebuilder:validation:MinLength=1
	NodeName string `json:"nodeName"`

	// DesiredHash is a deterministic hash of the desired partition state.
	// Used for idempotency and change detection.
	// +kubebuilder:validation:MinLength=1
	DesiredHash string `json:"desiredHash"`

	// Profile defines the partitioning profile to apply to the node.
	Profile PartitioningProfileSpec `json:"profile"`
}

// NodePartitioningStatus defines the observed state of NodePartitioning.
type NodePartitioningStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase represents the current phase in the state machine.
	// +optional
	Phase NodePartitioningPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the operation's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentHash is the hash of the current applied state.
	// +optional
	CurrentHash string `json:"currentHash,omitempty"`
}

// NodePartitioning is the Schema for the nodepartitionings API.
// It represents a per-node work item for applying a partition profile.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=nodepartition;np,categories=infrastructure;gpu
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profile.dcmProfileName`
// +kubebuilder:printcolumn:name="Attempts",type=integer,JSONPath=`.status.attempts`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type NodePartitioning struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePartitioningSpec   `json:"spec,omitempty"`
	Status NodePartitioningStatus `json:"status,omitempty"`
}

// NodePartitioningList contains a list of NodePartitioning.
// +kubebuilder:object:root=true
type NodePartitioningList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePartitioning `json:"items"`
}

// GetStatus returns a pointer to the NodePartitioning status.
func (n *NodePartitioning) GetStatus() *NodePartitioningStatus {
	return &n.Status
}

func init() {
	SchemeBuilder.Register(&NodePartitioning{}, &NodePartitioningList{})
}
