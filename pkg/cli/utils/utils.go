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

	"github.com/silogen/kaiwo/pkg/runtime/common"

	"github.com/silogen/kaiwo/pkg/api"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetWorkloadPods(ctx context.Context, k8sClient client.Client, workload api.KaiwoWorkload) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	obj := workload.GetKaiwoWorkloadObject()
	if err := k8sClient.List(ctx, podList, client.InNamespace(obj.GetNamespace()), client.MatchingLabels{
		common.KaiwoRunIdLabel: string(obj.GetUID()),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

func GetWorkloadServices(ctx context.Context, k8sClient client.Client, workload api.KaiwoWorkload) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	return serviceList.Items, nil
}
