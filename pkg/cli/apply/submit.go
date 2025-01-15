package cli

import (
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads"
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads/kueue"
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads/ray"
	"github.com/spf13/cobra"
)

var (
	useRayForJob        bool
	jobImage            string
	queue               string
	ttlMinAfterFinished int
)

const defaultQueue = "kaiwo"
const defaultTtlMinAfterFinished = 2880

func BuildSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a job",
		Run: func(cmd *cobra.Command, args []string) {
			sharedFlags := GetSharedFlags()

			var job workloads.Workload

			jobFlags := workloads.JobFlags{
				Queue:               queue,
				TtlMinAfterFinished: ttlMinAfterFinished,
				Image:               jobImage,
			}

			if useRayForJob {
				job = ray.Job{
					Job:    jobFlags,
					Shared: sharedFlags,
				}
			} else {
				job = kueue.Job{
					Job:    jobFlags,
					Shared: sharedFlags,
				}
			}
			RunApply(job)
		},
	}

	// Common shared flags
	AddExecFlags(cmd)
	AddSharedFlags(cmd)

	// Job-specific flags
	cmd.Flags().BoolVarP(
		&useRayForJob,
		"ray",
		"",
		false,
		"use ray for submitting the job",
	)

	cmd.Flags().StringVarP(
		&queue,
		"queue",
		"q",
		defaultQueue,
		"The queue to submit job to",
	)

	cmd.Flags().StringVarP(
		&jobImage,
		"image",
		"i",
		"",
		"The image to use for the job",
	)

	cmd.Flags().IntVarP(
		&ttlMinAfterFinished,
		"ttl-minutes-after-finished",
		"",
		defaultTtlMinAfterFinished,
		"Cleanup finished Jobs after minutes. Defaults to 48h (2880 min)",
	)

	return cmd
}
