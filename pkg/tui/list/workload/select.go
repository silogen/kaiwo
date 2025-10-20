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

package workloadlist

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/charmbracelet/huh/spinner"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	podlist "github.com/silogen/kaiwo/pkg/tui/list/pod"
	"github.com/silogen/kaiwo/pkg/workloads"

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

func RunSelectWorkloadType(_ context.Context, _ k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	data := [][]string{
		{"job"},
		{"service"},
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
	labelSelector := client.MatchingLabels{}

	if state.User != "" {
		labelSelector[workloads.KaiwoUsernameLabel] = state.User
	}

	var err error

	var workloadReferences []common.KaiwoWorkload

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
		return tuicomponents.StepResultErr, nil, spinnerErr
	}
	if loadErr != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to fetch workloads: %w", loadErr)
	}

	columns := []string{
		"Description",
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
		return tuicomponents.StepResultPrevious, nil, nil
	}

	title := fmt.Sprintf("Select the %s workload", state.WorkloadType)
	selectedRow, result, err := tuicomponents.RunSelectTable(data, columns, title, true)
	if err != nil {
		return result, nil, fmt.Errorf("failed to select the workload: %w", err)
	}
	if result == tuicomponents.StepResultOk {
		state.Workload = workloadReferences[selectedRow]
	} else {
		state.Workload = nil
	}

	return result, runSelectWorkloadAction, nil
}

func runSelectWorkloadAction(_ context.Context, _ k8s.KubernetesClients, _ *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
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
