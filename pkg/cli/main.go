// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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

package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/silogen/kaiwo/pkg/workloads/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	cli "github.com/silogen/kaiwo/pkg/cli/apply"
	"github.com/silogen/kaiwo/pkg/workloads"
)

// Build-time variables (set with -ldflags). Do not touch
var (
	version = "unknown" // Build version
	commit  = "unknown" // Git commit hash
	date    = "unknown" // Build timestamp
)

var (
	namespace string
	verbosity int
	quiet     int
)

func RunCli() {
	rootCmd := &cobra.Command{
		Use:          "kaiwo",
		SilenceUsage: true,
		Short:        "Kubernetes-native AI Workload Orchestrator",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

			if verbosity != 0 && quiet != 0 {
				return fmt.Errorf("cannot set both verbose and quiet at the same time")
			}

			switch verbosity {
			case 0:
			case 1:
				logrus.SetLevel(logrus.DebugLevel)
			default:
				logrus.SetLevel(logrus.TraceLevel)
			}

			switch quiet {
			case 0:
			case 1:
				logrus.SetLevel(logrus.WarnLevel)
			case 2:
				logrus.SetLevel(logrus.ErrorLevel)
			default:
				logrus.SetLevel(logrus.FatalLevel)
			}

			return nil
		},
	}
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Set verbosity level (use -v, -vv)")
	rootCmd.PersistentFlags().CountVarP(&quiet, "quiet", "q", "Set verbosity level (use -q, -qq, -qqq)")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show the version of kaiwo",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kaiwo version: %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("build date: %s\n", date)
		},
	})

	rootCmd.AddCommand(cli.BuildSubmitCmd())
	rootCmd.AddCommand(cli.BuildServeCmd())

	rootCmd.AddCommand(&cobra.Command{
		Use:   "attach",
		Short: "Attach to a workload",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Attach command placeholder")
		},
	})
	rootCmd.AddCommand(
		BuildLogCmd(),
		BuildListCmd(),
		BuildMonitorCmd("monitor", utils.DefaultMonitorCommand),
		BuildExecCommand(),
	)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "port-forward",
		Short: "Port-forward a workload",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Port-forward command placeholder")
		},
	})

	deleteCmd := &cobra.Command{
		Use:   "delete -n [namespace] [workload-type]/[workload-name]",
		Short: "Delete a workload. Workload type must be one of [job, deployment, rayjob, rayservice]",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			split := strings.Split(args[0], "/")

			if len(split) != 2 {
				logrus.Fatalf("Invalid input format. Expected format: [workload-type]/[workload-name].")
				return
			}

			workloadType := split[0]
			name := split[1]

			err := workloads.Cleanup(context.TODO(), workloadType, name, namespace, true)

			if err != nil {
				logrus.Errorf("Failed to delete workload: %v", err)
			}

		},
	}

	deleteCmd.Flags().StringVarP(&namespace, "namespace", "n", "kaiwo", "Kubenetes namespace to use. Defaults to `kaiwo`")

	rootCmd.AddCommand(deleteCmd)

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
