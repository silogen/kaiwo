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

package testutils

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type ChainsawTestStep struct {
	Name   string
	Action string
	Status string
	Raw    string
	Output string
}

type ChainsawTestWrapper struct {
	Name      string
	Namespace string
	HasError  bool
	Steps     []ChainsawTestStep
}

type ChainsawTestErrorCallback func(namespace string, testName string) error

// RunChainsawTests is a wrapper to run Chainsaw tests, but to receive a callback
// when a test fails. The Chainsaw tests are paused, and continued only after the
// callback returns. This can be used to diagnose the test namespace before Chainsaw
// has a chance to clean in up.
func RunChainsawTests(errorCallback ChainsawTestErrorCallback, catchErrors bool, args ...string) error {
	chainsawArgs := []string{
		"test",
	}
	if catchErrors {
		chainsawArgs = append(chainsawArgs, "--pause-on-failure")
	}
	chainsawArgs = append(chainsawArgs, args...)
	cmd := exec.Command("chainsaw", chainsawArgs...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe for chainsaw: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start chainsaw: %w", err)
	}

	doneOut := make(chan struct{})
	doneErr := make(chan struct{})

	tests := make(map[string]ChainsawTestWrapper)

	var previousStep *ChainsawTestStep
	testsActive := false
	testNameToNamespaceMap := make(map[string]string)
	errorDiagnosed := make(map[string]bool)

	// var enterOnce sync.Once

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.Contains(line, "Failure detected, press ENTER to continue") {
				_, err := stdinPipe.Write([]byte("\n"))
				if err != nil {
					fmt.Printf("Error simulating ENTER: %v\n", err)
				}
				continue
			}

			if strings.HasPrefix(line, "=== CONT") {
				continue
			}
			if strings.HasPrefix(line, "---") {
				testsActive = false
			}

			parts := strings.Split(line, "|")
			if len(parts) == 7 {
				testsActive = true
				testName := stripANSI(strings.TrimSpace(parts[2]))
				testStepName := stripANSI(strings.TrimSpace(parts[3]))
				testStepAction := stripANSI(strings.TrimSpace(parts[4]))
				testStepStatus := stripANSI(strings.TrimSpace(parts[5]))
				testStepTarget := stripANSI(strings.TrimSpace(parts[6]))

				testStep := ChainsawTestStep{
					Name:   testStepName,
					Action: testStepAction,
					Status: testStepStatus,
					Raw:    line,
				}

				previousStep = &testStep

				// Cache the namespace

				if _, exists := tests[testName]; !exists && testStepName == "@chainsaw" {
					testNamespace := strings.TrimSpace(strings.Split(strings.Split(testStepTarget, "@")[1], "/")[0])
					tests[testName] = ChainsawTestWrapper{
						Name:      testName,
						Namespace: testNamespace,
					}
				}

				testWrapper := tests[testName]
				testWrapper.Steps = append(testWrapper.Steps, testStep)

				if testStepStatus == "ERROR" {
					testWrapper.HasError = true
					testNamespace, exists := testNameToNamespaceMap[testName]
					if !exists {
						logrus.Error("Unexpected error")
						continue
					}
					if _, exists := errorDiagnosed[testName]; exists {
						continue
					}
					logrus.Warning("Failed test detected, diagnosing before continuing")
					if err := errorCallback(testNamespace, testName); err != nil {
						logrus.Warningf("Error running diagnosis: %v", err)
					}
					errorDiagnosed[testName] = true
				}
			} else if testsActive && previousStep != nil {
				previousStep.Output += line + "\n"
			}

		}
		if err := scanner.Err(); err != nil {
			logrus.Error("stdout scanner error: %w", err)
		}
		close(doneOut)
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line)
		}
		if err := scanner.Err(); err != nil {
			logrus.Error("stderr scanner error: %w", err)
		}
		close(doneErr)
	}()

	if err := cmd.Wait(); err != nil {
		logrus.Errorf("chainsaw returned error: %v", err)
	}

	// Wait until both output processing goroutines finish.
	select {
	case <-doneOut:
	case <-time.After(5 * time.Second):
		logrus.Warnf("timeout waiting for stdout to finish")
	}
	select {
	case <-doneErr:
	case <-time.After(5 * time.Second):
		logrus.Warnf("timeout waiting for stderr to finish")
	}

	return nil
}
