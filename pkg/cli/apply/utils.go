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

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads2"
)

var (
	dryRun      bool
	printOutput bool
	preview     bool

	createNamespace  bool
	baseManifestPath string
	path             string

	name            string
	namespace       string
	image           = baseutils.Pointer("")
	imagePullSecret = baseutils.Pointer("")
	version         = baseutils.Pointer("")

	gpus           = baseutils.Pointer(0)
	replicas       = baseutils.Pointer(0)
	gpusPerReplica = baseutils.Pointer(0)

	dangerous = baseutils.Pointer(false)
	useRay    = baseutils.Pointer(false)
)

// AddCliFlags adds flags that are needed for the execution of apply functions
func AddCliFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&createNamespace, "create-namespace", "", false, "Create namespace if it does not exist")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Run a server-side dry run without creating any actual resources")
	cmd.Flags().BoolVarP(&printOutput, "print", "", false, "Print the generated workload manifest without submitting it")
	cmd.Flags().BoolVarP(&preview, "preview", "", false, "Preview all the resources that the Kaiwo operator would create from this manifest")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "print", "preview")

	cmd.Flags().StringVarP(&path, "path", "p", "", "Path to directory for workload code, entrypoint/serveconfig, env-file, etc. Either image or path is mandatory")
	cmd.Flags().StringVarP(&baseManifestPath, "base-manifest", "", "", "Optional path to a base manifest file")

	cmd.Flags().StringVarP(&name, "name", "", "", "Optional name for the workload")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", baseutils.DefaultNamespace, fmt.Sprintf("Namespace of the workload. Defaults to %s", baseutils.DefaultNamespace))
	cmd.Flags().StringVarP(image, "image", "i", baseutils.DefaultRayImage, fmt.Sprintf("Optional Image to use for the workload. Defaults to %s. Either image or workload path is mandatory", baseutils.DefaultRayImage))
	cmd.Flags().StringVarP(imagePullSecret, "imagepullsecret", "", "", "ImagePullSecret name for job/service if private registry")
	cmd.Flags().StringVarP(version, "version", "", "", "Optional version for job/service")

	cmd.Flags().IntVarP(gpus, "gpus", "g", 0, "Number of GPUs requested for the workload")
	cmd.Flags().IntVarP(replicas, "replicas", "", 0, "Number of replicas requested for the workload")
	cmd.Flags().IntVarP(gpusPerReplica, "gpus-per-replica", "", 0, "Number of GPUs requested per replica")
	cmd.Flags().BoolVarP(useRay, "ray", "", false, "Use ray for submitting the workload")
	cmd.Flags().BoolVarP(useRay, "dangerous", "", false, "Skip adding the default security context to containers")
}

func GetCLIFlags(cmd *cobra.Command) workloads2.CLIFlags {
	if !cmd.Flags().Changed("image") {
		image = nil
	}
	if !cmd.Flags().Changed("imagepullsecret") {
		imagePullSecret = nil
	}
	if !cmd.Flags().Changed("version") {
		version = nil
	}

	if !cmd.Flags().Changed("gpus") {
		gpus = nil
	}
	if !cmd.Flags().Changed("replicas") {
		replicas = nil
	}
	if !cmd.Flags().Changed("gpus-per-replica") {
		gpusPerReplica = nil
	}
	if !cmd.Flags().Changed("ray") {
		useRay = nil
	}
	if !cmd.Flags().Changed("dangerous") {
		dangerous = nil
	}

	return workloads2.CLIFlags{
		CreateNamespace:  createNamespace,
		DryRun:           dryRun,
		PrintOutput:      printOutput,
		Preview:          preview,
		Path:             path,
		BaseManifestPath: baseManifestPath,
		Name:             name,
		Namespace:        namespace,
		Image:            image,
		ImagePullSecret:  imagePullSecret,
		Version:          version,
		GPUs:             gpus,
		Replicas:         replicas,
		GPUsPerReplica:   gpusPerReplica,
		UseRay:           useRay,
		Dangerous:        dangerous,
	}
}
