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
)

// KaiwoConfigSpec defines the desired configuration for the Kaiwo operator's configuration.
// There should typically be only one KaiwoConfig resource in the cluster.
type KaiwoConfigSpec struct {
	// ENFORCE_KAIWO_ON_GPU_WORKLOADS false
	// EnforceKaiwoOnGpuWorkloads bool `json:"enforceKaiwoOnGpuWorkloads"`

	// Ray defines the Ray-specific settings
	// +kubebuilder:default={}
	Ray KaiwoRayConfig `json:"ray,omitempty"`

	// Storage defines the storage-specific settings
	// +kubebuilder:default={}
	Storage KaiwoStorageConfig `json:"storage,omitempty"`

	// Nodes defines the node configuration settings
	// +kubebuilder:default={}
	Nodes KaiwoNodeConfig `json:"nodes,omitempty"`

	// Scheduling contains the configuration Kaiwo uses for workload scheduling
	// +kubebuilder:default={}
	Scheduling KaiwoSchedulingConfig `json:"scheduling,omitempty"`

	// ResourceMonitoring defines the resource-monitoring specific settings
	// +kubebuilder:default={}
	ResourceMonitoring KaiwoResourceMonitoringConfig `json:"resourceMonitoring,omitempty"`

	// DefaultClusterQueueName is the name of the default cluster queue that is used for workloads that don't explicitly specify a cluster queue.
	// +kubebuilder:default="kaiwo"
	DefaultClusterQueueName string `json:"defaultClusterQueueName,omitempty"`

	// DefaultClusterQueueCohortName is the name of the default cohort that is used for the default cluster queue.
	// ClusterQueues in the same cohort can share resources.
	// +kubebuilder:default="kaiwo"
	DefaultClusterQueueCohortName string `json:"defaultClusterQueueCohortName,omitempty"`

	// DynamicallyUpdateDefaultClusterQueue defines whether the Kaiwo operator should dynamically update default "kaiwo" clusterqueue.
	// If set to true, the operator will make sure that the default clusterqueue is always up to date and reflects total resources available.
	// If nodes are added or removed, the operator will update the default clusterqueue to reflect the current state of the cluster.
	// +kubebuilder:default=false
	DynamicallyUpdateDefaultClusterQueue bool `json:"dynamicallyUpdateDefaultClusterQueue,omitempty"`
}

// KaiwoRayConfig contains the Ray-specific configuration that Kaiwo uses.
type KaiwoRayConfig struct {
	// DefaultRayImage is the image that is used for Ray workloads if no image is provided in the workload CRD
	// +kubebuilder:default="ghcr.io/silogen/rocm-ray:6.4"
	DefaultRayImage string `json:"defaultRayImage,omitempty"`

	// HeadPodMemory is the amount of memory that is requested for the Ray head pod
	// +kubebuilder:default="16Gi"
	HeadPodMemory string `json:"headPodMemory,omitempty"`
}

type KaiwoStorageConfig struct {
	// DefaultStorageClass is the storage class that is used for workloads that don't explicitly specify a storage class.
	DefaultStorageClass string `json:"defaultStorageClass,omitempty"`

	// DefaultDataMountPath is the default path for the data storage and downloads that gets mounted in the workload pods.
	// This value can be overwritten in the workload CRD.
	// +kubebuilder:default="/workload"
	DefaultDataMountPath string `json:"defaultDataMountPath,omitempty"`

	// DefaultHfMountPath is the default path for the HuggingFace that gets mounted in the workload pods. The `HF_HOME` environmental variable
	// is also set to this value. This value can be overwritten in the workload CRD.
	// +kubebuilder:default="/hf_cache"
	DefaultHfMountPath string `json:"defaultHfMountPath,omitempty"`
}

type KaiwoNodeConfig struct {
	// DefaultGpuResourceKey defines the default GPU resource key that is used to reserve GPU capacity for pods
	// +kubebuilder:default="amd.com/gpu"
	DefaultGpuResourceKey string `json:"defaultGpuResourceKey,omitempty"`

	// DefaultGpuTaintKey is the key that is used to taint GPU nodes
	// +kubebuilder:default="kaiwo.silogen.ai/gpu"
	DefaultGpuTaintKey string `json:"defaultGpuTaintKey,omitempty"`

	// ExcludeMasterNodesFromNodePools allows excluding the master node(s) from the node pools
	// +kubebuilder:default=false
	ExcludeMasterNodesFromNodePools bool `json:"excludeMasterNodesFromNodePools,omitempty"`

	// AddTaintsToGpuNodes if set to true, will add the DefaultGpuTaintKey taint to the GPU nodes
	// +kubebuilder:default=false
	AddTaintsToGpuNodes bool `json:"addTaintsToGpuNodes,omitempty"`
}

// KaiwoResourceMonitoringConfig configures the resource monitoring feature.
// Note that the following must be set as environmental variables inside the Kaiwo controller manager as these cannot be updated without restarting the operator process.
//
// * Enabling the resource monitoring feature (`RESOURCE_MONITORING_ENABLED=true`)
// * Setting the metrics endpoint (`RESOURCE_MONITORING_METRICS_ENDPOINT=...`)
// * Setting the polling interval (`RESOURCE_MONITORING_POLLING_INTERVAL=30s`)
type KaiwoResourceMonitoringConfig struct {
	// LowUtilizationThreshold is the threshold which, if the metric goes under, the workload is considered underutilized. The threshold is interpreted as the percentage utilization versus the requested capacity.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	LowUtilizationThreshold float64 `json:"lowUtilizationThreshold,omitempty"`

	// TargetNamespaces is a list of namespaces to apply the monitoring to. If not supplied or empty, all namespaces apart from kube-system will be inspected. However, only pods associated with KaiwoJobs or KaiwoServices are impacted.
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// Profile chooses the target resource to monitor.
	// +kubebuilder:validation:Enum="gpu"
	// +kubebuilder:default="gpu"
	Profile string `json:"profile,omitempty"`

	// TerminateUnderutilized will terminate workloads that are underutilizing resources if set to `true`
	// +kubebuilder:default=false
	TerminateUnderutilized bool `json:"terminateUnderutilized,omitempty"`

	// TerminateUnderutilizedAfter specifies the duration after which the workload will be terminated if it has been underutilizing resources (for this amount of time)
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +kubebuilder:default="24h"
	TerminateUnderutilizedAfter string `json:"terminateUnderutilizedAfter,omitempty"`
}

// KaiwoSchedulingConfig contains the configuration Kaiwo uses for workload scheduling
type KaiwoSchedulingConfig struct {
	// KubeSchedulerName defines the default scheduler name that is used to schedule the workload
	// +kubebuilder:default="kaiwo-scheduler"
	KubeSchedulerName string `json:"kubeSchedulerName,omitempty"`
	// PendingThresholdForPreemption is the threshold that is used to determine if a workload is awaiting for compute resources to be available.
	// If the workload is requesting GPUs and pending for longer than this threshold, kaiwo will start preempting workloads that have exceeded their duration deadline and are using GPUs of the same vendor as the pending workload.
	// +kubebuilder:default="5m"
	PendingThresholdForPreemption string `json:"pendingThresholdForPreemption,omitempty"`
}

// KaiwoConfig manages the Kaiwo operator's configuration which can be modified during runtime.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type KaiwoConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state for the Kaiwo operator configuration.
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
