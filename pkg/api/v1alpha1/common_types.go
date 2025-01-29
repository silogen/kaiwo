// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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

// Common labels used across resources
const (
	UsernameLabel = "kaiwo-cli/username"
	QueueLabel    = "kueue.x-k8s.io/queue-name"
)

// CommonMetaSpec defines reusable metadata fields.
type CommonMetaSpec struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type CommonContainer struct {
	Name            string                      `json:"name"`
	Image           string                      `json:"image"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Command         []string                    `json:"command,omitempty"`
	Env             []corev1.EnvVar             `json:"env,omitempty"`
	Resources       corev1.ResourceRequirements `json:"resources,omitempty"`
	VolumeMounts    []corev1.VolumeMount        `json:"volumeMounts,omitempty"`
}

// CommonVolume defines simplified volume configuration.
type CommonVolume struct {
	Name      string                        `json:"name"`
	Secret    *corev1.SecretVolumeSource    `json:"secret,omitempty"`
	ConfigMap *corev1.ConfigMapVolumeSource `json:"configMap,omitempty"`
	EmptyDir  *corev1.EmptyDirVolumeSource  `json:"emptyDir,omitempty"`
}

// CommonPodSpec captures essential fields from the pod spec.
type CommonPodSpec struct {
	RestartPolicy    corev1.RestartPolicy          `json:"restartPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Containers       []CommonContainer             `json:"containers"`
	Volumes          []CommonVolume                `json:"volumes,omitempty"`
}

// // RayServiceSpec defines fields specific to RayService.
type RayServiceSpec struct {
	ServeConfig string `json:"serveConfig,omitempty"`

	// RayClusterConfig contains configuration specific to RayService, such as autoscaling
	RayClusterConfig RayClusterSpec `json:"rayClusterConfig"`
}

type RayJobSpec struct {
	EntryPoint string `json:"entryPoint,omitempty"`

	RayClusterSpec RayClusterSpec `json:"rayClusterSpec"`
}

type RayClusterSpec struct {
	// HeadGroupSpec is the specification for the head pod in the cluster
	HeadGroupSpec HeadGroupSpec `json:"headGroupSpec"`

	// WorkerGroupSpecs defines configurations for worker pods in the cluster
	WorkerGroupSpecs []WorkerGroupSpec `json:"workerGroupSpecs,omitempty"`
}

type HeadGroupSpec struct {
	RayStartParams map[string]string `json:"rayStartParams,omitempty"`
	Template       CommonPodSpec     `json:"template"`
}

type WorkerGroupSpec struct {
	GroupName      string            `json:"groupName"`
	Replicas       *int32            `json:"replicas,omitempty"`
	MinReplicas    *int32            `json:"minReplicas,omitempty"`
	MaxReplicas    *int32            `json:"maxReplicas,omitempty"`
	RayStartParams map[string]string `json:"rayStartParams,omitempty"`
	Template       CommonPodSpec     `json:"template"`
}

// DeploymentSpec defines fields specific to Kubernetes Deployment.
type DeploymentSpec struct {
	Replicas int32         `json:"replicas"`
	Template CommonPodSpec `json:"template"`
}

// JobSpec defines fields specific to Kubernetes Job.
type JobSpec struct {
	TTLSecondsAfterFinished *int32        `json:"ttlSecondsAfterFinished,omitempty"`
	Template                CommonPodSpec `json:"template"`
}
