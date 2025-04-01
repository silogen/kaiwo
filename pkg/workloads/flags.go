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

const (
	KaiwoconfigFilename               = "kaiwoconfig"
	EnvFilename                       = "env"
	KaiwoUsernameLabel                = "kaiwo-cli/username"
	KaiwoDefaultStorageClassNameLabel = "kaiwo-cli/default-storage-class-name"
	KaiwoDefaultStorageQuantityLabel  = "kaiwo-cli/default-storage-quantity"
	CustomTemplateValuesFilename      = "custom-template-values.yaml"
)

type CLIFlags struct {
	Name            string
	Namespace       string
	Image           *string
	ImagePullSecret *string
	Version         *string
	User            *string
	GPUs            *int
	GPUsPerReplica  *int
	Replicas        *int

	UseRay    *bool
	Dangerous *bool

	// Path to workload folder
	Path string

	BaseManifestPath string

	Queue *string

	// Run without modifying resources
	PrintOutput bool
	Preview     bool

	// Create namespace if it doesn't already exist
	CreateNamespace bool
}
