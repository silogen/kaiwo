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

// PartitioningPlanSpec defines the desired state of PartitioningPlan.
type PartitioningPlanSpec struct {
	// Paused indicates whether the plan should stop reconciling.
	// When true, no new node operations will be started, but status updates continue.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// DryRun indicates whether to simulate the plan without making any changes.
	// When true, NodePartitioning resources are created but marked as Skipped.
	// +optional
	DryRun bool `json:"dryRun,omitempty"`

	// Rollout defines how to orchestrate node changes.
	Rollout RolloutPolicy `json:"rollout,omitempty"`

	// Rules defines the rules mapping nodes to partition profiles.
	// +kubebuilder:validation:MinItems=1
	Rules []PartitioningRule `json:"rules"`
}

// PartitioningPlanStatus defines the observed state of PartitioningPlan.
type PartitioningPlanStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase represents the overall phase of the plan.
	// +optional
	Phase PartitioningPlanPhase `json:"phase,omitempty"`

	// Summary aggregates node counts by phase.
	// +optional
	Summary PlanSummary `json:"summary,omitempty"`

	// Conditions represent the latest available observations of the plan's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the timestamp of the last successful reconciliation.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// NodeStatuses is a lightweight cache of per-node status for dashboards.
	// The source of truth is in the NodePartitioning resources.
	// +optional
	NodeStatuses []NodeStatusSummary `json:"nodeStatuses,omitempty"`
}

// PartitioningPlan is the Schema for the partitioningplans API.
// It orchestrates the application of partition profiles across a set of nodes.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=gpuplan;plan,categories=infrastructure;gpu
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Succeeded",type=integer,JSONPath=`.status.summary.succeeded`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.summary.failed`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.summary.matchingNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PartitioningPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PartitioningPlanSpec   `json:"spec,omitempty"`
	Status PartitioningPlanStatus `json:"status,omitempty"`
}

// PartitioningPlanList contains a list of PartitioningPlan.
// +kubebuilder:object:root=true
type PartitioningPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PartitioningPlan `json:"items"`
}

// GetStatus returns a pointer to the PartitioningPlan status.
func (p *PartitioningPlan) GetStatus() *PartitioningPlanStatus {
	return &p.Status
}

func init() {
	SchemeBuilder.Register(&PartitioningPlan{}, &PartitioningPlanList{})
}
