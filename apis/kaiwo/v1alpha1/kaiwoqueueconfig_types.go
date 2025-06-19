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
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

type QueueConfigStatusDescription string

// StatusReady indicates a KaiwoService is fully deployed and ready to serve requests (Deployment ready or RayService healthy). Not applicable to KaiwoJob.
const (
	QueueConfigStatusReady  QueueConfigStatusDescription = "READY"
	QueueConfigStatusFailed QueueConfigStatusDescription = "FAILED"
)

// KaiwoQueueConfigSpec defines the desired configuration for Kaiwo's management of Kueue resources.
// There should typically be only one KaiwoQueueConfig resource in the cluster, named 'kaiwo'.
type KaiwoQueueConfigSpec struct {
	// ClusterQueues defines a list of Kueue ClusterQueues that Kaiwo should manage. Kaiwo ensures these ClusterQueues exist and match the provided specs.
	// +kubebuilder:validation:MaxItems=20
	ClusterQueues []ClusterQueue `json:"clusterQueues,omitempty"`

	// ResourceFlavors defines a list of Kueue ResourceFlavors that Kaiwo should manage. Kaiwo ensures these ResourceFlavors exist and match the provided specs. If omitted or empty, Kaiwo attempts to automatically discover node pools and create default flavors based on node labels.
	// +kubebuilder:validation:MaxItems=20
	ResourceFlavors []ResourceFlavorSpec `json:"resourceFlavors,omitempty"`

	// WorkloadPriorityClasses defines a list of Kueue WorkloadPriorityClasses that Kaiwo should manage. Kaiwo ensures these priority classes exist with the specified values. See Kueue documentation for `WorkloadPriorityClass`.
	// +kubebuilder:validation:MaxItems=10
	WorkloadPriorityClasses []kueuev1beta1.WorkloadPriorityClass `json:"workloadPriorityClasses,omitempty"`

	// Topologies defines a list of Kueue Topologies that Kaiwo should manage. Kaiwo ensures these Topologies exist with the specified values. See Kueue documentation for `Topology`.
	// +kubebuilder:validation:MaxItems=10
	Topologies []Topology `json:"topologies,omitempty"`
}

// ClusterQueue defines the configuration for a Kueue ClusterQueue managed by Kaiwo.
type ClusterQueue struct {
	// Name specifies the name of the Kueue ClusterQueue resource.
	Name string `json:"name"`

	// Spec contains the desired Kueue `ClusterQueueSpec`. Kaiwo ensures the corresponding ClusterQueue resource matches this spec. See Kueue documentation for `ClusterQueueSpec` fields like `resourceGroups`, `cohort`, `preemption`, etc.
	Spec kueuev1beta1.ClusterQueueSpec `json:"spec,omitempty"`

	// Namespaces optionally lists Kubernetes namespaces where Kaiwo should automatically create a Kueue `LocalQueue` resource pointing to this ClusterQueue.
	// If one or more namespaces are provided, the KaiwoQueueConfig controller takes over managing the LocalQueues for this ClusterQueue.
	// Leave this empty if you want to be able to create your own LocalQueues for this ClusterQueue.
	Namespaces []string `json:"namespaces,omitempty"`
}

// ResourceFlavorSpec defines the configuration for a Kueue ResourceFlavor managed by Kaiwo.
type ResourceFlavorSpec struct {
	// Name specifies the name of the Kueue ResourceFlavor resource (e.g., "amd-mi300-8gpu").
	Name string `json:"name"`

	// NodeLabels specifies the labels that pods requesting this flavor must match on nodes. This is used by Kueue for scheduling decisions. Keys and values should correspond to actual node labels. Example: `{"kaiwo/nodepool": "amd-gpu-nodes"}`
	// +kubebuilder:validation:MaxProperties=10
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// Taints specifies a list of taints associated with this flavor.
	// +kubebuilder:validation:MaxItems=5
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Tolerations specifies a list of tolerations associated with this flavor. This is less common than using Taints; Kueue primarily uses Taints to derive Tolerations.
	// +kubebuilder:validation:MaxItems=5
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// TopologyName specifies the name of the Kueue Topology that this flavor belongs to. If specified, it must match one of the Topologies defined in the KaiwoQueueConfig.
	// This is used to group flavors by topology for scheduling purposes.
	TopologyName string `json:"topologyName,omitempty"`
}

// Topology is the Schema for the topology API
type Topology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec TopologySpec `json:"spec,omitempty"`
}

type TopologySpec struct {
	// levels define the levels of topology.
	// +required
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:validation:XValidation:rule="size(self.filter(i, size(self.filter(j, j == i)) > 1)) == 0",message="must be unique"
	// +kubebuilder:validation:XValidation:rule="size(self.filter(i, i.nodeLabel == 'kubernetes.io/hostname')) == 0 || self[size(self) - 1].nodeLabel == 'kubernetes.io/hostname'",message="the kubernetes.io/hostname label can only be used at the lowest level of topology"
	Levels []kueuev1alpha1.TopologyLevel `json:"levels,omitempty"`
}

// KaiwoQueueConfigStatus represents the observed state of KaiwoQueueConfig.
type KaiwoQueueConfigStatus struct {
	// Conditions lists the observed conditions of the KaiwoQueueConfig resource, such as whether the managed Kueue resources are synchronized and ready.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status reflects the overall status of the Kueue resource synchronization managed by this config (e.g., READY, FAILED).
	Status QueueConfigStatusDescription `json:"status,omitempty"`
}

// KaiwoQueueConfig manages Kueue resources like ClusterQueues, ResourceFlavors, and WorkloadPriorityClasses based on its spec. It acts as a central configuration point for Kaiwo's integration with Kueue. Typically, only one cluster-scoped resource named 'kaiwo' should exist. The controller ensures that the specified Kueue resources are created, updated, or deleted to match the desired state defined here.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="WorkloadStatus",type="string",JSONPath=".status.status"
// KaiwoQueueConfig manages Kueue resources.
type KaiwoQueueConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state for Kueue resources managed by Kaiwo.
	Spec KaiwoQueueConfigSpec `json:"spec,omitempty"`

	// Status reflects the most recently observed state of the Kueue resource synchronization.
	Status KaiwoQueueConfigStatus `json:"status,omitempty"`
}

// KaiwoQueueConfigList contains a list of KaiwoQueueConfig resources.
// +kubebuilder:object:root=true
type KaiwoQueueConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoQueueConfig `json:"items"`
}

// Register Kaiwo CRDs
func init() {
	SchemeBuilder.Register(&KaiwoQueueConfig{}, &KaiwoQueueConfigList{})
}
