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

package tuicomponents

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/k8s"
)

type StepResult string

var (
	StepResultOk       StepResult = "ok"
	StepResultErr      StepResult = "err"
	StepResultPrevious StepResult = "previous"
	StepResultQuit     StepResult = "quit"
)

// RunStep provides a way to build multistep hierarchical terminal UIs
// The function returns a result of the step to indicate what should be done next,
// as well as an optional next RunStep to run
type RunStep[T any] func(context.Context, k8s.KubernetesClients, *T) (StepResult, RunStep[T], error)

// RunSteps manages the step execution flow by navigating between the steps
func RunSteps[T any](ctx context.Context, clients k8s.KubernetesClients, state *T, entrypoint RunStep[T]) error {
	stack := []RunStep[T]{entrypoint}

	for {

		if len(stack) == 0 {
			return nil
		}

		currentStep := stack[len(stack)-1]
		result, nextStep, err := currentStep(ctx, clients, state)
		if err != nil {
			return fmt.Errorf("step execution failed: %w", err)
		}

		if result == StepResultOk {
			if nextStep != nil {
				stack = append(stack, nextStep)
			} else {
				// Nothing left to do
				return nil
			}
		} else if result == StepResultPrevious {
			stack = stack[:len(stack)-1]
		} else if result == StepResultQuit {
			return nil
		} else {
			return fmt.Errorf("unexpected result from step (no error reported): %s", result)
		}

	}
}
