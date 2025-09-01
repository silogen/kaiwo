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
	"time"

	"github.com/silogen/kaiwo/pkg/api"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

type PreemptReason string

const (
	PreemptableConditionType         string        = "Preemptable"
	PreemptReasonDurationNotExceeded PreemptReason = "DurationNotExceeded"
	PreemptReasonDurationExceeded    PreemptReason = "DurationExceeded"
)

func GetRemainingTimeBeforeBecomingPreemptable(handler api.KaiwoWorkload) *time.Duration {
	status := handler.GetCommonStatusSpec()
	spec := handler.GetCommonSpec()
	if status.Status != v1alpha1.WorkloadStatusRunning || spec.Duration == nil || status.StartTime == nil {
		return nil
	}
	elapsed := time.Since(status.StartTime.Time)
	remaining := spec.Duration.Duration - elapsed
	return &remaining
}

// GetPreemptableCondition gets the correct preemptable condition
func GetPreemptableCondition(_ context.Context, handler api.KaiwoWorkload) *metav1.Condition {
	remaining := GetRemainingTimeBeforeBecomingPreemptable(handler)
	obj := handler.GetKaiwoWorkloadObject()
	if remaining == nil {
		return nil
	}
	if remaining.Seconds() > 0 {
		return &metav1.Condition{
			Type:               PreemptableConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             string(PreemptReasonDurationNotExceeded),
			Message:            "Workload is running and the duration has not been exceeded",
			ObservedGeneration: obj.GetGeneration(),
		}
	} else {
		return &metav1.Condition{
			Type:               PreemptableConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             string(PreemptReasonDurationExceeded),
			Message:            "Workload duration has exceeded",
			ObservedGeneration: obj.GetGeneration(),
		}
	}
}

func ShouldRequeueAfter(workload api.KaiwoWorkload) (bool, *time.Duration) {
	condition := meta.FindStatusCondition(workload.GetCommonStatusSpec().Conditions, PreemptableConditionType)
	if condition != nil &&
		condition.Status == metav1.ConditionFalse &&
		condition.ObservedGeneration == workload.GetKaiwoWorkloadObject().GetGeneration() {
		requeueAfter := GetRemainingTimeBeforeBecomingPreemptable(workload)
		if requeueAfter != nil {
			return true, requeueAfter
		}
	}
	return false, nil
}
