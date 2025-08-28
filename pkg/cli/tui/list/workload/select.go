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

	tuicomponents2 "github.com/silogen/kaiwo/pkg/cli/tui/components"
	podlist "github.com/silogen/kaiwo/pkg/cli/tui/list/pod"

	"github.com/silogen/kaiwo/pkg/kube/utils"

	"github.com/silogen/kaiwo/pkg/runtime/common"

	"github.com/silogen/kaiwo/pkg/api"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/charmbracelet/huh/spinner"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RunSelectWorkloadType(_ context.Context, _ utils.KubernetesClients, state *tuicomponents2.RunState) (tuicomponents2.StepResult, tuicomponents2.RunStep[tuicomponents2.RunState], error) {
	data := [][]string{
		{"job"},
		{"service"},
	}
	title := "Select the resource type"
	selectedRow, result, err := tuicomponents2.RunSelectTable(data, []string{"Workload type"}, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the resource: %w", err)
	}
	if result == tuicomponents2.StepResultOk {
		state.WorkloadType = data[selectedRow][0]
	} else {
		state.WorkloadType = ""
	}
	return result, runSelectWorkload, nil
}

func runSelectWorkload(ctx context.Context, clients utils.KubernetesClients, state *tuicomponents2.RunState) (tuicomponents2.StepResult, tuicomponents2.RunStep[tuicomponents2.RunState], error) {
	labelSelector := client.MatchingLabels{}

	if state.User != "" {
		labelSelector[common.KaiwoUsernameLabel] = state.User
	}

	var err error

	var workloadReferences []api.KaiwoWorkload

	var loadErr error
	loadReferences := func() {
		switch state.WorkloadType {
		case "job":
			jobList := &kaiwo.KaiwoJobList{}
			if err := clients.Client.List(ctx, jobList, labelSelector); err != nil {
				loadErr = fmt.Errorf("failed to list Kaiwo jobs: %s", err)
				return
			}
			for _, job := range jobList.Items {
				workloadReferences = append(workloadReferences, &job)
			}
		case "service":
			serviceList := &kaiwo.KaiwoServiceList{}
			if err := clients.Client.List(ctx, serviceList, labelSelector); err != nil {
				loadErr = fmt.Errorf("failed to list Kaiwo services: %s", err)
				return
			}
			for _, service := range serviceList.Items {
				workloadReferences = append(workloadReferences, &service)
			}
		}
	}

	if spinnerErr := spinner.New().Title("Listing workloads").Action(loadReferences).Run(); spinnerErr != nil {
		return tuicomponents2.StepResultErr, nil, spinnerErr
	}
	if loadErr != nil {
		return tuicomponents2.StepResultErr, nil, fmt.Errorf("failed to fetch workloads: %w", loadErr)
	}

	columns := []string{
		"Name",
		"WorkloadStatus",
		"Kaiwo user",
	}

	var data [][]string

	for _, workloadReference := range workloadReferences {
		obj := workloadReference.GetKaiwoWorkloadObject()
		data = append(data, []string{
			obj.GetName(),
			string(workloadReference.GetCommonStatusSpec().Status),
			workloadReference.GetCommonSpec().User,
		})
	}

	if len(data) == 0 {
		logrus.Warnf("No %s workloads found", state.WorkloadType)
		return tuicomponents2.StepResultPrevious, nil, nil
	}

	title := fmt.Sprintf("Select the %s workload", state.WorkloadType)
	selectedRow, result, err := tuicomponents2.RunSelectTable(data, columns, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the workload: %w", err)
	}
	if result == tuicomponents2.StepResultOk {
		state.Workload = workloadReferences[selectedRow]
	} else {
		state.Workload = nil
	}

	return result, runSelectWorkloadAction, nil
}

func runSelectWorkloadAction(_ context.Context, _ utils.KubernetesClients, _ *tuicomponents2.RunState) (tuicomponents2.StepResult, tuicomponents2.RunStep[tuicomponents2.RunState], error) {
	type workloadAction string
	var viewPods workloadAction = "View pods"
	var portForward workloadAction = "Port-forward"
	var deleteWorkload workloadAction = "Delete workload"

	data := [][]string{
		{string(viewPods)},
		{string(portForward)},
		{string(deleteWorkload)},
	}
	title := "Select the action you wish to take on the workload"
	selectedRow, result, err := tuicomponents2.RunSelectTable(data, []string{"Action"}, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the action: %w", err)
	}

	actionMap := map[workloadAction]tuicomponents2.RunStep[tuicomponents2.RunState]{
		viewPods:       podlist.RunSelectPodAndContainer,
		portForward:    runPortForward,
		deleteWorkload: runDeleteWorkload,
	}

	if result == tuicomponents2.StepResultOk {
		selectedAction := workloadAction(data[selectedRow][0])
		return result, actionMap[selectedAction], nil
	}

	return result, nil, nil
}
