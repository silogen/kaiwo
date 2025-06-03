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

package common

import (
	"context"
	"fmt"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

// WorkloadHandler handles the workload-related reconciler(s)
type WorkloadHandler struct {
	Workload WorkloadReconciler
}

type WorkloadAdmittedReason string

const (
	WorkloadAdmittedType = "Admitted"

	WorkloadAdmitted WorkloadAdmittedReason = "Admitted"
	WorkloadPending  WorkloadAdmittedReason = "Pending"
)

func (h WorkloadHandler) GetResourceReconcilers() []ResourceReconciler {
	return []ResourceReconciler{h.Workload}
}

func (h WorkloadHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, previousWorkloadStatus v1alpha1.WorkloadStatus) (v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	isAdmitted, err := IsAdmitted(ctx, k8sClient, h.Workload)
	if err != nil {
		return "", nil, fmt.Errorf("failed to check if workload is admitted: %w", err)
	}

	var conditions []metav1.Condition

	workloadStatus, workloadConditions, err := ObserveOverallStatus(ctx, k8sClient, h.GetResourceReconcilers(), previousWorkloadStatus)
	if err != nil {
		return "", nil, fmt.Errorf("failed to observe workload status: %w", err)
	}

	if isAdmitted {

		if workloadStatus == nil || *workloadStatus == v1alpha1.WorkloadStatusNew {
			workloadStatus = baseutils.Pointer(v1alpha1.WorkloadStatusStarting)
		}

		conditions = append(conditions, metav1.Condition{
			Type:    WorkloadAdmittedType,
			Status:  metav1.ConditionTrue,
			Reason:  string(WorkloadAdmitted),
			Message: "Workload is admitted",
		})

		conditions = append(conditions, workloadConditions...)

		// Check for preemption TODO move?
		if h.Workload.GetCommonSpec().Duration != nil {
			preemptCondition := GetPreemptableCondition(h.Workload)
			if preemptCondition != nil {
				conditions = append(conditions, *preemptCondition)

				if shouldPreempt, err := ShouldPreempt(ctx, k8sClient, h.Workload); err != nil {
					return "", nil, fmt.Errorf("failed to check if workload is preemptable: %w", err)
				} else if shouldPreempt {
					conditions = append(conditions, metav1.Condition{
						Type:    WorkloadEarlyTerminationConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  string(PreemptReasonDurationExceededWithActiveGpuDemand),
						Message: "Terminated since duration was exceeded and there is active GPU demand",
					})
					return v1alpha1.WorkloadStatusTerminating, conditions, nil
				}
			}

		}
		return *workloadStatus, conditions, nil
	} else {
		queueName := GetClusterQueueName(ctx, h.Workload)

		conditions = append(conditions, metav1.Condition{
			Type:    WorkloadAdmittedType,
			Status:  metav1.ConditionFalse,
			Reason:  string(WorkloadPending),
			Message: fmt.Sprintf("Workload is pending admission into '%s'", queueName),
		})
		return v1alpha1.WorkloadStatusPending, conditions, nil
	}
}
