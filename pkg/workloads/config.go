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

package workloads

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

const (
	KaiwoconfigFilename               = "kaiwoconfig"
	EnvFilename                       = "env"
	KaiwoUsernameLabel                = "kaiwo-cli/username"
	KaiwoDefaultStorageClassNameLabel = "kaiwo-cli/default-storage-class-name"
	KaiwoDefaultStorageQuantityLabel  = "kaiwo-cli/default-storage-quantity"
	CustomTemplateValuesFilename      = "custom-template-values.yaml"
)

// WorkloadTemplateConfig is the config context that is passed to the workload templates
type WorkloadTemplateConfig struct {
	// Workload-specific config
	Workload any

	// Workload type specific config
	WorkloadMeta any

	// Custom configuration from the user
	Custom any

	// Scheduling configuration for Kubernetes
	Scheduling SchedulingFlags

	// Meta configuration for Kubernetes
	Meta MetaFlags
}

// SchedulingFlags for scheduling options
type SchedulingFlags struct {
	// GPUsAvailablePerNode refers to the Kueue resource flavor's GPU count
	GPUsAvailablePerNode int

	RequestedGPUsPerReplica int

	// TotalRequestedGPUs refers to the total number of GPUs requested for the workload
	TotalRequestedGPUs int

	RequestedReplicas int

	// CalculatedGPUsPerReplica refers to the number of GPUs per replica, calculated from the available GPUs per node
	CalculatedGPUsPerReplica int

	// CalculatedNumReplicas refers to the number of replicas, calculated from the available GPUs per node
	CalculatedNumReplicas int

	Storage *StorageSchedulingFlags
}

type StorageSchedulingFlags struct {
	Quantity         string
	StorageClassName string
}

// MetaFlags contain flags that are shared by all workloads
type MetaFlags struct {
	// The name of the resource
	Name string

	// The namespace of the resource
	Namespace string

	// The base image to use
	Image string

	ImagePullSecret string

	Version string

	User string

	// Environment variables
	EnvVars []corev1.EnvVar

	// Secret volumes
	SecretVolumes []v1alpha1.SecretVolume

	// Whether there is associated config map data
	HasConfigMap bool
}

// ExecFlags contain flags that are shared by all workloads during the resource-creation process,
// but are not passed into the template context
type ExecFlags struct {
	// Run without modifying resources
	DryRun bool

	// Create namespace if it doesn't already exist
	CreateNamespace bool

	// Custom template, if any
	Template string

	// Path to workload folder
	Path string

	// OverlayPath contains specific files that override files in Path
	OverlayPath string

	// The key used to store the GPU count per node in the resource flavor
	ResourceFlavorGpuNodeLabelKey string

	// WorkloadFiles list the files that are considered to be part of the workload after merging Path and OverlayPath
	// The map is from the workload path (how the workload would see it) to the true relative path (how the CLI client sees it)
	WorkloadFiles map[string]string
}
