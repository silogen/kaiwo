/*
Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// TestRunnerHelper provides a shared interface for running Chainsaw test blocks
type TestRunnerHelper struct {
	runner          *ChainsawTestRunner
	kaiwoBuildPath  string
	registeredPaths map[string]*TestRegistry // Track registered paths and their report data
}

// TestRegistry holds information about registered test paths
type TestRegistry struct {
	BasePath      string
	TestBlockName string
	ReportPath    string
}

// TestTreeNode represents a node in the test discovery tree
type TestTreeNode struct {
	Name     string
	Path     string
	IsTest   bool // true if this directory contains chainsaw-test.yaml
	Children []*TestTreeNode
	Parent   *TestTreeNode
}

// TestDiscoveryTree represents the complete test discovery tree
type TestDiscoveryTree struct {
	Root      *TestTreeNode
	TestCount int
}

// ChainsawTestSpec represents the structure of a chainsaw-test.yaml file
type ChainsawTestSpec struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Steps []ChainsawTestStep `yaml:"steps"`
	} `yaml:"spec"`
}

// ChainsawTestStep represents a step in the chainsaw test
type ChainsawTestStep struct {
	Name string                   `yaml:"name,omitempty"`
	Try  []map[string]interface{} `yaml:"try,omitempty"`
}

// NewTestRunnerHelper creates a new test runner helper
func NewTestRunnerHelper() *TestRunnerHelper {
	// Default Loki service configuration (can be overridden via env vars)
	lokiNamespace := getEnvOrDefault("LOKI_NAMESPACE", "monitoring")
	lokiServiceName := getEnvOrDefault("LOKI_SERVICE_NAME", "monitoring-loki")
	lokiServicePort := getEnvOrDefaultInt("LOKI_SERVICE_PORT", 3100)

	return &TestRunnerHelper{
		runner:          NewChainsawTestRunner(lokiNamespace, lokiServiceName, lokiServicePort),
		kaiwoBuildPath:  "builds",
		registeredPaths: make(map[string]*TestRegistry),
	}
}

// Helper functions for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// RunTestBlock executes a Chainsaw test block with organized reporting
// This method is designed to be called within existing It blocks, not create new ones
func (h *TestRunnerHelper) RunTestBlock(name, description string, testPaths []string, configPath string, values []ChainsawValue) error {
	By("executing " + description)

	// Setup PATH for CLI tools
	currentPath := os.Getenv("PATH")
	absoluteKaiwoPath, _ := filepath.Abs(h.kaiwoBuildPath)
	newPath := currentPath + ":" + absoluteKaiwoPath

	// Create test block configuration
	block := ChainsawTestBlock{
		Name:        name,
		Description: description,
		TestPaths:   testPaths,
		Config: &ChainsawExecutionConfig{
			ConfigPath:     configPath,
			Values:         values,
			BaseValuesFile: os.Getenv("KAIWO_TEST_BASE_VALUES_FILE"),
		},
	}

	// Execute the test block and return any error for the caller to handle
	return h.runner.RunTestBlock(block, "PATH="+newPath)
}

// GetReportDirectory returns the base report directory for this test run
func (h *TestRunnerHelper) GetReportDirectory() string {
	return h.runner.ReportBaseDir
}

// Register scans a test path and dynamically creates Ginkgo Context/It/By blocks
// that mirror the Chainsaw test structure and check results from reports
func (h *TestRunnerHelper) Register(testPath, name string) {
	moduleRoot := GetModuleRoot()
	fullPath := filepath.Join(moduleRoot, testPath)

	// Build and display the test discovery tree
	discoveryTree := h.buildTestDiscoveryTree(fullPath, testPath)
	h.printTestDiscoveryTree(discoveryTree, name)

	// Register this path for later report correlation
	h.registeredPaths[testPath] = &TestRegistry{
		BasePath:      fullPath,
		TestBlockName: name,
	}

	// Scan and register the directory structure with the named context
	Context(name, func() {
		h.scanAndRegisterDirectory(fullPath, testPath, "")
	})
}

// scanAndRegisterDirectory recursively scans directories and creates Ginkgo structure
func (h *TestRunnerHelper) scanAndRegisterDirectory(dirPath, relativePath, contextPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	// Check if this directory contains a chainsaw-test.yaml
	chainsawTestPath := filepath.Join(dirPath, "chainsaw-test.yaml")
	if _, err := os.Stat(chainsawTestPath); err == nil {
		// This is a test directory - register the test directly without creating a Context for the directory name
		h.registerChainsawTest(chainsawTestPath, relativePath, contextPath)
		return
	}

	// This is an intermediate directory - only create Context for subdirectories that contain tests
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		entryName := entry.Name()
		newDirPath := filepath.Join(dirPath, entryName)
		newRelativePath := filepath.Join(relativePath, entryName)
		newContextPath := contextPath + "/" + entryName

		// Check if this subdirectory contains any tests before creating a Context
		if h.directoryContainsTests(newDirPath) {
			// Check if this directory directly contains a chainsaw-test.yaml (is a test leaf)
			chainsawTestInSubdir := filepath.Join(newDirPath, "chainsaw-test.yaml")
			if _, err := os.Stat(chainsawTestInSubdir); err == nil {
				// This directory directly contains a test - register it without creating a Context wrapper
				h.scanAndRegisterDirectory(newDirPath, newRelativePath, newContextPath)
			} else {
				// Create a Context for this directory level
				Context(entryName, func() {
					h.scanAndRegisterDirectory(newDirPath, newRelativePath, newContextPath)
				})
			}
		} else {
		}
	}
}

// directoryContainsTests recursively checks if a directory contains any chainsaw-test.yaml files
func (h *TestRunnerHelper) directoryContainsTests(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}

	// Check if this directory contains a chainsaw-test.yaml
	chainsawTestPath := filepath.Join(dirPath, "chainsaw-test.yaml")
	if _, err := os.Stat(chainsawTestPath); err == nil {
		return true
	}

	// Recursively check subdirectories
	for _, entry := range entries {
		if entry.IsDir() {
			subDirPath := filepath.Join(dirPath, entry.Name())
			if h.directoryContainsTests(subDirPath) {
				return true
			}
		}
	}

	return false
}

// buildTestDiscoveryTree creates a tree representation of the test structure
func (h *TestRunnerHelper) buildTestDiscoveryTree(rootPath, relativePath string) *TestDiscoveryTree {
	tree := &TestDiscoveryTree{TestCount: 0}
	tree.Root = h.buildTreeNode(rootPath, filepath.Base(relativePath), nil)
	tree.TestCount = h.countTests(tree.Root)
	return tree
}

// buildTreeNode creates a tree node for a directory and recursively processes subdirectories
func (h *TestRunnerHelper) buildTreeNode(dirPath, name string, parent *TestTreeNode) *TestTreeNode {
	node := &TestTreeNode{
		Name:     name,
		Path:     dirPath,
		IsTest:   false,
		Children: make([]*TestTreeNode, 0),
		Parent:   parent,
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return node
	}

	// Check if this directory contains a chainsaw-test.yaml
	chainsawTestPath := filepath.Join(dirPath, "chainsaw-test.yaml")
	if _, err := os.Stat(chainsawTestPath); err == nil {
		node.IsTest = true
		return node // Leaf node with test
	}

	// Process subdirectories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		entryName := entry.Name()
		subDirPath := filepath.Join(dirPath, entryName)

		// Only include directories that contain tests
		if h.directoryContainsTests(subDirPath) {
			childNode := h.buildTreeNode(subDirPath, entryName, node)
			node.Children = append(node.Children, childNode)
		}
	}

	return node
}

// countTests recursively counts the number of test nodes in the tree
func (h *TestRunnerHelper) countTests(node *TestTreeNode) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.IsTest {
		count = 1
	}

	for _, child := range node.Children {
		count += h.countTests(child)
	}

	return count
}

// printTestDiscoveryTree prints a visual tree representation
func (h *TestRunnerHelper) printTestDiscoveryTree(tree *TestDiscoveryTree, contextName string) {
	fmt.Printf("\n🔍 Discovered test structure for '%s':\n", contextName)
	h.printTreeNode(tree.Root, "", true, true)
	fmt.Printf("📊 Total tests discovered: %d\n\n", tree.TestCount)
}

// printTreeNode recursively prints a tree node and its children
func (h *TestRunnerHelper) printTreeNode(node *TestTreeNode, prefix string, isLast bool, isRoot bool) {
	if node == nil {
		return
	}

	// Choose the appropriate tree characters
	var connector, continuation string
	if isRoot {
		connector = ""
		continuation = ""
	} else if isLast {
		connector = "└── "
		continuation = "    "
	} else {
		connector = "├── "
		continuation = "│   "
	}

	// Print the current node
	icon := "📁"
	if node.IsTest {
		icon = "🧪"
	}

	if !isRoot {
		fmt.Printf("%s%s%s %s\n", prefix, connector, icon, node.Name)
	} else {
		fmt.Printf("%s %s\n", icon, node.Name)
	}

	// Print children
	newPrefix := prefix + continuation
	if isRoot {
		newPrefix = ""
	}

	for i, child := range node.Children {
		isChildLast := i == len(node.Children)-1
		h.printTreeNode(child, newPrefix, isChildLast, false)
	}
}

// registerChainsawTest creates the detailed test structure for a single chainsaw test
func (h *TestRunnerHelper) registerChainsawTest(chainsawTestPath, relativePath, contextPath string) {
	// Parse the chainsaw test file
	testSpec, err := h.parseChainsawTest(chainsawTestPath)
	if err != nil {
		// Skip tests that can't be parsed, but log the issue
		fmt.Printf("❌ Warning: Could not parse chainsaw test %s: %v\n", chainsawTestPath, err)
		return
	}

	// Create an It-block directly for the test (skip the intermediate Context and step names)
	// Make the test name include a reference to the chainsaw file for better IDE integration
	moduleRoot := GetModuleRoot()
	relativeYamlPath, _ := filepath.Rel(moduleRoot, chainsawTestPath)
	testDisplayName := fmt.Sprintf("%s (%s)", testSpec.Metadata.Name, relativeYamlPath)

	It(testDisplayName, func() {
		// Output the actual test file location prominently in console
		fmt.Printf("🪚 Chainsaw test file: %s:1\n", chainsawTestPath)

		// Get the actual test timing from the report for display
		var testStartTime, testEndTime time.Time
		var hasFailures bool

		if reportPath := h.findReportForPath(relativePath, testSpec.Metadata.Name); reportPath != "" {
			if report, err := h.parseReport(reportPath); err == nil {
				for _, test := range report.Tests {
					if test.Name == testSpec.Metadata.Name {
						testStartTime = test.StartTime
						testEndTime = test.EndTime
						break
					}
				}
			}
		}

		// Check all operations without By() to keep output clean for successful tests
		for stepIndex, step := range testSpec.Spec.Steps {
			for opIndex := range step.Try {
				// This is where we check the report for this specific operation
				err := h.checkOperationResultWithError(relativePath, testSpec.Metadata.Name, stepIndex, opIndex)
				if err != nil {
					hasFailures = true
				}
			}
		}

		// Add the actual execution time to the test report
		if !testStartTime.IsZero() && !testEndTime.IsZero() {
			actualDuration := testEndTime.Sub(testStartTime)
			status := "PASSED"
			if hasFailures {
				status = "FAILED"
			}
			AddReportEntry(fmt.Sprintf("Actual test duration: %s (%s)", actualDuration.Round(time.Millisecond), status))
		}
	})
}

// parseChainsawTest reads and parses a chainsaw-test.yaml file
func (h *TestRunnerHelper) parseChainsawTest(filePath string) (*ChainsawTestSpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var spec ChainsawTestSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	return &spec, nil
}

// findFailureLineInYAML just returns 1 to make the file clickable - line detection was too complex
func (h *TestRunnerHelper) findFailureLineInYAML(yamlFilePath string, stepIndex, opIndex int) int {
	return 1 // Simple approach: just make the file clickable
}

// checkOperationResultWithError checks the Chainsaw report and returns an error if the operation failed
func (h *TestRunnerHelper) checkOperationResultWithError(relativePath, testName string, stepIndex, opIndex int) error {
	// Find the report file for this test path
	reportPath := h.findReportForPath(relativePath, testName)
	if reportPath == "" {
		err := fmt.Errorf("no report found for test %s at path %s", testName, relativePath)
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	// Parse the report
	report, err := h.parseReport(reportPath)
	if err != nil {
		err := fmt.Errorf("could not parse report %s: %v", reportPath, err)
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	// Find the specific test in the report
	var testResult *ChainsawTest
	for _, test := range report.Tests {
		if strings.HasSuffix(test.BasePath, relativePath) && test.Name == testName {
			testResult = &test
			break
		}
	}

	if testResult == nil {
		err := fmt.Errorf("test %s not found in report %s", testName, reportPath)
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	// Check if we have the requested step and operation
	if stepIndex >= len(testResult.Steps) {
		err := fmt.Errorf("step %d not found in test %s (only %d steps)", stepIndex, testName, len(testResult.Steps))
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	step := testResult.Steps[stepIndex]
	if opIndex >= len(step.Operations) {
		err := fmt.Errorf("operation %d not found in step %d of test %s (only %d operations)",
			opIndex, stepIndex, testName, len(step.Operations))
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	operation := step.Operations[opIndex]

	// Check if the operation failed
	if operation.Failure != nil {
		// Find the chainsaw test file path to get line numbers
		chainsawTestPath := ""
		for registeredPath := range h.registeredPaths {
			if strings.Contains(relativePath, registeredPath) {
				// Reconstruct the path to the chainsaw-test.yaml
				moduleRoot := GetModuleRoot()
				pathFromModule := strings.TrimPrefix(testResult.BasePath, moduleRoot+"/")
				chainsawTestPath = filepath.Join(moduleRoot, pathFromModule, "chainsaw-test.yaml")
				break
			}
		}

		// Find the line number where this operation failed
		failureLine := 1
		if chainsawTestPath != "" {
			failureLine = h.findFailureLineInYAML(chainsawTestPath, stepIndex, opIndex)
		}

		// Output the failure with a clickable link and operation details
		fmt.Printf("❌ Failure at %s:%d (step %d, operation %d - %s): %s\n",
			chainsawTestPath, failureLine, stepIndex+1, opIndex+1, operation.Type, operation.Failure.Error)

		// Collect and display logs for this specific failure
		h.collectAndDisplayFailureLogs(testResult, stepIndex, opIndex, operation)

		// This marks the test as failed but allows other tests to continue
		err := fmt.Errorf("operation %d in step %d failed: %s", opIndex+1, stepIndex+1, operation.Failure.Error)
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	return nil
}

// checkOperationResult checks the Chainsaw report for a specific operation result (legacy method)
func (h *TestRunnerHelper) checkOperationResult(relativePath, testName string, stepIndex, opIndex int) {
	// Find the report file for this test path

	reportPath := h.findReportForPath(relativePath, testName)
	if reportPath == "" {
		// Test ANSI colors with enhanced detection
		colorTest := h.colorize("red", "No report found")
		fmt.Printf("%s for test %s at path %s\n", colorTest, testName, relativePath)
		return
	}

	// Parse the report
	report, err := h.parseReport(reportPath)
	if err != nil {
		Fail(fmt.Sprintf("Could not parse report %s: %v", reportPath, err))
		return
	}

	// Find the specific test in the report
	var testResult *ChainsawTest
	for _, test := range report.Tests {
		if strings.HasSuffix(test.BasePath, relativePath) && test.Name == testName {
			testResult = &test
			break
		}
	}

	if testResult == nil {
		Fail(fmt.Sprintf("Test %s not found in report %s", testName, reportPath))
		return
	}

	// Check if we have the requested step and operation
	if stepIndex >= len(testResult.Steps) {
		Fail(fmt.Sprintf("Step %d not found in test %s (only %d steps)", stepIndex, testName, len(testResult.Steps)))
		return
	}

	step := testResult.Steps[stepIndex]
	if opIndex >= len(step.Operations) {
		Fail(fmt.Sprintf("Operation %d not found in step %d of test %s (only %d operations)",
			opIndex, stepIndex, testName, len(step.Operations)))
		return
	}

	operation := step.Operations[opIndex]

	// Check if the operation failed
	if operation.Failure != nil {
		// Collect and display logs for this specific failure
		h.collectAndDisplayFailureLogs(testResult, stepIndex, opIndex, operation)

		// This marks the test as failed but allows other tests to continue
		err := fmt.Errorf("operation %d in step %d failed: %s", opIndex+1, stepIndex+1, operation.Failure.Error)
		Expect(err).NotTo(HaveOccurred())
	}

	// Operation succeeded - success messages removed to reduce verbosity
	// Only failures will be shown, successful operations are silent
}

// findReportForPath finds the report file that corresponds to a given test path
func (h *TestRunnerHelper) findReportForPath(relativePath, testName string) string {
	// First, try to find the report using the registered test block name
	for registeredPath, registry := range h.registeredPaths {
		if strings.Contains(relativePath, registeredPath) && registry.TestBlockName != "" {
			// Look for the specific named report file
			reportPath := filepath.Join(h.runner.ReportBaseDir, registry.TestBlockName, fmt.Sprintf("%s-report.json", registry.TestBlockName))
			if _, err := os.Stat(reportPath); err == nil {
				// Verify this report contains our test
				if report, err := h.parseReport(reportPath); err == nil {
					for _, test := range report.Tests {
						if strings.HasSuffix(test.BasePath, relativePath) && test.Name == testName {
							return reportPath
						}
					}
				}
			}
		}
	}

	// Fallback: Look for report files in the report directory (legacy behavior)
	pattern := filepath.Join(h.runner.ReportBaseDir, "*", "*-report.json")
	reportFiles, err := filepath.Glob(pattern)
	if err != nil {
		return ""
	}

	// Check each report file
	for _, reportFile := range reportFiles {
		if report, err := h.parseReport(reportFile); err == nil {
			// Check if this report contains our test
			for _, test := range report.Tests {
				if strings.HasSuffix(test.BasePath, relativePath) && test.Name == testName {
					return reportFile
				}
			}
		}
	}

	return ""
}

// parseReport parses a Chainsaw JSON report file
func (h *TestRunnerHelper) parseReport(reportPath string) (*ChainsawReport, error) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("reading report file: %w", err)
	}

	var report ChainsawReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return &report, nil
}

// collectAndDisplayFailureLogs collects and displays logs for a specific failed operation
func (h *TestRunnerHelper) collectAndDisplayFailureLogs(testResult *ChainsawTest, stepIndex, opIndex int, operation ChainsawOperation) {
	// fmt.Printf("\n🔴 LOGS FOR FAILED OPERATION\n")
	// fmt.Printf("=" + strings.Repeat("=", 40) + "\n")

	// Create a failure object for the log collection system
	failure := TestFailure{
		TestName:      testResult.Name,
		Namespace:     testResult.Namespace,
		Phase:         fmt.Sprintf("step-%d", stepIndex+1),
		BasePath:      testResult.BasePath,
		Operation:     operation.Type,
		Error:         operation.Failure.Error,
		Timestamp:     operation.StartTime,
		TestBlockName: "manual-check",
	}

	// Use the existing log collection system
	debugInfo := h.runner.collectDebugInformationFromLoki(failure)

	// Add Chainsaw report entries to the debug information (filtered by namespace)
	reportPath := h.findReportForPath(strings.TrimPrefix(testResult.BasePath, GetModuleRoot()+"/"), testResult.Name)
	if reportPath != "" {
		chainsawEntries := h.runner.parseReportToLogEntriesWithContext(reportPath, testResult.Namespace, stepIndex, opIndex, operation.Type)
		debugInfo = append(debugInfo, chainsawEntries...)
		// fmt.Printf("   📊 Added %d Chainsaw report entries for namespace %s\n", len(chainsawEntries), testResult.Namespace)
	}

	if len(debugInfo) > 0 {
		// fmt.Printf("\n📋 COLLECTED LOGS\n")
		// fmt.Printf("-" + strings.Repeat("-", 40) + "\n")
		h.runner.displayAggregatedLogs(debugInfo, testResult.Namespace)
	} else {
		fmt.Printf("⚠️ No debug information could be collected\n")
	}

	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
}

// ExecuteChainsawTests runs all registered Chainsaw test paths in parallel
func (h *TestRunnerHelper) ExecuteChainsawTests(configPath string, values []ChainsawValue) error {
	// Setup PATH for CLI tools
	currentPath := os.Getenv("PATH")
	absoluteKaiwoPath, _ := filepath.Abs(h.kaiwoBuildPath)
	newPath := currentPath + ":" + absoluteKaiwoPath

	var testPaths []string
	var blockName string

	for path, registry := range h.registeredPaths {
		testPaths = append(testPaths, path)
		if blockName == "" && registry.TestBlockName != "" {
			blockName = registry.TestBlockName
		}
	}

	// Default block name if none found
	if blockName == "" {
		blockName = "chainsaw-execution"
	}

	// Create test block configuration
	block := ChainsawTestBlock{
		Name:        blockName,
		Description: fmt.Sprintf("Chainsaw test execution: %s", blockName),
		TestPaths:   testPaths,
		Config: &ChainsawExecutionConfig{
			ConfigPath:     configPath,
			Values:         values,
			BaseValuesFile: os.Getenv("KAIWO_TEST_BASE_VALUES_FILE"),
		},
	}

	By("Executing the Chainsaw tests")

	// Execute all tests
	err := h.runner.RunTestBlock(block, "PATH="+newPath)
	if err != nil {
		fmt.Printf("⚠️ Chainsaw execution completed with error (may be expected): %v\n", err)
	} else {
		fmt.Printf("✅ Chainsaw execution completed for %d test paths\n", len(testPaths))
	}
	return err
}

// CheckSpecificTest checks results for a specific test path (can be branch or leaf)
// Uses shortened paths relative to the test execution context
// Examples:
//   - "kaiwojob" (checks all kaiwojob tests)
//   - "kaiwojob/batchjob" (checks specific batchjob test under kaiwojob)
//   - "workloads" (checks all workload tests)
func (h *TestRunnerHelper) CheckSpecificTest(testPath string) error {
	By(fmt.Sprintf("checking results for test path: %s", testPath))

	// Find all reports that match this path (could be multiple)
	matchingTests := h.findTestsForPath(testPath)

	if len(matchingTests) == 0 {
		err := fmt.Errorf("no test results found for path: %s", testPath)
		Expect(err).NotTo(HaveOccurred())
		return err
	}

	fmt.Printf("📊 Found %d test(s) matching path %s\n", len(matchingTests), testPath)

	// Check each matching test
	hasFailures := false
	for _, testResult := range matchingTests {
		if h.checkTestResult(testResult) {
			hasFailures = true
		}
	}

	if hasFailures {
		err := fmt.Errorf("one or more tests failed for path: %s", testPath)
		Expect(err).NotTo(HaveOccurred())
		return err
	} else {
		fmt.Printf("✅ All tests passed for path: %s\n", testPath)
		return nil
	}
}

// findTestsForPath finds all test results that match a given path pattern
func (h *TestRunnerHelper) findTestsForPath(targetPath string) []ChainsawTest {
	var matchingTests []ChainsawTest

	// Look for all report files
	pattern := filepath.Join(h.runner.ReportBaseDir, "*", "*-report.json")
	reportFiles, err := filepath.Glob(pattern)
	if err != nil {
		return matchingTests
	}

	// Check each report file
	for _, reportFile := range reportFiles {
		if report, err := h.parseReport(reportFile); err == nil {
			// Check each test in the report
			for _, test := range report.Tests {
				// Check if this test's base path matches our target path
				if h.pathMatches(test.BasePath, targetPath) {
					matchingTests = append(matchingTests, test)
				}
			}
		}
	}

	return matchingTests
}

// pathMatches checks if a test's base path matches the target path
// Supports shortened paths like "kaiwojob/batchjob" or "kaiwojob"
func (h *TestRunnerHelper) pathMatches(testBasePath, targetPath string) bool {
	// Convert test base path to relative path from module root
	moduleRoot := GetModuleRoot()
	relativeTestPath, err := filepath.Rel(moduleRoot, testBasePath)
	if err != nil {
		// If we can't get relative path, fall back to absolute comparison
		relativeTestPath = testBasePath
	}

	// Normalize path separators
	relativeTestPath = filepath.ToSlash(relativeTestPath)
	targetPath = filepath.ToSlash(targetPath)

	// Check if the test path ends with our target path
	// This allows "kaiwojob/batchjob" to match "test/chainsaw/tests/no-gpu/workloads/kaiwojob/batchjob"
	if strings.HasSuffix(relativeTestPath, targetPath) {
		// Make sure it's a proper path boundary (not partial match)
		beforeTarget := strings.TrimSuffix(relativeTestPath, targetPath)
		return beforeTarget == "" || strings.HasSuffix(beforeTarget, "/")
	}

	// Also check if our target path is a subdirectory of the test path
	// This allows "kaiwojob" to match "test/chainsaw/tests/no-gpu/workloads/kaiwojob/batchjob"
	if strings.Contains(relativeTestPath, targetPath) {
		// Find the position of targetPath in the relative path
		targetIndex := strings.Index(relativeTestPath, targetPath)
		if targetIndex >= 0 {
			// Check if it's at a path boundary
			beforeTarget := relativeTestPath[:targetIndex]
			afterTarget := relativeTestPath[targetIndex+len(targetPath):]

			validBefore := beforeTarget == "" || strings.HasSuffix(beforeTarget, "/")
			validAfter := afterTarget == "" || strings.HasPrefix(afterTarget, "/")

			return validBefore && validAfter
		}
	}

	return false
}

// checkTestResult checks a single test result and returns true if there were failures
func (h *TestRunnerHelper) checkTestResult(testResult ChainsawTest) bool {
	hasFailures := false

	fmt.Printf("\n🔍 Checking test: %s (namespace: %s)\n", testResult.Name, testResult.Namespace)

	// Check each step and operation for failures
	for stepIndex, step := range testResult.Steps {
		for opIndex, operation := range step.Operations {
			if operation.Failure != nil {
				hasFailures = true
				fmt.Printf("❌ Step %d, Operation %d failed: %s\n",
					stepIndex+1, opIndex+1, operation.Failure.Error)

				// Collect and display logs for this failure
				h.collectAndDisplayFailureLogs(&testResult, stepIndex, opIndex, operation)
			} else {
				fmt.Printf("✅ Step %d, Operation %d succeeded (%s)\n",
					stepIndex+1, opIndex+1, operation.Type)
			}
		}
	}

	return hasFailures
}

// DisplaySummary shows a summary of all test results
func (h *TestRunnerHelper) DisplaySummary() {
	By("displaying test execution summary")

	fmt.Printf("\n📊 TEST EXECUTION SUMMARY\n")
	fmt.Printf("========================\n")
	fmt.Printf("📁 Reports saved to: %s\n", h.runner.ReportBaseDir)
	fmt.Printf("🕐 Suite started at: %s\n", h.runner.SuiteStartTime.Format("15:04:05"))
	fmt.Printf("🕐 Duration: %s\n", time.Since(h.runner.SuiteStartTime).Round(time.Second))

	// Check for any failure files in the report directory
	failureFiles, _ := filepath.Glob(filepath.Join(h.runner.ReportBaseDir, "*-failures.json"))
	if len(failureFiles) > 0 {
		fmt.Printf("🔴 Failure files: %d\n", len(failureFiles))
		for _, file := range failureFiles {
			fmt.Printf("   - %s\n", filepath.Base(file))
		}
	} else {
		fmt.Printf("✅ No failures detected\n")
	}
}

// colorize adds ANSI colors with intelligent detection and fallback
func (h *TestRunnerHelper) colorize(color, text string) string {
	// Force colors if environment variable is set (useful for IDE terminals)
	forceColors := os.Getenv("FORCE_COLOR") == "1" || os.Getenv("FORCE_COLOR") == "true"

	// Check if we should use colors
	if !forceColors && !isColorTerminal() {
		return text // Return plain text if colors not supported
	}

	// ANSI color codes
	colors := map[string]string{
		"reset":       "\033[0m",
		"red":         "\033[31m",
		"green":       "\033[32m",
		"yellow":      "\033[33m",
		"blue":        "\033[34m",
		"purple":      "\033[35m",
		"cyan":        "\033[36m",
		"gray":        "\033[37m",
		"brightRed":   "\033[91m",
		"brightGreen": "\033[92m",
		"brightBlue":  "\033[94m",
	}

	colorCode, exists := colors[color]
	if !exists {
		return text // Return plain text if color not found
	}

	return colorCode + text + colors["reset"]
}

// isColorTerminal detects if the current terminal supports ANSI colors
func isColorTerminal() bool {
	// Check common environment variables that indicate color support
	term := os.Getenv("TERM")
	colorTerm := os.Getenv("COLORTERM")

	// Debug: uncomment to see environment detection
	// fmt.Printf("DEBUG: TERM='%s', COLORTERM='%s', CI='%s'\n", term, colorTerm, os.Getenv("CI"))

	// Force color support for common IDE terminals
	if strings.Contains(term, "xterm") ||
		strings.Contains(term, "screen") ||
		strings.Contains(term, "tmux") ||
		colorTerm != "" ||
		os.Getenv("CI") == "true" { // CI environments usually support colors
		return true
	}

	// GoLand and other IDEs often don't set TERM properly, but support colors
	// Check if we're likely in an IDE
	if os.Getenv("TERM") == "" || term == "dumb" {
		// Might be an IDE - try colors anyway if output is a terminal
		return true
	}

	return false
}
