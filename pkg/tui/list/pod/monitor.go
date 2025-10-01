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

package podlist

import (
	"context"
	"fmt"

	cliutils "github.com/silogen/kaiwo/pkg/cli/utils"
	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

func runMonitorAction(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	command := cliutils.ParseCommand(cliutils.DefaultMonitorCommand)
	if err := cliutils.ValidateCommand(command[0]); err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to validate command: %w", err)
	}
	obj := state.Workload.GetKaiwoWorkloadObject()
	if err := cliutils.ExecInContainer(
		ctx,
		clients.Clientset,
		clients.Kubeconfig,
		state.PodName,
		state.ContainerName,
		obj.GetNamespace(),
		command,
		true,
		true,
	); err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to run monitor command: %w", err)
	}

	return tuicomponents.StepResultOk, nil, nil
}
