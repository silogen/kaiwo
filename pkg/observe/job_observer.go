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

	"k8s.io/apimachinery/pkg/api/meta"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/platform/kueue"
)

// JobObserver observes Kubernetes Job status
type JobObserver struct {
	Identified
	CheckKueueAdmission bool
}

func NewJobObserver(nn types.NamespacedName, group UnitGroup, checkKueueAdmission bool) *JobObserver {
	return &JobObserver{
		Identified: Identified{
			NamespacedName: nn,
			Group:          group,
		},
		CheckKueueAdmission: checkKueueAdmission,
	}
}

func (o *JobObserver) Kind() string {
	return "Job"
}

// Observe keeps the same external behavior but has lower cyclomatic complexity.
func (o *JobObserver) Observe(ctx context.Context, c client.Client) (UnitStatus, error) {
	j, early := o.getJob(ctx, c)
	if early != nil {
		return *early, nil
	}

	// Kueue admission (only for workload jobs)
	if o.CheckKueueAdmission {
		if st := o.checkKueueAdmission(ctx, c, j); st != nil {
			return *st, nil
		}
	}

	conditions := convertJobConditions(j.Status.Conditions)

	// 1) Terminal conditions
	if st := statusFromTerminal(conditions); st != nil {
		return *st, nil
	}

	// 2) Non-terminal condition-driven states (success policy, suspension)
	if st := statusFromNonTerminal(j, conditions); st != nil {
		return *st, nil
	}

	// 3) Field-based fallbacks
	return statusFromFields(j, conditions), nil
}

// --- helpers ---

func (o *JobObserver) getJob(ctx context.Context, c client.Client) (*batchv1.Job, *UnitStatus) {
	var j batchv1.Job
	if err := c.Get(ctx, o.GetNamespacedName(), &j); err != nil {
		switch {
		case errors.IsNotFound(err):
			return nil, &UnitStatus{Phase: UnitPending}
		default:
			return nil, &UnitStatus{Phase: UnitUnknown, Reason: ReasonGetError, Message: err.Error()}
		}
	}
	return &j, nil
}

func (o *JobObserver) checkKueueAdmission(ctx context.Context, c client.Client, j *batchv1.Job) *UnitStatus {
	if wl, _ := kueue.GetKueueWorkload(ctx, c, j.GetNamespace(), string(j.GetUID())); wl != nil {
		if wl.Spec.Active != nil && !*wl.Spec.Active {
			return &UnitStatus{
				Phase:   UnitStopped,
				Reason:  "Reclaimed",
				Message: "Evicted by reclaimer (spec.active=false)",
			}
		}
	}

	admitted, err := CheckKueueAdmission(ctx, c, j.GetNamespace(), string(j.GetUID()))
	if err != nil {
		return &UnitStatus{Phase: UnitUnknown, Reason: ReasonAdmissionCheckError, Message: err.Error()}
	}
	if !admitted {
		return &UnitStatus{Phase: UnitPendingAdmission, Reason: ReasonPendingKueueAdmission}
	}
	return nil
}

func statusFromTerminal(conds []metav1.Condition) *UnitStatus {
	// Failed → terminal
	if meta.IsStatusConditionTrue(conds, string(batchv1.JobFailed)) {
		cnd := meta.FindStatusCondition(conds, string(batchv1.JobFailed))
		return &UnitStatus{
			Phase:      UnitFailed,
			Reason:     cnd.Reason,
			Message:    cnd.Message,
			Conditions: conds,
		}
	}
	// Complete → terminal
	if meta.IsStatusConditionTrue(conds, string(batchv1.JobComplete)) {
		cnd := meta.FindStatusCondition(conds, string(batchv1.JobComplete))
		return &UnitStatus{
			Phase:      UnitSucceeded,
			Ready:      true, // Ready == Succeeded for Jobs
			Reason:     cnd.Reason,
			Message:    cnd.Message,
			Conditions: conds,
		}
	}
	return nil
}

func statusFromNonTerminal(j *batchv1.Job, conds []metav1.Condition) *UnitStatus {
	// Success policy met; controller draining remaining pods
	if meta.IsStatusConditionTrue(conds, string(batchv1.JobSuccessCriteriaMet)) {
		cnd := meta.FindStatusCondition(conds, string(batchv1.JobSuccessCriteriaMet))
		return &UnitStatus{
			Phase:      UnitProgressing,
			Reason:     "SuccessPolicySatisfied",
			Message:    cnd.Message,
			Conditions: conds,
		}
	}

	// Suspension: make precedence explicit
	suspendSpec := ptr.Deref(j.Spec.Suspend, false)
	suspendCond := meta.IsStatusConditionTrue(conds, string(batchv1.JobSuspended))
	if suspendSpec || suspendCond {
		if jobStarted(j) {
			return &UnitStatus{Phase: UnitSuspended, Reason: "Paused"}
		}
		return &UnitStatus{
			Phase:      UnitPending,
			Reason:     "Suspended",
			Conditions: conds,
		}
	}
	return nil
}

func statusFromFields(j *batchv1.Job, conds []metav1.Condition) UnitStatus {
	ready := int32(0)
	if j.Status.Ready != nil {
		ready = *j.Status.Ready
	}

	switch {
	case j.Status.Active > 0 && ready > 0:
		return UnitStatus{Phase: UnitReady, Reason: "Running", Conditions: conds}
	case j.Status.Active > 0:
		return UnitStatus{Phase: UnitProgressing, Reason: "PodsLaunchingOrDraining", Conditions: conds}
	case j.Status.Succeeded > 0 && j.Status.Active == 0:
		return UnitStatus{Phase: UnitSucceeded, Reason: "SucceededPods", Conditions: conds}
	default:
		if j.Spec.BackoffLimit != nil && j.Status.Active == 0 && j.Status.Failed > *j.Spec.BackoffLimit {
			return UnitStatus{Phase: UnitFailed, Reason: "BackoffLimitExceeded(heuristic)", Conditions: conds}
		}
		return UnitStatus{Phase: UnitProgressing, Reason: "WaitingForPods", Conditions: conds}
	}
}

func jobStarted(j *batchv1.Job) bool {
	return j.Status.StartTime != nil ||
		j.Status.Active > 0 ||
		j.Status.Succeeded > 0 ||
		j.Status.Failed > 0
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
