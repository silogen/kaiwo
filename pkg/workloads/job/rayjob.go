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

package workloads

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/silogen/kaiwo/pkg/platform/ray"

	"github.com/silogen/kaiwo/pkg/platform/kueue"

	"github.com/silogen/kaiwo/pkg/runtime/config"

	common2 "github.com/silogen/kaiwo/pkg/runtime/common"

	"github.com/silogen/kaiwo/pkg/api"

	"github.com/silogen/kaiwo/pkg/observe"

	"k8s.io/apimachinery/pkg/runtime"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetDefaultRayJobSpec(config config.KaiwoConfigContext, dangerous bool) rayv1.RayJobSpec {
	return rayv1.RayJobSpec{
		ShutdownAfterJobFinishes: true,
		RayClusterSpec:           ray.GetRayClusterTemplate(config, dangerous),
	}
}

type RayJobHandler struct {
	KaiwoJob *kaiwo.KaiwoJob
	Scheme   *runtime.Scheme
}

func (handler *RayJobHandler) GetInitializedObject() client.Object {
	return &rayv1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RayJob",
			APIVersion: rayv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoJob.Name,
			Namespace: handler.KaiwoJob.Namespace,
			Labels:    map[string]string{},
		},
	}
}

func (handler *RayJobHandler) GetKaiwoWorkloadObject() client.Object {
	return handler.KaiwoJob
}

func (handler *RayJobHandler) GetCommonSpec() kaiwo.CommonMetaSpec {
	return handler.KaiwoJob.Spec.CommonMetaSpec
}

func (handler *RayJobHandler) GetCommonStatusSpec() *kaiwo.CommonStatusSpec {
	return &handler.KaiwoJob.Status.CommonStatusSpec
}

func (handler *RayJobHandler) BuildDesired(ctx context.Context, clusterCtx api.ClusterContext) (client.Object, error) {
	logger := log.FromContext(ctx)
	cfg := config.ConfigFromContext(ctx)

	spec := handler.KaiwoJob.Spec

	var rayJobSpec rayv1.RayJobSpec

	if spec.RayJob == nil {
		rayJobSpec = GetDefaultRayJobSpec(cfg, spec.Dangerous)
	} else {
		rayJobSpec = spec.RayJob.Spec
	}

	// Ensure backoff limit is 0
	if baseutils.ValueOrDefault(rayJobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		rayJobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	// Convert entrypoint
	if spec.EntryPoint != "" {
		rayJobSpec.Entrypoint = baseutils.ConvertMultilineEntrypoint(spec.EntryPoint, true).(string)
	}

	// Update ray cluster specs
	if err := ray.UpdateRayClusterSpec(ctx, clusterCtx, handler.KaiwoJob, rayJobSpec.RayClusterSpec); err != nil {
		return nil, fmt.Errorf("failed to update ray cluster spec: %w", err)
	}

	rayJob := handler.GetInitializedObject().(*rayv1.RayJob)
	rayJob.Labels = rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels
	rayJob.Spec = rayJobSpec

	common2.UpdateLabels(handler.KaiwoJob, &rayJob.ObjectMeta)

	rayJob.Labels[common2.QueueLabel] = api.GetClusterQueueName(ctx, handler)
	if priorityClass := handler.GetCommonSpec().WorkloadPriorityClass; priorityClass != "" {
		rayJob.Labels[common2.WorkloaddPriorityClassLabel] = priorityClass
	}

	return rayJob, nil
}

func (handler *RayJobHandler) MutateActual(ctx context.Context, clusterCtx api.ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

func (handler *RayJobHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	job := obj.(*rayv1.RayJob)

	switch job.Status.JobStatus {
	case rayv1.JobStatusNew, rayv1.JobStatusPending:
		// Kaiwo status Starting means that the job has been admitted, and is starting up
		return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
	case rayv1.JobStatusRunning:
		return baseutils.Pointer(kaiwo.WorkloadStatusRunning), nil, nil
	case rayv1.JobStatusFailed:
		return baseutils.Pointer(kaiwo.WorkloadStatusFailed), nil, nil
	case rayv1.JobStatusSucceeded:
		return baseutils.Pointer(kaiwo.WorkloadStatusComplete), nil, nil
	case rayv1.JobStatusStopped:
		return baseutils.Pointer(kaiwo.WorkloadStatusTerminated), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected job status: %s", job.Status.JobStatus)
	}
}

func (handler *RayJobHandler) GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error) {
	rayJob := &rayv1.RayJob{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(handler.KaiwoJob), rayJob); err != nil {
		return nil, fmt.Errorf("failed to get rayJob: %w", err)
	}

	// Use the RayJob UID to match Kueue Workload ownerReference
	workload, err := kueue.GetKueueWorkload(ctx, k8sClient, rayJob.GetNamespace(), string(rayJob.GetUID()))
	if err != nil {
		return nil, fmt.Errorf("failed to extract workload from handler: %w", err)
	}
	if workload == nil {
		return []kueuev1beta1.Workload{}, nil
	}
	return []kueuev1beta1.Workload{*workload}, nil
}

// RayJobObserver observes RayJob status
type RayJobObserver struct {
	observe.Identified
}

func NewRayJobObserver(nn types.NamespacedName, group observe.UnitGroup) *RayJobObserver {
	return &RayJobObserver{
		observe.Identified{
			NamespacedName: nn,
			Group:          group,
		},
	}
}

func (o *RayJobObserver) Kind() string {
	return "RayJob"
}

func (o *RayJobObserver) Observe(ctx context.Context, c client.Client) (observe.UnitStatus, error) {
	var rj rayv1.RayJob
	if err := c.Get(ctx, o.GetNamespacedName(), &rj); errors.IsNotFound(err) {
		return observe.UnitStatus{Phase: observe.UnitPending}, nil
	} else if err != nil {
		return observe.UnitStatus{
			Phase:   observe.UnitUnknown,
			Reason:  observe.ReasonGetError,
			Message: err.Error(),
		}, nil
	}

	// Check Kueue admission - this blocks everything else
	// If the Kueue Workload exists but is inactive (spec.active=false), it was reclaimed
	if wl, _ := kueue.GetKueueWorkload(ctx, c, rj.GetNamespace(), string(rj.GetUID())); wl != nil {
		if wl.Spec.Active != nil && !*wl.Spec.Active {
			return observe.UnitStatus{
				Phase:   observe.UnitStopped,
				Reason:  "Reclaimed",
				Message: "Evicted by reclaimer (spec.active=false)",
			}, nil
		}
	}

	admitted, err := observe.CheckKueueAdmission(ctx, c, rj.GetNamespace(), string(rj.GetUID()))
	if err != nil {
		return observe.UnitStatus{
			Phase:   observe.UnitUnknown,
			Reason:  observe.ReasonAdmissionCheckError,
			Message: err.Error(),
		}, nil
	}
	if !admitted {
		return observe.UnitStatus{
			Phase:  observe.UnitPendingAdmission,
			Reason: observe.ReasonPendingKueueAdmission,
		}, nil
	}

	obsGen := rj.Status.ObservedGeneration

	// 0) Respect "suspend" (intentional pause)
	if rj.Spec.Suspend {
		// It's an intentional pause. If it had started before, mark PausedAfterStart; otherwise Paused.
		if rj.Status.JobStatus == rayv1.JobStatusRunning {
			return observe.UnitStatus{
				Phase:              observe.UnitSuspended,
				Reason:             "Paused",
				ObservedGeneration: obsGen,
			}, nil
		}
		return observe.UnitStatus{
			Phase:              observe.UnitSuspended,
			Reason:             "Suspended",
			ObservedGeneration: obsGen,
		}, nil
	}

	// 1) Terminal RayJob states first
	switch rj.Status.JobStatus {
	case rayv1.JobStatusSucceeded:
		return observe.UnitStatus{
			Phase:              observe.UnitSucceeded,
			Ready:              true,
			ObservedGeneration: obsGen,
		}, nil
	case rayv1.JobStatusFailed:
		msg := rj.Status.Message
		if msg == "" {
			msg = "RayJob failed"
		}
		return observe.UnitStatus{
			Phase:              observe.UnitFailed,
			Reason:             observe.ReasonJobFailed,
			Message:            msg,
			ObservedGeneration: obsGen,
		}, nil
	case rayv1.JobStatusStopped:
		msg := rj.Status.Message
		if msg == "" {
			msg = "RayJob was stopped"
		}
		return observe.UnitStatus{
			Phase:              observe.UnitStopped,
			Reason:             observe.ReasonJobStopped,
			Message:            msg,
			ObservedGeneration: obsGen,
		}, nil
	}

	// 2) Cluster health via Conditions (preferred in v1.3+)
	rayClusterStatus := rj.Status.RayClusterStatus
	conditions := rayClusterStatus.Conditions

	// Helper to pull reason/message from a specific condition.
	conditionMessage := func(t string) (reason, message string) {
		if c := meta.FindStatusCondition(conditions, t); c != nil {
			return c.Reason, c.Message
		}
		return "", ""
	}

	// Common recoverable/unhealthy cluster signals -> Degraded (non-terminal)
	if len(conditions) > 0 {
		// ReplicaFailure means pods couldn’t be created/deleted; generally recoverable.
		if meta.IsStatusConditionPresentAndEqual(conditions, string(rayv1.RayClusterReplicaFailure), metav1.ConditionTrue) {
			r, m := conditionMessage(string(rayv1.RayClusterReplicaFailure))
			if m == "" {
				m = "Replica failure while creating/deleting Ray pods"
			}
			return observe.UnitStatus{
				Phase:              observe.UnitDegraded,
				Reason:             ifEmpty(r, "ReplicaFailure"),
				Message:            m,
				ObservedGeneration: rayClusterStatus.ObservedGeneration,
				Conditions:         conditions,
			}, nil
		}
		// Head not ready is also recoverable; keep it non-terminal.
		if meta.IsStatusConditionPresentAndEqual(conditions, string(rayv1.HeadPodReady), metav1.ConditionFalse) {
			r, m := conditionMessage(string(rayv1.HeadPodReady))
			if m == "" {
				m = "Ray head pod is not ready"
			}
			return observe.UnitStatus{
				Phase:              observe.UnitDegraded,
				Reason:             ifEmpty(r, "HeadPodNotReady"),
				Message:            m,
				ObservedGeneration: rayClusterStatus.ObservedGeneration,
				Conditions:         conditions,
			}, nil
		}
		// If we’ve never provisioned the cluster yet, we’re still pending/progressing.
		if !meta.IsStatusConditionPresentAndEqual(conditions, string(rayv1.RayClusterProvisioned), metav1.ConditionTrue) {
			return observe.UnitStatus{
				Phase:              observe.UnitPending, // or UnitProgressing if you prefer
				ObservedGeneration: rayClusterStatus.ObservedGeneration,
				Conditions:         conditions,
			}, nil
		}
	}

	// 4) Non-terminal RayJob states
	switch rj.Status.JobStatus {
	case rayv1.JobStatusNew, rayv1.JobStatusPending:
		return observe.UnitStatus{
			Phase:              observe.UnitPending,
			ObservedGeneration: obsGen,
			Conditions:         conditions,
		}, nil
	case rayv1.JobStatusRunning:
		return observe.UnitStatus{
			Phase:              observe.UnitProgressing,
			ObservedGeneration: obsGen,
			Conditions:         conditions,
		}, nil
	default:
		return observe.UnitStatus{
			Phase:              observe.UnitUnknown,
			Reason:             observe.ReasonUnknownStatus,
			Message:            fmt.Sprintf("unknown RayJob status: %q", rj.Status.JobStatus),
			ObservedGeneration: obsGen,
			Conditions:         conditions,
		}, nil
	}
}

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
