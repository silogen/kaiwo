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
	"fmt"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/silogen/kaiwo/pkg/workloads/ray"
	"github.com/spf13/cobra"
)

var (
	useRayForServe bool
	model          string
)

func BuildServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve a deployment process", // TODO name
		RunE: func(cmd *cobra.Command, args []string) error {
			var deployment workloads.Workload

			deploymentFlags := workloads.DeploymentFlags{
				Model: model,
			}

			if useRayForServe {
				deployment = ray.Deployment{}
			} else {
				return fmt.Errorf("only ray serve is available at the moment")
			}

			return RunApply(deployment, deploymentFlags)
		},
	}

	// Common shared flags
	AddExecFlags(cmd)
	AddMetaFlags(cmd)
	AddSchedulingFlags(cmd)

	// Deployment-specific flags
	cmd.Flags().BoolVarP(&useRayForServe, "ray", "", false, "use ray for submitting the job")
	cmd.Flags().StringVarP(&model, "model", "m", "", "the model to use")

	return cmd
}
