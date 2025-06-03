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

package podlist

import (
	"context"
	"fmt"

	"github.com/charmbracelet/huh"

	cliutils "github.com/silogen/kaiwo/pkg/cli/utils"
	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

func runCommandAction(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	execCommand := ""
	execInteractive := true
	execTTY := true

	var command []string

	commandPlaceholder := "Command to run"

	for {
		f := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Command").Value(&execCommand).Placeholder(commandPlaceholder),
			huh.NewConfirm().Title("Interactive").Value(&execInteractive),
			huh.NewConfirm().Title("TTY").Value(&execTTY)),
		)

		err := f.Run()
		if err != nil {
			return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to fetch input: %w", err)
		}

		if execCommand == "" {
			commandPlaceholder = "Please enter command to run"
			continue
		}

		command = cliutils.ParseCommand(execCommand)
		if err := cliutils.ValidateCommand(command[0]); err != nil {
			return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to validate command: %w", err)
		}

		break
	}

	if err := cliutils.ExecInContainer(
		ctx,
		clients.Clientset,
		clients.Kubeconfig,
		state.PodName,
		state.ContainerName,
		state.Workload.GetKaiwoWorkloadObject().GetNamespace(),
		command,
		execInteractive,
		execTTY,
	); err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to run command: %w", err)
	}
	return tuicomponents.StepResultOk, nil, nil
}
