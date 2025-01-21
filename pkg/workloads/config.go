/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package workloads

import (
	"github.com/silogen/kaiwo/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
)

const KaiwoconfigFilename = "kaiwoconfig"
const EnvFilename = "env"

// WorkloadTemplateConfig is the config context that is passed to the workflow templates
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

	// TotalRequestedGPUs refers to the total number of GPUs requested for the workload
	TotalRequestedGPUs int

	// CalculatedGPUsPerReplica refers to the number of GPUs per replica, calculated from the available GPUs per node
	CalculatedGPUsPerReplica int

	// CalculatedNumReplicas refers to the number of replicas, calculated from the available GPUs per node
	CalculatedNumReplicas int
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

	// Environment variables
	EnvVars []corev1.EnvVar

	// Secret volumes
	SecretVolumes []k8s.SecretVolume

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

	// The key used to store the GPU count per node in the resource flavor
	ResourceFlavorGpuNodeLabelKey string

	// Path to custom config file
	CustomConfigPath string
}
