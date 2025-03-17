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
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/silogen/kaiwo/pkg/k8s"

	"gopkg.in/yaml.v3"

	testutils "github.com/silogen/kaiwo/pkg/utils/test"

	"github.com/spf13/cobra"
)

var (
	e2eTestSourceDir string
	e2eTestOutputDir string

	debugChainsawNamespace  string
	debugChainsawPrintLevel string
)

func BuildTestsCmd() *cobra.Command {
	testCmd := &cobra.Command{
		Use:   "tests",
		Short: "Interact with Kaiwo tests",
	}

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate assets",
	}
	testCmd.AddCommand(generateCmd)

	generateE2eTestsCmd := &cobra.Command{
		Use:   "e2e-tests",
		Short: "Generate end-to-end tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateE2eTests(e2eTestSourceDir, e2eTestOutputDir)
		},
	}

	generateCmd.AddCommand(generateE2eTestsCmd)
	generateE2eTestsCmd.Flags().StringVarP(&e2eTestSourceDir, "input", "", "", "Directory to load feature files from")
	generateE2eTestsCmd.Flags().StringVarP(&e2eTestOutputDir, "output", "", "", "Directory to save generated tests in")

	debugCmd := &cobra.Command{
		Use: "debug",
	}
	testCmd.AddCommand(debugCmd)

	debugChainsawCmd := &cobra.Command{
		Use: "chainsaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := k8s.GetKubernetesClients()
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

	return testCmd
}

func runGenerateE2eTests(inputDir string, outputDir string) error {
	var features []testutils.GherkinFeature
	fmt.Println("Generating e2e tests...")
	fmt.Printf("Input dir: %s\n", inputDir)

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		fmt.Println("Walking directory:", path)
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ".yaml" && filepath.Base(path) != "" {
			fmt.Println("Loading feature file:", path)
			if strings.HasSuffix(path, ".feature.yaml") {
				var feature testutils.GherkinFeature
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("could not read file %s: %w", path, err)
				}

				err = yaml.Unmarshal(data, &feature)
				if err != nil {
					return fmt.Errorf("could not unmarshal file %s: %w", path, err)
				}
				fmt.Printf("Found file: %s\n", path)
				features = append(features, feature)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not walk dir: %w", err)
	}

	for _, feature := range features {
		fmt.Printf("Building tests")
		if err := feature.CreateChainsawTests(outputDir, inputDir); err != nil {
			return fmt.Errorf("could not build chainsaw tests: %w", err)
		}
	}
	return nil
}
