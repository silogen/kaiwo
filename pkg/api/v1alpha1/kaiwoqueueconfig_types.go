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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceFlavorSpec represents the configuration for a ResourceFlavor.
type ResourceFlavorSpec struct {
	Name       string            `json:"name"`       // Name of the resource flavor
	NodeLabels map[string]string `json:"nodeLabels"` // Node labels associated with this flavor

	// Taints applied to nodes with this flavor
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Tolerations for workloads that should be scheduled on nodes with this flavor
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// WorkloadPriorityClassSpec represents the configuration for a WorkloadPriorityClass.
type WorkloadPriorityClassSpec struct {
	Name        string `json:"name"`        // Priority class name
	Value       int32  `json:"value"`       // Numeric priority value
	Description string `json:"description"` // Optional description
}

// ResourceQuota represents the quota for a specific resource.
type ResourceQuota struct {
	ResourceName string `json:"resourceName"` // e.g., "cpu", "memory", "gpu"
	NominalQuota string `json:"nominalQuota"` // e.g., "10", "50Gi"
}

// ClusterQueueSpec represents the configuration of a ClusterQueue.
type ClusterQueueSpec struct {
	Name              string            `json:"name"`                        // Name of the cluster queue
	NamespaceSelector map[string]string `json:"namespaceSelector,omitempty"` // Namespace selector
	ResourceGroups    []ResourceGroup   `json:"resourceGroups"`              // Resource group definitions
}

// ResourceGroup defines a group of covered resources and their available flavors.
type ResourceGroup struct {
	CoveredResources []string              `json:"coveredResources"` // List of covered resources, e.g., ["cpu", "memory"]
	Flavors          []FlavorResourceQuota `json:"flavors"`          // Mapping of flavors to quotas
}

// FlavorResourceQuota maps a flavor to a set of resource quotas.
type FlavorResourceQuota struct {
	Name      string          `json:"name"`      // Flavor name
	Resources []ResourceQuota `json:"resources"` // List of resource quotas
}

// KaiwoQueueConfigSpec defines the desired state of KaiwoQueueConfig.
type KaiwoQueueConfigSpec struct {
	ClusterQueues           []ClusterQueueSpec          `json:"clusterQueues,omitempty"`           // List of ClusterQueues
	ResourceFlavors         []ResourceFlavorSpec        `json:"resourceFlavors,omitempty"`         // List of ResourceFlavors
	WorkloadPriorityClasses []WorkloadPriorityClassSpec `json:"workloadPriorityClasses,omitempty"` // List of WorkloadPriorityClasses
}

// KaiwoQueueConfigStatus defines the observed state of KaiwoQueueConfig.
type KaiwoQueueConfigStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoQueueConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoQueueConfigSpec   `json:"spec,omitempty"`
	Status KaiwoQueueConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type KaiwoQueueConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoQueueConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoQueueConfig{}, &KaiwoQueueConfigList{})
}
