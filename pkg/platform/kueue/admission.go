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

package kueue

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/api"

	"k8s.io/apimachinery/pkg/api/errors"
	metautil "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

func GetKueueWorkload(ctx context.Context, k8sClient client.Client, namespace string, uid string) (*kueuev1beta1.Workload, error) {
	workloadList := &kueuev1beta1.WorkloadList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{"kueue.x-k8s.io/job-uid": uid}),
	}
	if err := k8sClient.List(ctx, workloadList, listOptions...); err != nil {
		return nil, fmt.Errorf("failed to list kueue workloads: %w", err)
	}
	if len(workloadList.Items) == 0 {
		return nil, nil
	} else if len(workloadList.Items) > 1 {
		return nil, fmt.Errorf("expected a single workload for job '%s/%s', but found %d workloads", namespace, uid, len(workloadList.Items))
	}
	return &workloadList.Items[0], nil
}

// IsAdmitted checks if a workload is fully admitted by Kueue
func IsAdmitted(ctx context.Context, k8sClient client.Client, workload api.WorkloadReconciler) (bool, error) {
	workloads, err := workload.GetKueueWorkloads(ctx, k8sClient)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get kueue workloads: %w", err)
	}
	if len(workloads) == 0 {
		return false, nil
	}
	for _, w := range workloads {
		admittedCondition := metautil.FindStatusCondition(w.Status.Conditions, kueuev1beta1.WorkloadAdmitted)
		if admittedCondition == nil || admittedCondition.Status == metav1.ConditionFalse {
			return false, nil
		}
	}
	return true, nil
}
