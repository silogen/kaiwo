package cli

import (
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads"
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads/ray"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	useRayForServe bool
	serveImage     string
	model          string
)

func BuildServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve a deployment process", // TODO name
		Run: func(cmd *cobra.Command, args []string) {
			sharedFlags := GetSharedFlags()

			var deployment workloads.Workload

			deploymentFlags := workloads.DeploymentFlags{
				Model: model,
				Image: serveImage,
			}

			if useRayForServe {
				deployment = ray.Deployment{
					Deployment: deploymentFlags,
					Shared:     sharedFlags,
				}
			} else {
				// Raise error
				logrus.Error("Only ray serve is available at the moment")
				return
			}

			RunApply(deployment)
		},
	}

	// Common shared flags
	AddExecFlags(cmd)
	AddSharedFlags(cmd)

	// Job-specific flags
	cmd.Flags().BoolVarP(
		&useRayForServe,
		"ray",
		"",
		false,
		"use ray for submitting the job",
	)

	cmd.Flags().StringVarP(
		&model,
		"model",
		"m",
		"",
		"The model to use",
	)

	cmd.Flags().StringVarP(
		&serveImage,
		"image",
		"i",
		"",
		"The image to use for the job",
	)

	return cmd
}
