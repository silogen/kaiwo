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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoNodeSpec defines the desired state of Kaiwo node.
type KaiwoNodeSpec struct {
	// Partitioning configures the node's partitioning
	Partitioning PartitioningConfig `json:"partitioning,omitempty"`
}

type PartitioningConfig struct {
	// Enabled enforces partitioning on the node's GPUs. Note that this switches the partitioning changes on or off. If partitioning is first enabled, and then disabled, any changes that were made will persist, but any changes made to the partitioning profile while partitioning is disabled will not take effect.
	Enabled bool `json:"enabled,omitempty"`

	// Profile is the partitioning profile to apply.
	Profile GpuPartitioningProfile `json:"profile,omitempty"`
}

type GpuPartitioningProfile string

const (
	GpuPartitioningProfileAmdSpx GpuPartitioningProfile = "spx"
	GpuPartitioningProfileAmdCpx GpuPartitioningProfile = "cpx"
)

// KaiwoNodeStatus defines the observed state of KaiwoNode.
type KaiwoNodeStatus struct {
	// KaiwoNodeStatus reports the current status for the node
	// +kubebuilder:default=Unknown
	Status KaiwoNodeStatusType `json:"status,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// NodeType reports the type of the node
	NodeType NodeType `json:"nodeType,omitempty"`

	// Resources reports the resources that the node currently has
	Resources NodeResources `json:"resources,omitempty"`

	// KueueFlavorName is the name of the flavor that Kueue uses for this node
	KueueFlavorName string `json:"kueueFlavorName,omitempty"`

	Unschedulable bool `json:"unschedulable,omitempty"`

	IsControlPlane bool `json:"isControlPlane,omitempty"`

	Partitioning PartitioningStatus `json:"partitioning,omitempty"`
}

type KaiwoNodeStatusType string

const (
	KaiwoNodeStatusNotReady     KaiwoNodeStatusType = "NotReady"
	KaiwoNodeStatusReady        KaiwoNodeStatusType = "Ready"
	KaiwoNodeStatusUnknown      KaiwoNodeStatusType = "Unknown"
	KaiwoNodeStatusError        KaiwoNodeStatusType = "Error"
	KaiwoNodeStatusPartitioning KaiwoNodeStatusType = "Partitioning"
)

type PartitioningConditionReason string

const (
	PartitioningCompletedConditionType string                      = "PartitioningCompleted"
	PartitioningConditionInProgress    PartitioningConditionReason = "InProgress"
	PartitioningConditionComplete      PartitioningConditionReason = "Complete"
	PartitioningConditionFailed        PartitioningConditionReason = "Failed"
)

type NodeType string

const (
	NodeTypeGpu NodeType = "gpu"
	NodeTypeCpu NodeType = "cpu-only"
)

type NodeResources struct {
	Cpu    NominalResourceWrapper `json:"cpu,omitempty"`
	Memory NominalResourceWrapper `json:"memory,omitempty"`
	Gpus   *NodeGpuInfo           `json:"gpus,omitempty"`
}

type NominalResourceWrapper struct {
	Actual  *resource.Quantity `json:"actual,omitempty"`
	Nominal *resource.Quantity `json:"nominal,omitempty"`
}

type NodeGpuInfo struct {
	// Mandatory fields, parsing a node will fail if the following fields cannot be extracted

	// Vendor is the vendor of the GPU (primarily `amd` or `nvidia`)
	Vendor GpuVendor `json:"vendor,omitempty"`

	// Model is the model name of the GPU
	Model string `json:"model,omitempty"`

	// ResourceName is the GPU resource name for this GPU
	ResourceName v1.ResourceName `json:"resourceName,omitempty"`

	// LogicalCount is the number of logical GPUs in the node and is taken from the Kubernetes node status capacity. If the GPUs are not partitioned, this is the same as PhysicalCount
	LogicalCount int `json:"logicalCount,omitempty"`

	// PhysicalCount is the number of physical GPUs in the node
	PhysicalCount *int `json:"physicalCount,omitempty"`

	// PhysicalVramPerGpu is the vRAM that each physical GPU has.
	PhysicalVramPerGpu *resource.Quantity `json:"physicalVramPerGpu,omitempty"`

	// LogicalVramPerGpu is the vRAM that each logical GPU sees. If the node's GPUs are not partitioned, this is the
	// total vRAM that each GPU has. If the node's GPUs are partitioned, this is the vRAM that each partition has.
	LogicalVramPerGpu *resource.Quantity `json:"logicalVramPerGpu,omitempty"`

	// IsPartitioned reports whether the GPUs are currently partitioned or not. A null value implicates that it is unclear
	// whether the GPUs are partitioned or not
	IsPartitioned *bool `json:"isPartitioned,omitempty"`
}

type PartitioningStatus struct {
	// AppliedProfile reports the currently active profile
	AppliedProfile *GpuPartitioningProfile `json:"appliedProfile,omitempty"`

	// Phase reports the current partitioning phase
	Phase PartitioningPhase `json:"phase,omitempty"`
}

type PartitioningPhase string

const (
	PartitioningPhaseDrainingUntoleratedPods  PartitioningPhase = "DrainingUntoleratedPods"
	PartitioningPhaseApplyingPartitions       PartitioningPhase = "ApplyingPartitions"
	PartitioningPhaseWaitingForResourceUpdate PartitioningPhase = "WaitingForResourceUpdate"
	PartitioningPhaseRestartingPods           PartitioningPhase = "RestartingPods"
	PartitioningPhaseWaitingForLabels         PartitioningPhase = "WaitingForLabels"
	PartitioningPhaseCompleted                PartitioningPhase = "Completed"
	PartitioningPhaseError                    PartitioningPhase = "Error"
)

// KaiwoNode represents a proxy that Kaiwo uses to interact with and report on node statuses.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="GPU partitioning",type="string",JSONPath=".status.partitioning.appliedProfile"
// +kubebuilder:printcolumn:name="Flavor",type="string",JSONPath=".status.kueueFlavorName"
type KaiwoNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KaiwoNode.
	Spec KaiwoNodeSpec `json:"spec,omitempty"`

	// Status reflects the most recently observed state of the KaiwoNode.
	Status KaiwoNodeStatus `json:"status,omitempty"`
}

func (n *KaiwoNode) IsUnschedulable() bool {
	return n.Status.Status != KaiwoNodeStatusReady
}

func (n *KaiwoNode) IsControlPlane() bool {
	return n.Status.IsControlPlane
}

func (n *KaiwoNode) IsCpuOnlyNode() bool {
	return n.Status.NodeType == NodeTypeCpu
}

// KaiwoNodeList
// +kubebuilder:object:root=true
type KaiwoNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoNode{}, &KaiwoNodeList{})
}
