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
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/pkg/tui"
	"github.com/silogen/kaiwo/pkg/workloads"
)

var (
	containerSelectColumns = []string{
		"Logical group",
		"Pod name",
		"Pod phase",
		"Container name",
		"Container status",
	}
)

type PodSelectionPredicate func(pod corev1.Pod) bool

func IsGPUPod(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			if resourceName == "nvidia.com/gpu" || resourceName == "amd.com/gpu" {
				return true
			}
		}
	}
	return false
}

// ChoosePodAndContainer allows the user to choose the pod and the container they want to interact with
// predicates define an optional list of predicates that must be matched in order to include the pod in the list
func ChoosePodAndContainer(reference workloads.WorkloadReference, predicates ...PodSelectionPredicate) (string, string, error, bool) {

	allPods := reference.GetPods()

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

		// TODO add
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
		for _, predicate := range predicates {
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

	title := ""
	selectedRow, err := tui.RunSelectTable(data, containerSelectColumns, title, true)

	selectedContainerName := ""
	selectedPodName := ""

	if selectedRow != nil {
		selectedPodName = (*selectedRow)[1]
		selectedContainerName = (*selectedRow)[3]
	}

	return selectedPodName, selectedContainerName, err, selectedRow == nil && err != nil
}
