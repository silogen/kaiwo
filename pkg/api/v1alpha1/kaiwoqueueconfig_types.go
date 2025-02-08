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
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// ResourceFlavorSpec defines the configuration for a specific resource flavor.
type ResourceFlavorSpec struct {
	Name        string              `json:"name"`
	NodeLabels  map[string]string   `json:"nodeLabels"`
	Taints      []corev1.Taint      `json:"taints,omitempty"`
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// WorkloadPriorityClassSpec defines the priority class configuration for workloads.
type WorkloadPriorityClassSpec struct {
	Name        string `json:"name"`
	Value       int32  `json:"value"`
	Description string `json:"description"`
}

// KaiwoQueueConfigSpec defines the desired configuration for Kaiwo.
type KaiwoQueueConfigSpec struct {
	ClusterQueues           []kueuev1beta1.ClusterQueue   `json:"clusterQueues,omitempty"`
	ResourceFlavors         []kueuev1beta1.ResourceFlavor `json:"resourceFlavors,omitempty"`
	WorkloadPriorityClasses []WorkloadPriorityClassSpec   `json:"workloadPriorityClasses,omitempty"`
}

// KaiwoQueueConfigStatus represents the observed state of KaiwoQueueConfig.
type KaiwoQueueConfigStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// KaiwoQueueConfig manages Kueue resources.
type KaiwoQueueConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KaiwoQueueConfigSpec   `json:"spec,omitempty"`
	Status            KaiwoQueueConfigStatus `json:"status,omitempty"`
}

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
