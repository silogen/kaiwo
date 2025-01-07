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

package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/silogen/ai-workload-orchestrator/pkg/submit"
)

var (
	path    string
	image   string
	job     bool
	service bool
	gpus    int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aiwo",
		Short: "AI Workflow Orchestrator",
	}

	submitCmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a workload",
		Run: func(cmd *cobra.Command, args []string) {
			if err := submit.Submit(path, image, job, service, gpus); err != nil {
				logrus.Fatalf("Failed to submit workload: %v", err)
			}
		},
	}
	submitCmd.Flags().StringVarP(&path, "path", "p", "", "Path to workload code and entrypoint/serveconfig. Format: workloads/workload_type/modality/method_type/workload_code_directory")
	submitCmd.Flags().StringVarP(&image, "image", "i", "", "Container image to use. Defaults to ghcr.io/silogen/rocm-ray:vx.x")
	submitCmd.Flags().BoolVarP(&job, "job", "j", false, "Submit as RayJob")
	submitCmd.Flags().BoolVarP(&service, "service", "s", false, "Submit as RayService")
	submitCmd.Flags().IntVarP(&gpus, "gpus", "g", 1, "Number of GPUs required")

	rootCmd.AddCommand(submitCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "attach",
		Short: "Attach to a workload",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Attach command placeholder")
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "logs",
		Short: "View logs of a workload",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Logs command placeholder")
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "port-forward",
		Short: "Port-forward a workload",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Port-forward command placeholder")
		},
	})

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
