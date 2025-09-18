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
	"time"

	"k8s.io/apimachinery/pkg/types"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metautil "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

const WorkloadEarlyTerminationConditionType = "WorkloadTerminatedEarly"

type WorkloadTerminationReason string

// TerminateWorkload terminates a given workload by deleting all the child objects and setting
// an early termination condition and emitting an event
func TerminateWorkload(
	ctx context.Context,
	k8sClient client.Client,
	recorder record.EventRecorder,
	handler WorkloadReconciler,
) error {
	logger := log.FromContext(ctx)
	obj := handler.GetKaiwoWorkloadObject()
	workload := obj.(KaiwoWorkload)
	var condition *metav1.Condition

	statusSpec := workload.GetCommonStatusSpec()
	statusSpec.Status = kaiwo.WorkloadStatusTerminated

	condition = metautil.FindStatusCondition(statusSpec.Conditions, WorkloadEarlyTerminationConditionType)
	if condition == nil {
		condition = &metav1.Condition{
			Type:    WorkloadEarlyTerminationConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "EarlyTermination",
			Message: "Workload terminated early (check events and other conditions for likely causes)",
		}
		metautil.SetStatusCondition(&statusSpec.Conditions, *condition)
	} else {
		// Conditional already exists (created when setting the TERMINATING status), just update to true
		condition.Status = metav1.ConditionTrue
		metautil.SetStatusCondition(&statusSpec.Conditions, *condition)
	}

	// Update status first to avoid doing any further reconciliation
	if err := k8sClient.Status().Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	logger.Info("Terminating workload", "reason", condition.Reason, "message", condition.Message)

	if err := DeleteUnderlyingResources(ctx, obj.GetUID(), obj.GetName(), obj.GetNamespace(), k8sClient); err != nil {
		return fmt.Errorf("failed to delete workload resources: %w", err)
	}

	recorder.Event(obj, corev1.EventTypeWarning, condition.Reason, condition.Message)

	return nil
}

// CheckKaiwoWorkloadShouldBeTerminatedForUnderutilization checks if the Kaiwo workload should be terminated due to resource underutilization
func CheckKaiwoWorkloadShouldBeTerminatedForUnderutilization(ctx context.Context, workload KaiwoWorkload) (bool, string) {
	config := ConfigFromContext(ctx)
	logger := log.FromContext(ctx)

	if !config.ResourceMonitoring.TerminateUnderutilized {
		return false, ""
	}

	condition := metautil.FindStatusCondition(workload.GetCommonStatusSpec().Conditions, KaiwoResourceUtilizationType)
	if condition == nil || condition.Status == metav1.ConditionFalse {
		return false, ""
	}

	terminateAfter, err := time.ParseDuration(config.ResourceMonitoring.TerminateUnderutilizedAfter)
	if err != nil {
		logger.Error(err, "Failed to parse duration", "duration", config.ResourceMonitoring.TerminateUnderutilizedAfter)
		return false, ""
	}

	if time.Since(condition.LastTransitionTime.Time) > terminateAfter {
		return true, condition.Reason
	}

	return false, ""
}

// DeleteUnderlyingResources deletes all the underlying resources that a workload owns
func DeleteUnderlyingResources(ctx context.Context, uid types.UID, name string, namespace string, k8sClient client.Client) error {
	resourceTypes := []client.ObjectList{
		&appsv1.DeploymentList{},
		&rayv1.RayServiceList{},
		&batchv1.JobList{},
		&rayv1.RayJobList{},
		&corev1.PersistentVolumeClaimList{},
	}

	logger := log.FromContext(ctx)

	var deletedAny bool

	for _, list := range resourceTypes {
		if err := k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
		}

		items, err := metautil.ExtractList(list)
		if err != nil {
			return fmt.Errorf("failed to extract items: %w", err)
		}

		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			for _, owner := range obj.GetOwnerReferences() {
				if owner.UID == uid && owner.Controller != nil && *owner.Controller {
					foreground := metav1.DeletePropagationForeground
					if err := k8sClient.Delete(ctx, obj, &client.DeleteOptions{
						PropagationPolicy: &foreground,
					}); err != nil {
						return fmt.Errorf("failed to delete resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
					}
					logger.Info("Deleted resource", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
					deletedAny = true
				}
			}
		}
	}

	if !deletedAny {
		return fmt.Errorf("no owned resources found to delete for %s", name)
	}
	return nil
}

// SetEarlyTermination flags a workload for early termination by
// 1. Setting the status to TERMINATING
// 2. Creating the WorkloadTerminatedEarly condition, but keeping its status as False (in order to record the reason)
func SetEarlyTermination(ctx context.Context, workload KaiwoWorkload, reason string, message string) {
	logger := log.FromContext(ctx)

	logger.Info("Flagging workload for early termination", "reason", reason, "message", message)

	statusSpec := workload.GetCommonStatusSpec()
	statusSpec.Status = kaiwo.WorkloadStatusTerminating

	metautil.SetStatusCondition(&statusSpec.Conditions, metav1.Condition{
		Type:    WorkloadEarlyTerminationConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}
