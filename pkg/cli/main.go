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

package cmd

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	cli "github.com/silogen/kaiwo/pkg/cli/apply"
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
	// TODO re-enable
	// rootCmd.AddCommand(cli.BuildServeCmd())

	// TODO re-enable
	//rootCmd.AddCommand(
	//BuildLogCmd(),
	//BuildListCmd(),
	//BuildMonitorCmd("monitor", utils.DefaultMonitorCommand),
	//BuildExecCommand(),
	//)

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
