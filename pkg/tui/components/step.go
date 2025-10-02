/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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

		switch result {
		case StepResultOk:
			if nextStep != nil {
				stack = append(stack, nextStep)
			} else {
				// Nothing left to do
				return nil
			}
		case StepResultPrevious:
			stack = stack[:len(stack)-1]
		case StepResultQuit:
			return nil
		default:
			return fmt.Errorf("unexpected result from step (no error reported): %s", result)
		}

	}
}
