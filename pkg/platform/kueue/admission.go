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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// GetKueueWorkload finds the Kueue Workload owned by the given controller UID (Job, RayJob, AppWrapper)
func GetKueueWorkload(ctx context.Context, k8sClient client.Client, namespace string, uid string) (*kueuev1beta1.Workload, error) {
	workloadList := &kueuev1beta1.WorkloadList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
	}
	if err := k8sClient.List(ctx, workloadList, listOptions...); err != nil {
		return nil, fmt.Errorf("failed to list kueue workloads: %w", err)
	}

	var matches []kueuev1beta1.Workload
	for _, wl := range workloadList.Items {
		for _, owner := range wl.OwnerReferences {
			if owner.UID == types.UID(uid) {
				matches = append(matches, wl)
				break
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("expected a single workload for '%s/%s', found %d", namespace, uid, len(matches))
	}
}

// IsAdmitted checks if a workload is fully admitted by Kueue
func IsAdmitted(ctx context.Context, k8sClient client.Client, workload api.WorkloadReconciler) (bool, error) {
	logger := log.FromContext(ctx).WithName("IsAdmitted")

	workloads, err := workload.GetKueueWorkloads(ctx, k8sClient)
	if err != nil {
		logger.Error(err, "Failed to get Kueue workloads")
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
