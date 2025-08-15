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

package observe

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// JobObserver observes Kubernetes Job status
type JobObserver struct {
	NamespacedName types.NamespacedName
	Group          UnitGroup
}

func NewJobObserver(nn types.NamespacedName, group UnitGroup) *JobObserver {
	return &JobObserver{
		NamespacedName: nn,
		Group:          group,
	}
}

func (o *JobObserver) Kind() string {
	return "Job"
}

func (o *JobObserver) Observe(ctx context.Context, c client.Client) (UnitStatus, error) {
	var j batchv1.Job
	if err := c.Get(ctx, o.NamespacedName, &j); errors.IsNotFound(err) {
		return UnitStatus{
			Phase: UnitPending,
		}, nil
	} else if err != nil {
		return UnitStatus{
			Phase:   UnitUnknown,
			Reason:  ReasonGetError,
			Message: err.Error(),
		}, nil
	}

	conds := j.Status.Conditions
	get := func(t batchv1.JobConditionType) *batchv1.JobCondition {
		for i := range conds {
			if conds[i].Type == t {
				return &conds[i]
			}
		}
		return nil
	}

	// 1) Terminal first: Failed, then Complete
	if cnd := get(batchv1.JobFailed); cnd != nil && cnd.Status == corev1.ConditionTrue {
		return UnitStatus{
			Phase:      UnitFailed,
			Reason:     cnd.Reason,
			Message:    cnd.Message,
			Conditions: convertJobConditions(conds),
		}, nil
	}
	if cnd := get(batchv1.JobComplete); cnd != nil && cnd.Status == corev1.ConditionTrue {
		return UnitStatus{
			Phase:      UnitSucceeded,
			Ready:      true, // in your model, Ready==Succeeded for Jobs
			Reason:     cnd.Reason,
			Message:    cnd.Message,
			Conditions: convertJobConditions(conds),
		}, nil
	}

	// 2) Non-terminal conditions
	if cnd := get(batchv1.JobSuccessCriteriaMet); cnd != nil && cnd.Status == corev1.ConditionTrue {
		// Success policy met; controller is draining remaining pods → not terminal yet
		return UnitStatus{
			Phase:      UnitProgressing,
			Reason:     "SuccessPolicySatisfied",
			Message:    cnd.Message,
			Conditions: convertJobConditions(conds),
		}, nil
	}
	if cnd := get(batchv1.JobSuspended); cnd != nil && cnd.Status == corev1.ConditionTrue || ptr.Deref(j.Spec.Suspend, false) {
		return UnitStatus{
			Phase:      UnitPending,
			Reason:     "Suspended",
			Message:    "",
			Conditions: convertJobConditions(conds),
		}, nil
	}

	// Catch job suspension after start
	started := j.Status.StartTime != nil || j.Status.Active > 0 || j.Status.Succeeded > 0 || j.Status.Failed > 0
	if ptr.Deref(j.Spec.Suspend, false) && started {
		return UnitStatus{
			Phase:  UnitSuspended,
			Reason: "Paused",
		}, nil
	}

	// 3) Field-based fallbacks (cover brief windows before conditions update)
	ready := int32(0)
	if j.Status.Ready != nil {
		ready = *j.Status.Ready
	}
	switch {
	case j.Status.Active > 0 && ready > 0:
		return UnitStatus{
			Phase:      UnitProgressing,
			Reason:     "Running",
			Conditions: convertJobConditions(conds),
		}, nil
	case j.Status.Active > 0:
		// Pods starting or being drained after success-policy; not terminal
		return UnitStatus{
			Phase:      UnitProgressing,
			Reason:     "PodsLaunchingOrDraining",
			Conditions: convertJobConditions(conds),
		}, nil
	case j.Status.Succeeded > 0 && j.Status.Active == 0:
		// Fallback success if the Complete condition hasn't been set yet
		return UnitStatus{
			Phase:      UnitSucceeded,
			Reason:     "SucceededPods",
			Conditions: convertJobConditions(conds),
		}, nil
	default:
		// Optional heuristic: if backoff limit exceeded & no active pods, likely failing.
		if j.Spec.BackoffLimit != nil && j.Status.Active == 0 && j.Status.Failed >= *j.Spec.BackoffLimit {
			return UnitStatus{
				Phase:      UnitFailed, // heuristic; prefer condition when available
				Reason:     "BackoffLimitExceeded(heuristic)",
				Conditions: convertJobConditions(conds),
			}, nil
		}
		// Nothing running yet
		return UnitStatus{
			Phase:      UnitProgressing,
			Reason:     "WaitingForPods",
			Conditions: convertJobConditions(conds),
		}, nil
	}
}

func convertJobConditions(conditions []batchv1.JobCondition) []metav1.Condition {
	var result []metav1.Condition
	for _, c := range conditions {
		result = append(result, metav1.Condition{
			Type:               string(c.Type),
			Status:             metav1.ConditionStatus(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			LastTransitionTime: c.LastTransitionTime,
		})
	}
	return result
}
