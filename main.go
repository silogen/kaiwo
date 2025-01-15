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
	"fmt"
	"os"
	"slices"

	"github.com/silogen/kaiwo/pkg/utils"

	"github.com/silogen/kaiwo/pkg/submit"
	"github.com/silogen/kaiwo/pkg/templates"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var kaiwoBanner = `
 _  __     _
| |/ /__ _(_)_      _____
| ' // _' | \ \ /\ / / _ \
| . \ (_| | |\ V  V / (_) |
|_|\_\__,_|_| \_/\_/ \___/
Kubernetes-native AI Workload Orchestrator


`

// Build-time variables (set with -ldflags). Do not touch
var (
	version = "unknown" // Build version
	commit  = "unknown" // Git commit hash
	date    = "unknown" // Build timestamp
)

var (
	path                string
	image               string
	queue               string
	name                string
	namespace           string
	type_               string
	template            string
	gpus                int
	dryRun              bool
	createNamespace     bool
	ttlMinAfterFinished int
)

const defaultImage = "ghcr.io/silogen/rocm-ray:v0.4"
const defaultQueue = "kaiwo"
const defaultTtlMinAfterFinished = 2880

func main() {

	fmt.Fprint(os.Stderr, kaiwoBanner)

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	rootCmd := &cobra.Command{
		Use:   "kaiwo",
		Short: "Kubernetes-native AI Workload Orchestrator",
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show the version of kaiwo",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kaiwo version: %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("build date: %s\n", date)
		},
	})

	submitCmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a workload",
		Run: func(cmd *cobra.Command, args []string) {

			workloadArgs := utils.WorkloadArgs{
				Path:                path,
				Image:               image,
				Queue:               queue,
				Name:                name,
				Namespace:           namespace,
				TemplatePath:        template,
				Type:                type_,
				GPUs:                gpus,
				DryRun:              dryRun,
				CreateNamespace:     createNamespace,
				TtlMinAfterFinished: ttlMinAfterFinished,
			}
			if err := submit.Submit(workloadArgs); err != nil {
				logrus.Fatalf("Failed to submit workload: %v", err)
			}
		},

		PreRunE: func(cmd *cobra.Command, args []string) error {
			// TODO Move other validation here as well
			if !slices.Contains(templates.WorkloadTypes, type_) {
				return fmt.Errorf("invalid workload type %s. Must be one of %v", type_, templates.WorkloadTypes)
			}
			return nil
		},
	}

	submitCmd.Flags().StringVarP(&path, "path", "p", "", "absolute or relative path to workload code and entrypoint/serveconfig directory")
	submitCmd.Flags().StringVarP(&image, "image", "i", defaultImage, "Container image to use. Defaults to ghcr.io/silogen/rocm-ray:vx.x")
	submitCmd.Flags().StringVarP(&queue, "queue", "q", defaultQueue, "ClusterQueue to use. Defaults to queue")
	submitCmd.Flags().StringVarP(&name, "name", "", "", "Kubernetes name to use for the workflow")
	submitCmd.Flags().StringVarP(&namespace, "namespace", "n", "kaiwo", "Kubernetes namespace to use. Defaults to `kaiwo`")
	submitCmd.Flags().BoolVarP(&createNamespace, "create-namespace", "", false, "Create namespace if it does not exist")
	submitCmd.Flags().IntVarP(&ttlMinAfterFinished, "ttl-minutes-after-finished", "", defaultTtlMinAfterFinished, "Cleanup finished Jobs after minutes. Defaults to 48h (2880 min)")
	submitCmd.Flags().StringVarP(&template, "template", "", "", "Path to a custom template to use for the workload. If not provided, a default template will be used")
	submitCmd.Flags().StringVarP(&type_, "type", "t", "job", "Workload type, one of [rayjob, rayservice, job] (default 'job')")
	submitCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Print the generated workload manifest without submitting it")
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
	rootCmd.AddCommand(&cobra.Command{
		Use:   "delete",
		Short: "Delete a workload",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Delete command placeholder")
		},
	})

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
