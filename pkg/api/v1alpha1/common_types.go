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
)

// Common labels used across resources.
const (
	UsernameLabel = "kaiwo-cli/username"
	QueueLabel    = "kueue.x-k8s.io/queue-name"
)

// CommonMetaSpec defines reusable metadata fields for workloads.
type CommonMetaSpec struct {
	// Username specifies the owner or creator of the workload.
	Username string `json:"username,omitempty"`

	// Name is the name of the workload.
	Name string `json:"name,omitempty"`

	// Namespace defines the namespace in which the workload is deployed.
	Namespace string `json:"namespace,omitempty"`

	// Labels is a map of key-value pairs used for organizing workloads.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations provides additional metadata for the workload.
	Annotations map[string]string `json:"annotations,omitempty"`

	// Gpus specifies the total number of GPUs allocated to the workload.
	Gpus int `json:"gpus,omitempty"`

	// Version is an optional field specifying the version of the workload.
	Version string `json:"version,omitempty"`

	// Replicas specifies the number of replicas for the workload.
	// If greater than one, the workload must use Ray.
	Replicas int `json:"replicas,omitempty"`

	// GpusPerReplica specifies the number of GPUs allocated per replica.
	GpusPerReplica int `json:"gpus-per-replica,omitempty"`

	// Image defines the container image used for the workload.
	Image string `json:"image,omitempty"`

	// ImagePullSecrets contains the list of secrets used to pull the container image.
	ImagePullSecrets []string `json:"image-pull-secrets,omitempty"`

	// Ray determines whether the operator should use RayCluster for workload execution.
	// Default is false.
	Ray bool `json:"ray"`

	// ConfigMap optionally mounts a ConfigMap into the workload.
	ConfigMap *corev1.ConfigMap `json:"configmap,omitempty"`
}

// CommonContainer defines the container specifications within a workload.
type CommonContainer struct {
	// Name is the name of the container.
	Name string `json:"name"`

	// Image specifies the container image to be used.
	Image string `json:"image"`

	// ImagePullPolicy defines when Kubernetes should pull the container image.
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Command specifies the entrypoint commands executed within the container.
	Command []string `json:"command,omitempty"`

	// Env defines environment variables for the container.
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources specifies CPU, memory, and GPU requirements.
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// VolumeMounts defines the volumes mounted to the container.
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
}

// CommonVolume defines simplified volume configurations.
type CommonVolume struct {
	// Name is the name of the volume.
	Name string `json:"name"`

	// Secret defines a volume sourced from a Kubernetes Secret.
	Secret *corev1.SecretVolumeSource `json:"secret,omitempty"`

	// ConfigMap defines a volume sourced from a Kubernetes ConfigMap.
	ConfigMap *corev1.ConfigMapVolumeSource `json:"configMap,omitempty"`

	// EmptyDir defines an ephemeral volume backed by node storage.
	EmptyDir *corev1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`
}

// CommonPodSpec captures essential fields from the Pod specification.
type CommonPodSpec struct {
	// RestartPolicy defines the restart behavior of the pod.
	RestartPolicy corev1.RestartPolicy `json:"restartPolicy,omitempty"`

	// ImagePullSecrets contains secrets used to pull container images.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Containers lists the primary containers in the pod.
	Containers []CommonContainer `json:"containers"`

	// Volumes lists the volumes mounted to the pod.
	Volumes []CommonVolume `json:"volumes,omitempty"`
}

// RayServiceSpec defines fields specific to a RayService workload.
type RayServiceSpec struct {
	// ServeConfig specifies the Ray Serve configuration.
	ServeConfig string `json:"serveConfig,omitempty"`

	// RayClusterConfig contains the configuration for the underlying Ray cluster.
	RayClusterConfig RayClusterSpec `json:"rayClusterConfig"`
}

// RayJobSpec defines fields specific to a RayJob workload.
type RayJobSpec struct {
	// EntryPoint specifies the command or script that starts the Ray job.
	EntryPoint string `json:"entryPoint,omitempty"`

	// RayClusterSpec defines the Ray cluster that executes the job.
	RayClusterSpec RayClusterSpec `json:"rayClusterSpec"`
}

// RayClusterSpec defines the configuration of a Ray cluster.
type RayClusterSpec struct {
	// HeadGroupSpec specifies the configuration for the Ray cluster head node.
	HeadGroupSpec HeadGroupSpec `json:"headGroupSpec"`

	// WorkerGroupSpecs defines the configurations for Ray worker nodes.
	WorkerGroupSpecs []WorkerGroupSpec `json:"workerGroupSpecs,omitempty"`
}

// HeadGroupSpec defines the configuration for the Ray head node.
type HeadGroupSpec struct {
	// RayStartParams specifies startup parameters for the Ray head node.
	RayStartParams map[string]string `json:"rayStartParams,omitempty"`

	// Template defines the pod specifications for the Ray head node.
	Template CommonPodSpec `json:"template"`
}

// WorkerGroupSpec defines the configuration for a Ray worker node group.
type WorkerGroupSpec struct {
	// GroupName specifies the name of the worker group.
	GroupName string `json:"groupName"`

	// Replicas defines the desired number of worker nodes in the group.
	Replicas *int32 `json:"replicas,omitempty"`

	// MinReplicas defines the minimum number of worker nodes.
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas defines the maximum number of worker nodes.
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// RayStartParams specifies startup parameters for Ray worker nodes.
	RayStartParams map[string]string `json:"rayStartParams,omitempty"`

	// Template defines the pod specifications for Ray worker nodes.
	Template CommonPodSpec `json:"template"`
}

// DeploymentSpec defines fields specific to Kubernetes Deployment workloads.
type DeploymentSpec struct {
	// Replicas defines the number of pod replicas for the deployment.
	Replicas int32 `json:"replicas"`

	// Template defines the pod specifications for the deployment.
	Template CommonPodSpec `json:"template"`
}

// JobSpec defines fields specific to Kubernetes Job workloads.
type JobSpec struct {
	// TTLSecondsAfterFinished specifies how long a job should persist after completion.
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// Template defines the pod specifications for the job.
	Template CommonPodSpec `json:"template"`
}
