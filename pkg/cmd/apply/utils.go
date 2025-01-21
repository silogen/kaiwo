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

package cli

import (
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/spf13/cobra"
)

const defaultGpuNodeLabelKey = "beta.amd.com/gpu.family.AI"

// Exec flags
var (
	dryRun           bool
	createNamespace  bool
	template         string
	path             string
	gpuNodeLabelKey  string
	customConfigPath string
)

// AddExecFlags adds flags that are needed for the execution of apply functions
func AddExecFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&createNamespace, "create-namespace", "", false, "Create namespace if it does not exist")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Print the generated workload manifest without submitting it")
	cmd.Flags().StringVarP(&template, "template", "t", "", "Path to a custom template to use for the workload. If not provided, a default template will be used")
	cmd.Flags().StringVarP(&path, "path", "p", "", "absolute or relative path to workload code and entrypoint/serveconfig directory")
	cmd.Flags().StringVarP(&gpuNodeLabelKey, "gpu-node-label-key", "", defaultGpuNodeLabelKey, "The node label key used to specify the resource flavor GPU count")
	cmd.Flags().StringVarP(&customConfigPath, "custom-config", "c", "", "Path to a custom YAML configuration file whose contents are made available in the template")
}

func GetExecFlags() workloads.ExecFlags {
	return workloads.ExecFlags{
		CreateNamespace:               createNamespace,
		DryRun:                        dryRun,
		Template:                      template,
		Path:                          path,
		ResourceFlavorGpuNodeLabelKey: gpuNodeLabelKey,
		CustomConfigPath:              customConfigPath,
	}
}

// Kubernetes meta flags

const defaultNamespace = "kaiwo"
const defaultImage = "ghcr.io/silogen/rocm-ray:v0.4"

var (
	name      string
	namespace string
	image     string
	imagePullSecret string
)

// AddMetaFlags adds flags that are needed for basic Kubernetes metadata
func AddMetaFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&name, "name", "", "", "Name of the workload")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", defaultNamespace, "Namespace of the workload")
	cmd.Flags().StringVarP(&image, "image", "i", defaultImage, "The image to use for the workload")
	cmd.Flags().StringVarP(&imagePullSecret, "imagepullsecret", "", "", "ImagePullSecret name for job/deployment if private registry")
}

func GetMetaFlags() workloads.MetaFlags {
	return workloads.MetaFlags{
		Name:      name,
		Namespace: namespace,
		Image:     image,
		ImagePullSecret: imagePullSecret,
	}
}

// Scheduling flags

var (
	gpus int
)

// AddSchedulingFlags adds flags related to (Kueue) scheduling
func AddSchedulingFlags(cmd *cobra.Command) {
	cmd.Flags().IntVarP(&gpus, "gpus", "g", 0, "Number of GPUs requested for the workload")
}

// GetSchedulingFlags initializes the scheduling flags with the number of GPUs requested
func GetSchedulingFlags() workloads.SchedulingFlags {
	return workloads.SchedulingFlags{
		TotalRequestedGPUs: gpus,
	}
}
