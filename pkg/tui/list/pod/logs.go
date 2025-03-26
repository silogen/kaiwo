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
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/sirupsen/logrus"

	cliutils "github.com/silogen/kaiwo/pkg/cli/utils"
	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

func runViewLogsAction(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	follow := true
	lines := ""
	var numLines int

	for {
		err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Tail lines (-1 shows all lines)").Value(&lines).Placeholder("-1"),
			huh.NewConfirm().Title("Follow").Value(&follow),
		)).Run()
		if err != nil {
			return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to fetch input: %w", err)
		}

		if lines == "" {
			lines = "-1"
		}

		numLines, err = strconv.Atoi(lines)
		if err != nil || numLines == 0 {
			logrus.Warn("Lines must be a non-zero integer")
			continue
		}

		break
	}

	if err := cliutils.OutputLogs(ctx, clients.Clientset, state.PodName, state.ContainerName, int64(numLines), state.Workload.GetObjectMeta().Namespace, follow); err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to output the logs: %w", err)
	}

	return tuicomponents.StepResultOk, nil, nil
}
