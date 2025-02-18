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

package workloadlist

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
	podlist "github.com/silogen/kaiwo/pkg/tui/list/pod"
)

func RunSelectWorkloadType(_ context.Context, _ k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	data := [][]string{
		{"job"},
		{"deployment"},
		{"rayjob"},
		{"rayservice"},
	}
	title := "Select the resource type"
	selectedRow, result, err := tuicomponents.RunSelectTable(data, []string{"Workload type"}, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the resource: %w", err)
	}
	if result == tuicomponents.StepResultOk {
		state.WorkloadType = data[selectedRow][0]
	} else {
		state.WorkloadType = ""
	}
	return result, runSelectWorkload, nil
}

func runSelectWorkload(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	panic("")
	//labelSelector := client.MatchingLabels{}
	//
	//if state.User != "" {
	//	labelSelector[workloads.KaiwoUsernameLabel] = state.User
	//}
	//
	//var err error
	//
	//var workloadReferences []workloads.Workload
	//
	//loadReferences := func() {
	//	workloadReferences, err = factory.ListObjects(ctx, clients.Client, state.WorkloadType, labelSelector, client.InNamespace(state.Namespace))
	//}
	//
	//if spinnerErr := spinner.New().Title("Listing workloads").Action(loadReferences).Run(); spinnerErr != nil {
	//	return tuicomponents.StepResultErr, nil, spinnerErr
	//}
	//
	//if err != nil {
	//	return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to list workloads: %w", err)
	//}
	//
	//columns := []string{
	//	"Name",
	//	"Status",
	//	"Kaiwo user",
	//}
	//
	//var data [][]string
	//
	//dataMap := map[string]workloads.Workload{}
	//
	//for _, workloadReference := range workloadReferences {
	//	data = append(data, []string{
	//		workloadReference.GetName(),
	//		workloadReference.GetStatus(),
	//		workloadReference.GetKaiwoUser(),
	//	})
	//	dataMap[workloadReference.GetName()] = workloadReference
	//}
	//
	//if len(data) == 0 {
	//	logrus.Warnf("No %s workloads found", state.WorkloadType)
	//	return tuicomponents.StepResultPrevious, nil, nil
	//}
	//
	//title := fmt.Sprintf("Select the %s workload", state.WorkloadType)
	//selectedRow, result, err := tuicomponents.RunSelectTable(data, columns, title, true)
	//if err != nil {
	//	return result, nil, fmt.Errorf("failed to select the workload: %w", err)
	//}
	//if result == tuicomponents.StepResultOk {
	//	state.Workload = workloadReferences[selectedRow]
	//} else {
	//	state.Workload = nil
	//}
	//
	//return result, runSelectWorkloadAction, nil
}

func runSelectWorkloadAction(_ context.Context, _ k8s.KubernetesClients, _ *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	type workloadAction string
	var (
		viewPods       workloadAction = "View pods"
		portForward    workloadAction = "Port-forward"
		deleteWorkload workloadAction = "Delete workload"
	)

	data := [][]string{
		{string(viewPods)},
		{string(portForward)},
		{string(deleteWorkload)},
	}
	title := "Select the action you wish to take on the workload"
	selectedRow, result, err := tuicomponents.RunSelectTable(data, []string{"Action"}, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the action: %w", err)
	}

	actionMap := map[workloadAction]tuicomponents.RunStep[tuicomponents.RunState]{
		viewPods:       podlist.RunSelectPodAndContainer,
		portForward:    runPortForward,
		deleteWorkload: runDeleteWorkload,
	}

	if result == tuicomponents.StepResultOk {
		selectedAction := workloadAction(data[selectedRow][0])
		return result, actionMap[selectedAction], nil
	}

	return result, nil, nil
}
