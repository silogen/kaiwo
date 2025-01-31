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

// ResourceFlavorSpec defines the configuration for a specific resource flavor.
// A resource flavor describes a particular type of node with specific labels,
// taints, and tolerations.
type ResourceFlavorSpec struct {
	// Name specifies the name of the resource flavor.
	Name string `json:"name"`

	// NodeLabels defines labels associated with nodes that match this flavor.
	NodeLabels map[string]string `json:"nodeLabels"`

	// Taints lists taints applied to nodes with this flavor.
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Tolerations defines the tolerations required for workloads to be scheduled on nodes with this flavor.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// WorkloadPriorityClassSpec defines the priority class configuration for workloads.
// Priority classes influence the scheduling order of workloads.
type WorkloadPriorityClassSpec struct {
	// Name is the name of the priority class.
	Name string `json:"name"`

	// Value represents the numeric priority value assigned to this priority class.
	Value int32 `json:"value"`

	// Description provides an optional explanation for this priority class.
	Description string `json:"description"`
}

// ResourceQuota defines the quota allocation for a specific resource within a flavor or cluster queue.
type ResourceQuota struct {
	// ResourceName specifies the type of resource (e.g., "cpu", "memory", "gpu").
	ResourceName string `json:"resourceName"`

	// NominalQuota defines the quantity of the resource allocated (e.g., "10", "50Gi").
	NominalQuota string `json:"nominalQuota"`
}

// ClusterQueueSpec defines the configuration for a ClusterQueue.
// A ClusterQueue determines how workloads are scheduled across available resources.
type ClusterQueueSpec struct {
	// Name specifies the name of the cluster queue.
	Name string `json:"name"`

	// NamespaceSelector defines which namespaces are eligible for scheduling in this cluster queue.
	NamespaceSelector map[string]string `json:"namespaceSelector,omitempty"`

	// ResourceGroups contains definitions for resource allocations within the cluster queue.
	ResourceGroups []ResourceGroup `json:"resourceGroups"`
}

// ResourceGroup defines a grouping of resources and their associated flavors within a cluster queue.
type ResourceGroup struct {
	// CoveredResources lists the types of resources managed in this group (e.g., ["cpu", "memory"]).
	CoveredResources []string `json:"coveredResources"`

	// Flavors specifies the available resource flavors and their associated quotas.
	Flavors []FlavorResourceQuota `json:"flavors"`
}

// FlavorResourceQuota associates a resource flavor with specific quotas.
type FlavorResourceQuota struct {
	// Name specifies the name of the resource flavor.
	Name string `json:"name"`

	// Resources defines the resource quotas allocated to this flavor.
	Resources []ResourceQuota `json:"resources"`
}

// KaiwoQueueConfigSpec defines the desired configuration for a KaiwoQueueConfig.
// It includes multiple cluster queues, resource flavors, and workload priority classes.
type KaiwoQueueConfigSpec struct {
	// ClusterQueues is a list of ClusterQueue specifications that define scheduling policies.
	ClusterQueues []ClusterQueueSpec `json:"clusterQueues,omitempty"`

	// ResourceFlavors specifies the different node flavors available in the cluster.
	ResourceFlavors []ResourceFlavorSpec `json:"resourceFlavors,omitempty"`

	// WorkloadPriorityClasses defines the priority classes available for workloads.
	WorkloadPriorityClasses []WorkloadPriorityClassSpec `json:"workloadPriorityClasses,omitempty"`
}

// KaiwoQueueConfigStatus represents the current observed state of KaiwoQueueConfig.
type KaiwoQueueConfigStatus struct {
	// Conditions provide information on the status of the KaiwoQueueConfig.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// KaiwoQueueConfig is the API resource that defines the configuration for queue management
// within the Kaiwo Operator. It specifies cluster queues, resource flavors, and priority classes.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoQueueConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired configuration for the queue system.
	Spec KaiwoQueueConfigSpec `json:"spec,omitempty"`

	// Status represents the observed state of the queue configuration.
	Status KaiwoQueueConfigStatus `json:"status,omitempty"`
}

// KaiwoQueueConfigList is a list of KaiwoQueueConfig resources.
//
// +kubebuilder:object:root=true
type KaiwoQueueConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items contains multiple KaiwoQueueConfig resources.
	Items []KaiwoQueueConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoQueueConfig{}, &KaiwoQueueConfigList{})
}
