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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
func ChoosePodAndContainer(ctx context.Context, k8sClient client.Client, reference workloads.WorkloadReference, predicates ...PodSelectionPredicate) (string, string, error, bool) {

	state := &runState{
		workloadReference:      reference,
		podSelectionPredicates: predicates,
	}

	result, err := runSelectPodAndContainer(
		ctx,
		k8sClient,
		state,
	)

	return state.podName, state.containerName, err, result == tui.SelectTableQuit || result == tui.SelectTableGoToPrevious
}
