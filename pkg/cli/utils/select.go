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

package cliutils

import (
	"context"
	"fmt"
	"strings"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func GetWorkload(ctx context.Context, k8sClient client.Client, workloadSelector string, namespace string) (common.KaiwoWorkload, error) {
	parts := strings.Split(workloadSelector, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid workload selector, must be type/name: %s", workloadSelector)
	}
	type_ := parts[0]
	name := parts[1]
	var obj client.Object
	switch type_ {
	case "job":
		obj = &kaiwo.KaiwoJob{}
	case "service":
		obj = &kaiwo.KaiwoService{}
	default:
		return nil, fmt.Errorf("invalid workload type: %s, must be either 'job' or 'service'", type_)
	}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, fmt.Errorf("error getting Kaiwo %s %s/%s: %v", type_, namespace, name, err)
	}
	return obj.(common.KaiwoWorkload), nil
}
