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
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/silogen/kaiwo/pkg/workloads/deployments"
	"github.com/silogen/kaiwo/pkg/workloads/ray"
)

var (
	useRayForServe bool
	model          string
)

func BuildServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve a deployment process",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Parent().PersistentPreRunE(cmd, args); err != nil {
				return err
			}
			return PreRunLoadConfig(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var deployment workloads.Workload

			deploymentFlags := workloads.DeploymentFlags{
				Model: model,
			}

			if useRayForServe {
				deployment = ray.Deployment{}
				logrus.Debugln("Using RayService for deployment")
			} else {
				deployment = deployments.Deployment{}
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
	cmd.Flags().StringVarP(&model, "model", "m", "", "the model to use (Not implemented yet)")

	return cmd
}
