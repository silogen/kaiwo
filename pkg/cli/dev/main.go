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

package main

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"

	k8sUtils "github.com/silogen/kaiwo/pkg/k8s"
	testutils "github.com/silogen/kaiwo/pkg/utils/test"
)

var (
	debugChainsawNamespace  string
	debugChainsawPrintLevel string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kaiwo-dev",
		Short: "Use Kaiwo developer features",
	}

	debugCmd := &cobra.Command{
		Use: "debug",
	}
	rootCmd.AddCommand(debugCmd)

	debugChainsawCmd := &cobra.Command{
		Use: "chainsaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := k8sUtils.GetKubernetesClients()
			if err != nil {
				return fmt.Errorf("failed to get k8s clients: %w", err)
			}

			if err := testutils.DebugTest(context.Background(), clients.Clientset, debugChainsawNamespace, debugChainsawPrintLevel); err != nil {
				return err
			}

			return nil
		},
	}
	debugCmd.AddCommand(debugChainsawCmd)
	debugChainsawCmd.Flags().StringVarP(&debugChainsawNamespace, "namespace", "n", "", "Test namespace to inspect")
	debugChainsawCmd.Flags().StringVarP(&debugChainsawPrintLevel, "print-level", "", "info", "The log level to print")

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
