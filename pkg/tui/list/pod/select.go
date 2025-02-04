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
	"time"

	"github.com/charmbracelet/huh/spinner"
	corev1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/silogen/kaiwo/pkg/workloads/utils"
)

var containerSelectColumns = []string{
	"Logical group",
	"Pod name",
	"Pod phase",
	"Container name",
	"Container status",
}

// ChoosePodAndContainer allows the User to choose the pod and the container they want to interact with
// predicates define an optional list of predicates that must be matched in order to include the pod in the list
func ChoosePodAndContainer(ctx context.Context, clients k8s.KubernetesClients, reference workloads.WorkloadReference, predicates ...utils.PodSelectionPredicate) (string, string, error, bool) {
	state := &tuicomponents.RunState{
		WorkloadReference:      reference,
		PodSelectionPredicates: predicates,
	}

	result, _, err := RunSelectPodAndContainer(
		ctx,
		clients,
		state,
	)

	return state.PodName, state.ContainerName, err, result == tuicomponents.StepResultQuit || result == tuicomponents.StepResultPrevious
}

func RunSelectPodAndContainer(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	var err error

	loadReference := func() {
		err = state.WorkloadReference.Load(ctx, clients.Client)
	}

	if spinnerErr := spinner.New().Title("Loading workload").Action(loadReference).Run(); spinnerErr != nil {
		return tuicomponents.StepResultErr, nil, spinnerErr
	}

	if err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to load workload: %w", err)
	}

	allPods := state.WorkloadReference.GetPods()

	data := gatherPodData(allPods, state.PodSelectionPredicates)

	title := "Select pod and container"
	selectedRow, result, err := tuicomponents.RunSelectTable(data, containerSelectColumns, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the pod: %w", err)
	}
	if result == tuicomponents.StepResultOk {
		state.PodName = data[selectedRow][1]
		state.ContainerName = data[selectedRow][3]
	} else {
		state.PodName = ""
		state.ContainerName = ""
	}
	return result, runSelectAndDoAction, nil
}

func gatherPodData(pods []workloads.WorkloadPod, predicates []utils.PodSelectionPredicate) [][]string {
	var data [][]string

	for _, pod := range pods {
		if !applyPredicates(predicates, pod.Pod) {
			continue
		}
		data = append(data, getContainerData(pod)...)
	}

	return data
}

func applyPredicates(predicates []utils.PodSelectionPredicate, pod corev1.Pod) bool {
	for _, predicate := range predicates {
		if !predicate(pod) {
			return false
		}
	}
	return true
}

func getContainerData(pod workloads.WorkloadPod) [][]string {
	var rows [][]string
	for _, container := range pod.Pod.Status.ContainerStatuses {
		rows = append(rows, formatContainerRow(pod, container))
	}
	for _, container := range pod.Pod.Status.InitContainerStatuses {
		rows = append(rows, formatContainerRow(pod, container))
	}
	return rows
}

func formatContainerRow(pod workloads.WorkloadPod, container corev1.ContainerStatus) []string {
	containerStatus := "N/A"
	if container.State.Running != nil {
		containerStatus = fmt.Sprintf("Running since %s", container.State.Running.StartedAt.Format(time.RFC3339))
	} else if container.State.Waiting != nil {
		containerStatus = fmt.Sprintf("Waiting (%s)", container.State.Waiting.Reason)
	} else if container.State.Terminated != nil {
		containerStatus = fmt.Sprintf("Terminated (%s)", container.State.Terminated.Reason)
	}

	return []string{
		pod.LogicalGroup,
		pod.Pod.Name,
		string(pod.Pod.Status.Phase),
		container.Name,
		containerStatus,
	}
}

func runSelectAndDoAction(_ context.Context, _ k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	columns := []string{
		"Action",
	}

	type runAction string

	var (
		viewLogsAction runAction = "View logs"
		monitorAction  runAction = "Monitor GPUs"
		commandAction  runAction = "Run command"
	)

	data := [][]string{
		{string(viewLogsAction)},
		{string(monitorAction)},
		{string(commandAction)},
	}

	title := fmt.Sprintf("Select action to perform on %s/%s, pod: %s, container %s", state.WorkloadType, state.WorkloadReference.GetName(), state.PodName, state.ContainerName)
	selectedRow, result, err := tuicomponents.RunSelectTable(data, columns, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the pod: %w", err)
	}

	actionMap := map[runAction]tuicomponents.RunStep[tuicomponents.RunState]{
		viewLogsAction: runViewLogsAction,
		monitorAction:  runMonitorAction,
		commandAction:  runCommandAction,
	}

	if result == tuicomponents.StepResultOk {
		selectedAction := runAction(data[selectedRow][0])
		return result, actionMap[selectedAction], nil
	}

	return result, nil, nil
}
