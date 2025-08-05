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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Test report structures for JSON parsing
type ChainsawReport struct {
	Name      string         `json:"name"`
	StartTime time.Time      `json:"startTime"`
	EndTime   time.Time      `json:"endTime"`
	Tests     []ChainsawTest `json:"tests"`
}

type ChainsawTest struct {
	BasePath  string         `json:"basePath"`
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	StartTime time.Time      `json:"startTime"`
	EndTime   time.Time      `json:"endTime"`
	Steps     []ChainsawStep `json:"steps"`
}

type ChainsawStep struct {
	Name       string              `json:"name"`
	StartTime  time.Time           `json:"startTime"`
	EndTime    time.Time           `json:"endTime"`
	Operations []ChainsawOperation `json:"operations"`
}

type ChainsawOperation struct {
	Name      string                    `json:"name"`
	Type      string                    `json:"type"`
	StartTime time.Time                 `json:"startTime"`
	EndTime   time.Time                 `json:"endTime"`
	Failure   *ChainsawOperationFailure `json:"failure,omitempty"`
}

type ChainsawOperationFailure struct {
	Error string `json:"error"`
}

type TestFailure struct {
	TestName      string    `json:"testName"`
	Namespace     string    `json:"namespace"`
	Phase         string    `json:"phase"`
	BasePath      string    `json:"basePath"`
	Operation     string    `json:"operation"`
	Error         string    `json:"error"`
	Timestamp     time.Time `json:"timestamp"`
	TestBlockName string    `json:"testBlockName"`
	ReportPath    string    `json:"reportPath"`
}

// parseAndProcessFailures parses the JSON report and processes any failures
func (r *ChainsawTestRunner) parseAndProcessFailures(reportPath, testBlockName string) error {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("reading report file: %w", err)
	}

	var report ChainsawReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("parsing JSON report: %w", err)
	}

	var failures []TestFailure
	hasFailures := false

	// Process each test for failures
	for _, test := range report.Tests {
		for _, step := range test.Steps {
			for _, operation := range step.Operations {
				if operation.Failure != nil {
					hasFailures = true
					failure := TestFailure{
						TestName:      test.Name,
						Namespace:     test.Namespace,
						Phase:         step.Name,
						BasePath:      test.BasePath,
						Operation:     operation.Type,
						Error:         operation.Failure.Error,
						Timestamp:     operation.StartTime,
						TestBlockName: testBlockName,
						ReportPath:    reportPath,
					}
					failures = append(failures, failure)
				}
			}
		}
	}

	if hasFailures {
		fmt.Printf("\n🔴 FAILURES DETECTED in %s (details will be shown in individual test results)\n", testBlockName)

		// Save failures to file for later reference (Stage 2 will read these)
		r.saveFailuresToFile(failures, testBlockName)
	} else {
		fmt.Printf("✅ All tests passed in %s\n", testBlockName)
	}

	return nil
}

// saveFailuresToFile saves all failures to a JSON file for later analysis
func (r *ChainsawTestRunner) saveFailuresToFile(failures []TestFailure, testBlockName string) {
	failureFile := filepath.Join(r.ReportBaseDir, fmt.Sprintf("%s-failures.json", testBlockName))

	data, err := json.MarshalIndent(failures, "", "  ")
	if err != nil {
		fmt.Printf("Warning: Could not marshal failures: %v\n", err)
		return
	}

	if err := os.WriteFile(failureFile, data, 0o644); err != nil {
		fmt.Printf("Warning: Could not write failures file: %v\n", err)
		return
	}

	fmt.Printf("💾 Failures saved to: %s\n", failureFile)
}
