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
	"k8s.io/apimachinery/pkg/api/resource"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoConfigSpec defines the desired configuration for the Kaiwo operator's configuration.
// There should typically be only one KaiwoConfig resource in the cluster.
type KaiwoConfigSpec struct {
	// ENFORCE_KAIWO_ON_GPU_WORKLOADS false
	// EnforceKaiwoOnGpuWorkloads bool `json:"enforceKaiwoOnGpuWorkloads"`

	// Kueue defines the Kueue-specific settings
	// +kubebuilder:default={}
	Kueue KaiwoKueueConfig `json:"kueue,omitempty"`

	// Ray defines the Ray-specific settings
	// +kubebuilder:default={}
	Ray KaiwoRayConfig `json:"ray,omitempty"`

	// Data defines the data-specific settings
	// +kubebuilder:default={}
	Data KaiwoDataConfig `json:"data,omitempty"`

	// Nodes defines the node configuration settings
	// +kubebuilder:default={}
	Nodes KaiwoNodeConfig `json:"nodes,omitempty"`

	// Scheduling contains the configuration Kaiwo uses for workload scheduling
	// +kubebuilder:default={}
	Scheduling KaiwoSchedulingConfig `json:"scheduling,omitempty"`

	// ResourceMonitoring defines the resource-monitoring specific settings
	// +kubebuilder:default={}
	ResourceMonitoring KaiwoResourceMonitoringConfig `json:"resourceMonitoring,omitempty"`

	// DefaultKaiwoQueueConfigName is the name of the singleton Kaiwo Queue Config object that is used
	// +kubebuilder:default="kaiwo"
	DefaultKaiwoQueueConfigName string `json:"defaultKaiwoQueueConfigName,omitempty"`
}

// KaiwoRayConfig contains the Ray-specific configuration that Kaiwo uses.
type KaiwoRayConfig struct {
	// DefaultRayImage is the image that is used for Ray workloads if no image is provided in the workload CRD
	// +kubebuilder:default="ghcr.io/silogen/rocm-ray:v0.9"
	DefaultRayImage string `json:"defaultRayImage,omitempty"`

	// HeadPodMemory is the amount of memory that is requested for the Ray head pod
	HeadPodMemory resource.Quantity `json:"headPodMemory,omitempty"`
}

// KaiwoKueueConfig contains the Kueue-specific configuration that Kaiwo uses.
type KaiwoKueueConfig struct {
	// DefaultClusterQueueName is the default cluster queue name that is used, if none is provided in the workload CRD.
	// +kubebuilder:default="kaiwo"
	DefaultClusterQueueName string `json:"defaultClusterQueueName,omitempty"`
}

type KaiwoDataConfig struct {
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
// * Setting the Prometheus endpoint (`RESOURCE_MONITORING_PROMETHEUS_ENDPOINT=...`)
// * Setting the polling interval (`RESOURCE_MONITORING_POLLING_INTERVAL=10m`)
type KaiwoResourceMonitoringConfig struct {
	// AveragingTime is the time to use to average the metrics over
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +kubebuilder:default="20m"
	AveragingTime string `json:"averagingTime,omitempty"`

	// MinAliveTime is the time that a pod must have been alive for in order to qualify for inspection
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +kubebuilder:default="20m"
	MinAliveTime string `json:"minAliveTime,omitempty"`

	// LowUtilizationThreshold is the threshold which, if the metric goes under, the workload is considered underutilized. The threshold is interpreted as the percentage utilization versus the requested capacity.
	// +kubebuilder:default=20
	// +kubebuilder:validation:Minimum=0
	LowUtilizationThreshold float64 `json:"lowUtilizationThreshold,omitempty"`

	// TargetNamespaces is a list of namespaces to apply the monitoring to. If not supplied or empty, all namespaces apart from kube-system will be inspected. However, only pods associated with KaiwoJobs or KaiwoServices are impacted.
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// Profile chooses the target resource to monitor.
	// +kubebuilder:validation:Enum="gpu";"cpu"
	// +kubebuilder:default="gpu"
	Profile string `json:"profile,omitempty"`
}

// KaiwoSchedulingConfig contains the configuration Kaiwo uses for workload scheduling
type KaiwoSchedulingConfig struct {
	// KubeSchedulerName defines the default scheduler name that is used to schedule the workload
	// +kubebuilder:default="default-scheduler"
	KubeSchedulerName string `json:"kubeSchedulerName,omitempty"`
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
