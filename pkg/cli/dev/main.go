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
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sUtils "github.com/silogen/kaiwo/pkg/k8s"
	testutils "github.com/silogen/kaiwo/pkg/utils/test"
)

var (
	e2eTestSourceDir string
	e2eTestOutputDir string

	debugChainsawNamespace  string
	debugChainsawPrintLevel string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kaiwo-dev",
		Short: "Use Kaiwo developer features",
	}

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate assets",
	}
	rootCmd.AddCommand(generateCmd)

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

	helpersCmd := &cobra.Command{
		Use: "helpers",
	}
	rootCmd.AddCommand(helpersCmd)

	getFileCmd := &cobra.Command{
		Use: "get-file",
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := k8sUtils.GetKubernetesClients()
			if err != nil {
				return fmt.Errorf("failed to get k8s clients: %w", err)
			}

			namespace := cmd.Flag("namespace").Value.String()
			selector := cmd.Flag("selector").Value.String()
			path := cmd.Flag("path").Value.String()

			if namespace == "" || selector == "" || path == "" {
				return fmt.Errorf("namespace, selector, and path flags are required")
			}

			pods, err := clients.Clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return fmt.Errorf("failed to list pods: %w", err)
			}
			if len(pods.Items) == 0 {
				return fmt.Errorf("no pods found matching selector: %s", selector)
			}

			podName := pods.Items[0].Name

			execCmd := exec.Command("kubectl", "exec", "-n", namespace, podName, "--", "cat", path)
			var outBuf, errBuf bytes.Buffer
			execCmd.Stdout = &outBuf
			execCmd.Stderr = &errBuf

			if err := execCmd.Run(); err != nil {
				return fmt.Errorf("failed to exec into pod: %v, stderr: %s", err, errBuf.String())
			}

			content := outBuf.Bytes()

			// Check for binary content: non-UTF-8 or control characters
			if isBinary(content) {
				return fmt.Errorf("file appears to be binary, refusing to print")
			}

			// Print file content
			fmt.Println(content)

			return nil
		},
	}
	getFileCmd.Flags().String("namespace", "", "The namespace")
	getFileCmd.Flags().String("selector", "", "The label selector to find the pod")
	getFileCmd.Flags().String("path", "", "The path to read the file from")
	helpersCmd.AddCommand(getFileCmd)

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func isBinary(data []byte) bool {
	// Consider it binary if it contains a null byte or lots of non-printable characters
	nonPrintable := 0
	for _, b := range data {
		if b == 0 {
			return true
		}
		if (b < 32 || b > 126) && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(len(data)) > 0.3
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
