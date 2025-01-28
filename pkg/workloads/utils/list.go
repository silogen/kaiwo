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

package utils

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/silogen/kaiwo/pkg/tui"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/silogen/kaiwo/pkg/workloads/factory"
)

var (
	runFuncs = []runFunc{
		runSelectWorkloadType,
		runSelectWorkload,
		runSelectPodAndContainer,
		runSelectAndDoAction,
	}
)

func RunList(workloadType string, workloadName string, namespace string, user string) error {

	if workloadType == "" && workloadName != "" {
		return fmt.Errorf("cannot determine workload from name without a type")
	}

	k8sClient, err := k8s.GetClient()
	if err != nil {
		return err
	}

	// TODO fix
	kubeconfig := "/home/alex/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build config from flags: %w", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to build config from flags: %w", err)
	}

	_ = clientset
	_ = k8sClient

	ctx := context.Background()

	var workloadReference workloads.WorkloadReference

	if workloadName != "" {
		// If the workload name is set, attempt to load the workload reference
		workload, objectKey, err := factory.GetWorkloadAndObjectKey(fmt.Sprintf("%s/%s", workloadType, workloadName), namespace)
		if err != nil {
			return fmt.Errorf("failed to get workload: %w", err)
		}
		workloadReference, err = workload.BuildReference(ctx, k8sClient, objectKey)
		if err != nil {
			return fmt.Errorf("failed to build workload reference: %w", err)
		}
	}

	screenIndex := 0

	runState := &runState{
		workloadType:      workloadType,
		workloadReference: workloadReference,
		user:              user,
		namespace:         namespace,
	}

	if workloadType != "" {
		screenIndex = 1
	}
	if workloadName != "" {
		screenIndex = 2
	}

	for {
		runFunc := runFuncs[screenIndex]

		result, err := runFunc(ctx, k8sClient, runState)

		if err != nil {
			return fmt.Errorf("failed select: %w", err)
		}

		if result == tui.SelectTableGoToPrevious {
			if screenIndex == 0 {
				// Already at the beginning, do nothing
				continue
			}
			screenIndex -= 1
		} else if result == tui.SelectTableQuit {
			// Quit
			return nil
		} else if result == tui.SelectTableRowSelected {
			if screenIndex < len(runFuncs)-1 {
				// Still have more screens left
				screenIndex += 1
			} else {
				// Final screen finished, quit
				return nil
			}
		}
	}
}

type runState struct {
	workloadType           string
	workloadReference      workloads.WorkloadReference
	user                   string
	namespace              string
	podName                string
	containerName          string
	podSelectionPredicates []PodSelectionPredicate
}

type runFunc func(context.Context, client.Client, *runState) (tui.SelectTableResult, error)

func runSelectWorkloadType(_ context.Context, _ client.Client, runState *runState) (tui.SelectTableResult, error) {
	data := [][]string{
		{"job"},
		{"deployment"},
		{"rayjob"},
		{"rayservice"},
	}
	title := "Select the resource type"
	selectedRow, result, err := tui.RunSelectTable(data, []string{"Workload type"}, title, true)
	if err != nil {
		return result, fmt.Errorf("failed to select the resource: %w", err)
	}
	if result == tui.SelectTableRowSelected {
		runState.workloadType = (*selectedRow)[0]
	} else {
		runState.workloadType = ""
	}
	return result, nil
}

func runSelectWorkload(ctx context.Context, k8sClient client.Client, runState *runState) (tui.SelectTableResult, error) {
	labelSelector := client.MatchingLabels{}

	if runState.user != "" {
		labelSelector[workloads.KaiwoUsernameLabel] = runState.user
	}

	var err error

	var workloadReferences []workloads.WorkloadReference

	loadReferences := func() {
		workloadReferences, err = factory.ListObjects(ctx, k8sClient, runState.workloadType, labelSelector, client.InNamespace(runState.namespace))
	}

	if spinnerErr := spinner.New().Title("Listing workloads").Action(loadReferences).Run(); spinnerErr != nil {
		return tui.SelectTableError, spinnerErr
	}

	if err != nil {
		return tui.SelectTableError, fmt.Errorf("failed to list workloads: %w", err)
	}

	columns := []string{
		"Name",
		"Status",
		"Kaiwo user",
	}

	var data [][]string

	dataMap := map[string]workloads.WorkloadReference{}

	for _, workloadReference := range workloadReferences {
		data = append(data, []string{
			workloadReference.GetName(),
			workloadReference.GetStatus(),
			workloadReference.GetKaiwoUser(),
		})
		dataMap[workloadReference.GetName()] = workloadReference
	}

	if len(data) == 0 {
		logrus.Warnf("No %s workloads found", runState.workloadType)
		return tui.SelectTableGoToPrevious, nil
	}

	title := fmt.Sprintf("Select the %s workload", runState.workloadType)
	selectedRow, result, err := tui.RunSelectTable(data, columns, title, true)

	if err != nil {
		return result, fmt.Errorf("failed to select the workload: %w", err)
	}
	if result == tui.SelectTableRowSelected {
		runState.workloadReference = dataMap[(*selectedRow)[0]]
	} else {
		runState.workloadReference = nil
	}

	return result, nil
}

func runSelectPodAndContainer(ctx context.Context, k8sClient client.Client, runState *runState) (tui.SelectTableResult, error) {
	var err error

	loadReference := func() {
		err = runState.workloadReference.Load(ctx, k8sClient)
	}

	if spinnerErr := spinner.New().Title("Loading workload").Action(loadReference).Run(); spinnerErr != nil {
		return tui.SelectTableError, spinnerErr
	}

	if err != nil {
		return tui.SelectTableError, fmt.Errorf("failed to load workload: %w", err)
	}

	allPods := runState.workloadReference.GetPods()

	var data [][]string

	containerStatusToRow := func(pod workloads.WorkloadPod, containerStatus corev1.ContainerStatus, isInitContainer bool) []string {
		containerStatusMsg := ""

		if containerStatus.State.Running != nil {
			containerStatusMsg = fmt.Sprintf("Running since %s", containerStatus.State.Running.StartedAt.Format(time.RFC3339))
		} else if containerStatus.State.Waiting != nil {
			containerStatusMsg = fmt.Sprintf("Waiting (%s)", containerStatus.State.Waiting.Reason)
		} else if containerStatus.State.Terminated != nil {
			containerStatusMsg = fmt.Sprintf("Terminated (%s)", containerStatus.State.Terminated.Reason)
		} else {
			containerStatusMsg = "N/A"
		}

		// TODO add indicator about being an init container (another column?)
		//prefix := ""
		//if isInitContainer {
		//	prefix = "[init] "
		//}

		return []string{
			pod.LogicalGroup,
			pod.Pod.Name,
			string(pod.Pod.Status.Phase),
			containerStatus.Name,
			containerStatusMsg,
		}
	}

	for _, pod := range allPods {

		skip := false
		for _, predicate := range runState.podSelectionPredicates {
			if !predicate(pod.Pod) {
				skip = true
			}
		}
		if skip {
			continue
		}

		for _, container := range pod.Pod.Status.ContainerStatuses {
			data = append(data, containerStatusToRow(pod, container, false))
		}
		for _, container := range pod.Pod.Status.InitContainerStatuses {
			data = append(data, containerStatusToRow(pod, container, true))
		}
		logrus.Infof("Found pod %s (%s)", pod.Pod.Name, pod.LogicalGroup)
	}

	title := "Select pod and container"
	selectedRow, result, err := tui.RunSelectTable(data, containerSelectColumns, title, true)
	if err != nil {
		return result, fmt.Errorf("failed to select the pod: %w", err)
	}
	if result == tui.SelectTableRowSelected {
		runState.podName = (*selectedRow)[1]
		runState.containerName = (*selectedRow)[3]
	} else {
		runState.podName = ""
		runState.containerName = ""
	}
	return result, nil
}

type runAction string

var (
	viewLogsAction runAction = "View logs"
	monitorAction  runAction = "Monitor"
	commandAction  runAction = "Run command"
)

func runSelectAndDoAction(ctx context.Context, k8sClient client.Client, runState *runState) (tui.SelectTableResult, error) {
	columns := []string{
		"Action",
	}

	data := [][]string{
		{string(viewLogsAction)},
		{string(monitorAction)},
		{string(commandAction)},
	}

	title := fmt.Sprintf("Select action to perform on %s/%s, pod: %s, container %s", runState.workloadType, runState.workloadReference.GetName(), runState.podName, runState.containerName)
	selectedRow, result, err := tui.RunSelectTable(data, columns, title, true)

	if err != nil {
		return result, fmt.Errorf("failed to select the pod: %w", err)
	}

	if result == tui.SelectTableRowSelected {
		selectedAction := runAction((*selectedRow)[0])
		switch selectedAction {
		case viewLogsAction:
			return tui.SelectTableRowSelected, runViewLogsAction(ctx, k8sClient, runState)
		case monitorAction:
			return tui.SelectTableRowSelected, runMonitorAction(ctx, k8sClient, runState)
		case commandAction:
			return tui.SelectTableRowSelected, runCommandAction(ctx, k8sClient, runState)
		default:
			return tui.SelectTableError, fmt.Errorf("unknown action: %s", selectedAction)
		}
	}

	return tui.SelectTableRowSelected, nil
}

func runViewLogsAction(ctx context.Context, _ client.Client, runState *runState) error {
	follow := true
	lines := ""
	var numLines int
	showFormatMessage := false

	clientset, err := k8s.GetClientset()
	if err != nil {
		return fmt.Errorf("failed to get clientset: %w", err)
	}

	for {
		inputs := []huh.Field{
			huh.NewInput().Title("Tail lines (-1 shows all lines)").Value(&lines).Placeholder("-1"),
			huh.NewConfirm().Title("Follow").Value(&follow),
		}

		if showFormatMessage {
			inputs = append(inputs, huh.NewNote().Title("Lines must be a non-zero whole number"))
		}

		f := huh.NewForm(huh.NewGroup(inputs...))

		err := f.Run()
		if err != nil {
			return fmt.Errorf("failed to fetch input: %w", err)
		}

		if lines == "" {
			lines = "-1"
		}

		numLines, err = strconv.Atoi(lines)
		if err != nil || numLines == 0 {
			showFormatMessage = true
			continue
		}
		break
	}

	return OutputLogs(ctx, clientset, runState.podName, runState.containerName, int64(numLines), runState.workloadReference.GetNamespace(), follow)
}

func runMonitorAction(ctx context.Context, _ client.Client, runState *runState) error {
	kubeconfig, _ := k8s.GetKubeConfig()
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	command := ParseCommand(DefaultMonitorCommand)
	if err := ValidateCommand(command[0]); err != nil {
		return fmt.Errorf("failed to validate command: %w", err)
	}

	return ExecInContainer(
		ctx,
		clientset,
		config,
		runState.podName,
		runState.containerName,
		runState.workloadReference.GetNamespace(),
		command,
		true,
		true,
	)
}

func runCommandAction(ctx context.Context, _ client.Client, runState *runState) error {
	kubeconfig, _ := k8s.GetKubeConfig()
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

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
			return fmt.Errorf("failed to fetch input: %w", err)
		}

		if execCommand == "" {
			commandPlaceholder = "Please enter command to run"
			continue
		}

		command = ParseCommand(execCommand)
		if err := ValidateCommand(command[0]); err != nil {
			return fmt.Errorf("failed to validate command: %w", err)
		}

		break
	}

	return ExecInContainer(
		ctx,
		clientset,
		config,
		runState.podName,
		runState.containerName,
		runState.workloadReference.GetNamespace(),
		command,
		execInteractive,
		execTTY,
	)
}
