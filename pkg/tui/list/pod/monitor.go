// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
	"github.com/silogen/kaiwo/pkg/workloads/utils"
)

func runMonitorAction(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	command := utils.ParseCommand(utils.DefaultMonitorCommand)
	if err := utils.ValidateCommand(command[0]); err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to validate command: %w", err)
	}

	if err := utils.ExecInContainer(
		ctx,
		clients.Clientset,
		clients.Kubeconfig,
		state.PodName,
		state.ContainerName,
		state.WorkloadReference.GetNamespace(),
		command,
		true,
		true,
	); err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to run monitor command: %w", err)
	}

	return tuicomponents.StepResultOk, nil, nil
}
