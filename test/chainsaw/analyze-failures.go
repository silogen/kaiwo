// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// This script can be used to extract failures from a Chainsaw JSON report file

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type ChainsawReport struct {
	Name      string `json:"name"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Tests     []Test `json:"tests"`
}

type Test struct {
	BasePath  string `json:"basePath"`
	Name      string `json:"name"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Namespace string `json:"namespace"`
	Steps     []Step `json:"steps"`
}

type Step struct {
	Name       string      `json:"name"`
	StartTime  string      `json:"startTime"`
	EndTime    string      `json:"endTime"`
	Operations []Operation `json:"operations"`
}

type Operation struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	StartTime string   `json:"startTime"`
	EndTime   string   `json:"endTime"`
	Failure   *Failure `json:"failure,omitempty"`
}

type Failure struct {
	Error string `json:"error"`
}

type FailureSummary struct {
	Summary struct {
		Total  int `json:"total"`
		Passed int `json:"passed"`
		Failed int `json:"failed"`
	} `json:"summary"`
	Failures []FailedTest `json:"failures"`
}

type FailedTest struct {
	Name        string       `json:"name"`
	BasePath    string       `json:"basePath"`
	Namespace   string       `json:"namespace"`
	FailedSteps []FailedStep `json:"failedSteps"`
	AllSteps    []Step       `json:"allSteps"`
}

type FailedStep struct {
	StepName         string            `json:"stepName"`
	FailedOperations []FailedOperation `json:"failedOperations"`
}

type FailedOperation struct {
	OperationName string `json:"operationName"`
	OperationType string `json:"operationType"`
	Error         string `json:"error"`
}

func main() {
	format := flag.String("format", "json", "Output format: json or text")
	reportPath := flag.String("report", "chainsaw-report.json", "Path to chainsaw report")
	flag.Parse()

	// Get absolute path
	absPath, err := filepath.Abs(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Read the report file
	data, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading report file: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON
	var report ChainsawReport
	if err := json.Unmarshal(data, &report); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	// Analyze failures
	summary := analyzeFailures(report)

	// Output based on format
	switch *format {
	case "json":
		outputJSON(summary)
	case "text":
		outputText(summary)
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s (use 'json' or 'text')\n", *format)
		os.Exit(1)
	}
}

func analyzeFailures(report ChainsawReport) FailureSummary {
	var summary FailureSummary
	summary.Summary.Total = len(report.Tests)

	for _, test := range report.Tests {
		hasFailure := false
		var failedSteps []FailedStep

		for _, step := range test.Steps {
			var failedOps []FailedOperation
			for _, op := range step.Operations {
				if op.Failure != nil {
					hasFailure = true
					failedOps = append(failedOps, FailedOperation{
						OperationName: op.Name,
						OperationType: op.Type,
						Error:         op.Failure.Error,
					})
				}
			}

			if len(failedOps) > 0 {
				failedSteps = append(failedSteps, FailedStep{
					StepName:         step.Name,
					FailedOperations: failedOps,
				})
			}
		}

		if hasFailure {
			summary.Summary.Failed++
			summary.Failures = append(summary.Failures, FailedTest{
				Name:        test.Name,
				BasePath:    test.BasePath,
				Namespace:   test.Namespace,
				FailedSteps: failedSteps,
				AllSteps:    test.Steps,
			})
		} else {
			summary.Summary.Passed++
		}
	}

	return summary
}

func outputJSON(summary FailureSummary) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

func outputText(summary FailureSummary) {
	fmt.Printf("=== Chainsaw Test Failure Summary ===\n\n")
	fmt.Printf("Total Tests: %d\n", summary.Summary.Total)
	fmt.Printf("Passed: %d\n", summary.Summary.Passed)
	fmt.Printf("Failed: %d\n\n", summary.Summary.Failed)

	if summary.Summary.Failed == 0 {
		fmt.Println("✓ All tests passed!")
		return
	}

	fmt.Printf("=== Failed Tests (%d) ===\n\n", summary.Summary.Failed)

	for i, failure := range summary.Failures {
		fmt.Printf("%d. %s\n", i+1, failure.Name)
		fmt.Printf("   Path: %s\n", failure.BasePath)
		fmt.Printf("   Namespace: %s\n\n", failure.Namespace)

		fmt.Printf("   Failed Steps:\n")
		for _, step := range failure.FailedSteps {
			fmt.Printf("   - %s\n", step.StepName)
			for _, op := range step.FailedOperations {
				fmt.Printf("     • Operation: %s (type: %s)\n", op.OperationName, op.OperationType)
				fmt.Printf("       Error:\n")
				// Indent error message
				for _, line := range splitLines(op.Error) {
					fmt.Printf("       %s\n", line)
				}
				fmt.Println()
			}
		}

		fmt.Printf("   All Steps in Test:\n")
		for j, step := range failure.AllSteps {
			marker := "  "
			// Mark failed steps
			for _, failedStep := range failure.FailedSteps {
				if failedStep.StepName == step.Name {
					marker = "✗ "
					break
				}
			}
			fmt.Printf("   %s%d. %s\n", marker, j+1, step.Name)
		}
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
