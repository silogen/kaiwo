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
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// KaiwoConfigSpec defines the desired configuration for Kaiwo's management of Kueue resources.
// There should typically be only one KaiwoConfig resource in the cluster, named 'kaiwo'.
type KaiwoConfigSpec struct {
	Kueue KaiwoKueueConfig `json:"kueue"`

	// ENFORCE_KAIWO_ON_GPU_WORKLOADS false
	// EnforceKaiwoOnGpuWorkloads bool `json:"enforceKaiwoOnGpuWorkloads"`

	// amd.com/gpu
	DefaultGpuResourceKey string `json:"defaultGpuResourceKey,omitempty"`

	Ray KaiwoRayConfig `json:"ray,omitempty"`

	Data KaiwoDataConfig `json:"data,omitempty"`

	// default-scheduler
	KubeSchedulerName string `json:"kube-scheduler-name"`

	// kaiwo.silogen.ai/gpu
	DefaultGpuTaintKey string `json:"defaultGpuTaintKey,omitempty"`

	// kaiwo
	DefaultKaiwoQueueConfigName string `json:"defaultKaiwoQueueConfigName,omitempty"`

	// ClusterQueues defines a list of Kueue ClusterQueues that Kaiwo should manage. Kaiwo ensures these ClusterQueues exist and match the provided specs.
	// +kubebuilder:validation:MaxItems=10
	ClusterQueues []ClusterQueue `json:"clusterQueues,omitempty"`

	// ResourceFlavors defines a list of Kueue ResourceFlavors that Kaiwo should manage. Kaiwo ensures these ResourceFlavors exist and match the provided specs. If omitted or empty, Kaiwo attempts to automatically discover node pools and create default flavors based on node labels.
	ResourceFlavors []ResourceFlavorSpec `json:"resourceFlavors,omitempty"`

	// WorkloadPriorityClasses defines a list of Kueue WorkloadPriorityClasses that Kaiwo should manage. Kaiwo ensures these priority classes exist with the specified values. See Kueue documentation for `WorkloadPriorityClass`.
	// +kubebuilder:validation:MaxItems=5
	WorkloadPriorityClasses []kueuev1beta1.WorkloadPriorityClass `json:"workloadPriorityClasses,omitempty"`
}

type KaiwoRayConfig struct {
	// ghcr.io/silogen/rocm-ray:v0.8
	DefaultRayImage string `json:"defaultRayImage,omitempty"`

	HeadPodMemory string `json:"headPodMemory,omitempty"`
}

type KaiwoKueueConfig struct {
	// false
	ExcludeMasterNodesFromNodePools bool `json:"excludeMasterNodesFromNodePools,omitempty"`

	// true
	AddTaintsToGpuNodes bool `json:"addTaintsToGpuNodes,omitempty"`

	// kaiwo
	DefaultClusterQueueName string `json:"defaultClusterQueueName,omitempty"`
}

type KaiwoDataConfig struct {
	// /workload
	DefaultDataMountPath string `json:"defaultDataMountPath,omitempty"`

	// /hf_cache
	DefaultHfMountPath string `json:"defaultHfMountPath,omitempty"`
}

// KaiwoConfig manages Kueue resources like ClusterQueues, ResourceFlavors, and WorkloadPriorityClasses based on its spec. It acts as a central configuration point for Kaiwo's integration with Kueue. Typically, only one cluster-scoped resource named 'kaiwo' should exist. The controller ensures that the specified Kueue resources are created, updated, or deleted to match the desired state defined here.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type KaiwoConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state for Kueue resources managed by Kaiwo.
	Spec KaiwoConfigSpec `json:"spec,omitempty"`
}

// KaiwoConfigList contains a list of KaiwoConfig resources.
// +kubebuilder:object:root=true
type KaiwoConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoConfig `json:"items"`
}

// Register Kaiwo CRDs
func init() {
	SchemeBuilder.Register(&KaiwoConfig{}, &KaiwoConfigList{})
}
