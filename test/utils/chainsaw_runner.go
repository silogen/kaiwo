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

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ChainsawTestBlock represents a single test execution block
type ChainsawTestBlock struct {
	Name        string
	Description string
	TestPaths   []string
	Config      *ChainsawExecutionConfig
}

// ChainsawTestRunner manages multiple test blocks with organized reporting
type ChainsawTestRunner struct {
	SuiteStartTime   time.Time
	ReportBaseDir    string
	LokiNamespace    string
	LokiServiceName  string
	LokiServicePort  int
	kubernetesClient kubernetes.Interface
	kubernetesConfig *rest.Config
}

// NewChainsawTestRunner creates a new test runner with organized reporting
func NewChainsawTestRunner(lokiNamespace, lokiServiceName string, lokiServicePort int) *ChainsawTestRunner {
	suiteStartTime := time.Now()
	reportBaseDir := fmt.Sprintf("/tmp/kaiwo-test-reports/%s", suiteStartTime.Format("20060102-150405"))

	// Initialize Kubernetes client
	config, err := getKubernetesConfig()
	if err != nil {
		fmt.Printf("Warning: Could not initialize Kubernetes client: %v\n", err)
		config = nil
	}

	var client kubernetes.Interface
	if config != nil {
		client, err = kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Printf("Warning: Could not create Kubernetes client: %v\n", err)
			client = nil
		}
	}

	return &ChainsawTestRunner{
		SuiteStartTime:   suiteStartTime,
		ReportBaseDir:    reportBaseDir,
		LokiNamespace:    lokiNamespace,
		LokiServiceName:  lokiServiceName,
		LokiServicePort:  lokiServicePort,
		kubernetesClient: client,
		kubernetesConfig: config,
	}
}

// getKubernetesConfig gets the Kubernetes configuration
func getKubernetesConfig() (*rest.Config, error) {
	// Try in-cluster config first
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	// Fall back to kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := os.Getenv("HOME"); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// RunTestBlock executes a single test block with organized reporting
func (r *ChainsawTestRunner) RunTestBlock(block ChainsawTestBlock, additionalEnv string) error {
	// Create block-specific report directory
	blockReportDir := filepath.Join(r.ReportBaseDir, block.Name)
	if err := os.MkdirAll(blockReportDir, 0o755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	// Update config with test paths
	config := block.Config
	config.Tests = block.TestPaths

	// RunWithOutput the test block
	reportPath := filepath.Join(blockReportDir, fmt.Sprintf("%s-report.json", block.Name))
	err := r.runChainsawWithReport(config, reportPath, additionalEnv)

	// Always parse the report for failures, even if test succeeded
	if parseErr := r.parseAndProcessFailures(reportPath, block.Name); parseErr != nil {
		fmt.Printf("Warning: Could not parse test report for %s: %v\n", block.Name, parseErr)
	}

	return err
}

// runChainsawWithReport executes chainsaw and generates JSON report
func (r *ChainsawTestRunner) runChainsawWithReport(config *ChainsawExecutionConfig, reportPath, additionalEnv string) error {
	moduleRootPath := GetModuleRoot()

	args := []string{
		"test",
		"--report-format=JSON",
		"--report-name=" + strings.TrimSuffix(filepath.Base(reportPath), ".json"),
		"--report-path=" + filepath.Dir(reportPath),
	}

	if config.ConfigPath != "" {
		args = append(args, "--config", config.ConfigPath)
	}

	// Add values file if needed
	if len(config.Values) > 0 {
		valuesFile, err := r.createValuesFile(config.Values)
		if err != nil {
			return fmt.Errorf("creating values file: %w", err)
		}
		defer os.Remove(valuesFile)
		args = append(args, "--values", valuesFile)
	}

	// Add test paths
	for _, testPath := range config.Tests {
		args = append(args, filepath.Join(moduleRootPath, testPath))
	}

	fmt.Printf("🔧 Running chainsaw: %s\n", strings.Join(args, " "))

	cmd := exec.Command("chainsaw", args...)
	if additionalEnv != "" {
		cmd.Env = append(os.Environ(), additionalEnv)
	}
	// cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// createValuesFile creates a temporary values file for chainsaw
func (r *ChainsawTestRunner) createValuesFile(values []ChainsawValue) (string, error) {
	valuesFile, err := os.CreateTemp("", "kaiwo-chainsaw-values-*.yaml")
	if err != nil {
		return "", err
	}
	defer valuesFile.Close()

	chainsawValues := map[string]string{}
	for _, value := range values {
		val, err := value.GetValue()
		if err != nil {
			return "", err
		}
		chainsawValues[value.Name] = val
	}

	enc := yaml.NewEncoder(valuesFile)
	if err := enc.Encode(chainsawValues); err != nil {
		return "", err
	}

	return valuesFile.Name(), nil
}
