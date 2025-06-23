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

	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	Client   client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme

	// WorkloadHandler is the handler for the actual workload
	WorkloadHandler WorkloadHandler

	// StorageHandler is the handler for the optional storage component (PVCs and download job)
	StorageHandler *StorageHandler

	ClusterContext ClusterContext
}

// Reconcile serves as a central reconciliation function for all Kaiwo workloads. It is broken into the following steps
// 1. Observe the workload status from the cluster
// 2. If the status or conditions have changed, update the status and requeue
// 3. If the status is active, ensure all remote resources match the desired state
func (wr *Reconciler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the latest config
	ctx, err := GetContextWithConfig(ctx, wr.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch kaiwo config: %w", err)
	}

	commonStatusSpec := wr.WorkloadHandler.Workload.GetCommonStatusSpec()

	if status := commonStatusSpec.Status; isTerminalStatus(status) {
		baseutils.Debug(logger, fmt.Sprintf("Skipping reconciliation as status is '%s'", commonStatusSpec.Status))
		return ctrl.Result{}, nil
	} else if status == v1alpha1.WorkloadStatusTerminating {
		if err := TerminateWorkload(ctx, wr.Client, wr.Recorder, wr.WorkloadHandler.Workload); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to terminate workload: %w", err)
		}
		return ctrl.Result{}, nil
	}

	clusterContext, err := GetClusterContext(ctx, wr.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch cluster context: %w", err)
	}
	wr.ClusterContext = *clusterContext

	// Observe the current status based on cluster resources
	observedStatus, conditions, err := wr.observeOverallStatus(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to compute observed status: %w", err)
	}

	conditionsChanged := !ConditionsEqual(conditions, commonStatusSpec.Conditions)
	// If the status is new, update and requeue
	if observedStatus != commonStatusSpec.Status || conditionsChanged {
		if result, err := wr.handleStatusTransition(ctx, observedStatus, conditions); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle status transition: %w", err)
		} else {
			return result, nil
		}
	}

	// If the workload is active, ensure the remote resources match
	if isActiveStatus(observedStatus) {
		if err := wr.ensureAllResources(ctx, observedStatus, conditions); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure all resources: %w", err)
		}
	}

	switch observedStatus {
	case v1alpha1.WorkloadStatusPending:
		// Attempt to clean up expired workloads so that this one can be admitted
		if _, err := CleanupExpiredWorkloads(ctx, wr.Client); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to cleanup expired workloads: %w", err)
		}
	case v1alpha1.WorkloadStatusRunning:
		// Requeue after the workload's duration is expired, so that the workload can become preemptable
		if shouldRequeue, requeueAfter := ShouldRequeueAfter(wr.WorkloadHandler.Workload); shouldRequeue {
			return ctrl.Result{RequeueAfter: *requeueAfter}, nil
		}
	}

	return ctrl.Result{}, nil
}

// observeOverallStatus determines the overall workload's current status based on the current cluster context
func (wr *Reconciler) observeOverallStatus(ctx context.Context) (v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	schedulableCondition := GetSchedulableCondition(ctx, wr.ClusterContext, wr.WorkloadHandler.Workload)
	if schedulableCondition.Status == metav1.ConditionFalse {
		return v1alpha1.WorkloadStatusError, []metav1.Condition{schedulableCondition}, nil
	}

	status := wr.WorkloadHandler.Workload.GetCommonStatusSpec()
	conditions := []metav1.Condition{schedulableCondition}

	if wr.StorageHandler != nil {
		storageStatus, storageConditions, err := wr.StorageHandler.ObserveStatus(ctx, wr.Client, wr.ClusterContext, status.Status)
		if err != nil {
			return "", nil, fmt.Errorf("failed to observe storage status: %w", err)
		}

		conditions = append(conditions, storageConditions...)

		switch storageStatus {
		// If the download job is pending or running, return the Downloading status
		case v1alpha1.WorkloadStatusNew, v1alpha1.WorkloadStatusPending, v1alpha1.WorkloadStatusRunning, v1alpha1.WorkloadStatusStarting:
			return v1alpha1.WorkloadStatusDownloading, conditions, nil
		// Propagate a failed status
		case v1alpha1.WorkloadStatusFailed:
			return storageStatus, conditions, nil
		case v1alpha1.WorkloadStatusComplete:
			// fall-through to workload handler
		default:
			return "", nil, fmt.Errorf(`unexpected storage status "%s"`, storageStatus)
		}

		// Download job is complete and / or storage is healthy
	}

	workloadStatus, workloadConditions, err := wr.WorkloadHandler.ObserveStatus(ctx, wr.Client, wr.ClusterContext, status.Status)
	if err != nil {
		return "", nil, fmt.Errorf("failed to observe workload status: %w", err)
	}
	return workloadStatus, append(conditions, workloadConditions...), nil
}

// handleStatusTransition handles a new status by emitting events and updating the status object
func (wr *Reconciler) handleStatusTransition(ctx context.Context, newStatus v1alpha1.WorkloadStatus, conditions []metav1.Condition) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := wr.WorkloadHandler.Workload.GetKaiwoWorkloadObject()
	commonStatusSpec := wr.WorkloadHandler.Workload.GetCommonStatusSpec()
	previousStatus := commonStatusSpec.Status

	commonStatusSpec.Status = newStatus
	for _, condition := range conditions {
		meta.SetStatusCondition(&commonStatusSpec.Conditions, condition)
	}

	result := ctrl.Result{Requeue: true}

	if previousStatus != newStatus {
		logger.Info(fmt.Sprintf("Workload has new status: %s", newStatus))
		switch newStatus {
		case v1alpha1.WorkloadStatusPending:
			wr.Recorder.Event(obj, corev1.EventTypeNormal, "WorkloadPending", "Workload is pending admission")
		case v1alpha1.WorkloadStatusStarting:
			wr.Recorder.Event(obj, corev1.EventTypeNormal, "WorkloadStarting", "Workload was admitted, starting")
		case v1alpha1.WorkloadStatusRunning:
			wr.Recorder.Event(obj, corev1.EventTypeNormal, "WorkloadRunning", "Workload has started")
			commonStatusSpec.StartTime = &metav1.Time{Time: time.Now()}
		case v1alpha1.WorkloadStatusComplete:
			wr.Recorder.Event(obj, corev1.EventTypeNormal, "WorkloadComplete", "Workload has completed")
		case v1alpha1.WorkloadStatusFailed:
			wr.Recorder.Event(obj, corev1.EventTypeWarning, "WorkloadFailed", "Workload has failed")
		case v1alpha1.WorkloadStatusTerminating:
			wr.Recorder.Event(obj, corev1.EventTypeNormal, "WorkloadTermination", "Workload has been flagged for termination")
		case v1alpha1.WorkloadStatusTerminated:
			wr.Recorder.Event(obj, corev1.EventTypeNormal, "WorkloadTerminated", "Workload has been terminated")
		}
	}
	// Update duration
	switch commonStatusSpec.Status {
	case v1alpha1.WorkloadStatusComplete, v1alpha1.WorkloadStatusFailed, v1alpha1.WorkloadStatusTerminating, v1alpha1.WorkloadStatusRunning:
		if commonStatusSpec.StartTime != nil {
			commonStatusSpec.Duration = int64(time.Since(commonStatusSpec.StartTime.Time).Seconds())
		}
	}

	condition := meta.FindStatusCondition(commonStatusSpec.Conditions, PreemptableConditionType)
	if condition != nil && condition.Status == metav1.ConditionTrue && condition.ObservedGeneration == 0 {
		wr.Recorder.Event(obj, corev1.EventTypeNormal, "Preemptable", "Workload has exceeded its duration")
	}

	if err := wr.Client.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}
	logger.Info(fmt.Sprintf("Result: %v", result))
	return result, nil
}

// isActiveStatus checks if the given status is an active status, i.e. one which requires reconciling any resources
func isActiveStatus(status v1alpha1.WorkloadStatus) bool {
	switch status {
	case
		v1alpha1.WorkloadStatusNew,
		v1alpha1.WorkloadStatusDownloading,
		v1alpha1.WorkloadStatusPending,
		v1alpha1.WorkloadStatusStarting,
		v1alpha1.WorkloadStatusRunning:
		return true
	default:
		return false
	}
}

// isTerminalStatus checks if the given status is a terminal status, i.e. one which cannot be recovered from and does not cause any further changes
func isTerminalStatus(status v1alpha1.WorkloadStatus) bool {
	switch status {
	case
		v1alpha1.WorkloadStatusComplete,
		v1alpha1.WorkloadStatusTerminated,
		v1alpha1.WorkloadStatusFailed:
		return true
	default:
		return false
	}
}

func (wr *Reconciler) ensureAllResources(ctx context.Context, observedStatus v1alpha1.WorkloadStatus, conditions []metav1.Condition) error {
	ensureStorageOnly := false

	// Determine whether to ensure storage or workload
	switch observedStatus {
	case v1alpha1.WorkloadStatusNew:
		// If we first need to perform a download, skip workload for now
		hasDownloads := wr.WorkloadHandler.Workload.GetCommonSpec().Storage.HasDownloads()
		downloadsComplete := meta.IsStatusConditionTrue(conditions, DownloadJobSucceededConditionType)
		ensureStorageOnly = hasDownloads && !downloadsComplete
	case v1alpha1.WorkloadStatusDownloading:
		// If we are currently downloading, skip workload for now
		ensureStorageOnly = true
	default:
		ensureStorageOnly = false
	}

	if err := wr.ensureStorageResources(ctx, wr.ClusterContext); err != nil {
		return fmt.Errorf("failed to ensure storage resources: %w", err)
	}

	if ensureStorageOnly {
		return nil
	}

	if err := wr.ensureWorkloadResources(ctx, wr.ClusterContext); err != nil {
		return fmt.Errorf("failed to ensure workload resources: %w", err)
	}

	return nil
}

// ensureStorageResources ensures that the download resources exist if the workload is in the download phase
func (wr *Reconciler) ensureStorageResources(ctx context.Context, clusterContext ClusterContext) error {
	if wr.StorageHandler == nil {
		return nil
	}
	return wr.reconcileHandler(ctx, clusterContext, wr.StorageHandler)
}

// ensureWorkloadResources ensures that the workload resources exist if the workload is in the main run phase
func (wr *Reconciler) ensureWorkloadResources(ctx context.Context, clusterContext ClusterContext) error {
	if err := wr.ensureLocalQueue(ctx); err != nil {
		return fmt.Errorf("failed to ensure local queue: %w", err)
	}
	return wr.reconcileHandler(ctx, clusterContext, wr.WorkloadHandler)
}

// ensureLocalQueue makes sure a LocalQueue exists for the current namespace / ClusterQueue combination
// If no LocalQueue exists and the KaiwoQueueConfig ClusterConfig has no namespaces defined, a new LocalQueue is created.
// If there are namespaces defined but the workload's namespace is not one of them, an error is raised
func (wr *Reconciler) ensureLocalQueue(ctx context.Context) error {
	namespace := wr.WorkloadHandler.Workload.GetKaiwoWorkloadObject().GetNamespace()
	clusterQueueName := GetClusterQueueName(ctx, wr.WorkloadHandler.Workload)

	if err := EnsureLocalQueue(ctx, wr.Client, wr.Scheme, clusterQueueName, clusterQueueName, namespace); err != nil {
		return fmt.Errorf("failed to ensure local queue: %w", err)
	}
	return nil
}

func (wr *Reconciler) reconcileHandler(ctx context.Context, clusterCtx ClusterContext, handler GroupReconciler) error {
	for _, reconciler := range handler.GetResourceReconcilers(ctx) {
		if err := wr.apply(ctx, clusterCtx, reconciler); err != nil {
			return fmt.Errorf("failed to reconcile resource: %w", err)
		}
	}
	return nil
}

// apply performs an apply (create or update), and ensures that the
// object to create has the controller owner reference set
func (wr *Reconciler) apply(ctx context.Context, clusterCtx ClusterContext, reconciler ResourceReconciler) error {
	logger := log.FromContext(ctx)
	owner := wr.WorkloadHandler.Workload.GetKaiwoWorkloadObject()
	desired, err := reconciler.BuildDesired(ctx, clusterCtx)
	if err != nil {
		return fmt.Errorf("failed to build desired object: %w", err)
	}

	if err := controllerutil.SetControllerReference(owner, desired, wr.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference for object: %w", err)
	}

	result, err := controllerutil.CreateOrPatch(ctx, wr.Client, desired, func() error {
		// inside this function, desired = actual
		return reconciler.MutateActual(ctx, clusterCtx, desired)
	})
	if err != nil {
		return fmt.Errorf("failed to apply object: %w", err)
	}

	if err := wr.stampGVK(desired); err != nil {
		return fmt.Errorf("failed to stamp object: %w", err)
	}
	objectKey := fmt.Sprintf("%s %s/%s", desired.GetObjectKind().GroupVersionKind().String(), desired.GetNamespace(), desired.GetName())
	switch result {
	case controllerutil.OperationResultCreated:
		wr.Recorder.Eventf(owner, corev1.EventTypeNormal, "ResourceCreated", "Created %s: %s", desired.GetObjectKind().GroupVersionKind().String(), desired.GetName())
		logger.Info(fmt.Sprintf("Created object '%s'", objectKey), "name", desired.GetName(), "namespace", desired.GetNamespace(), "gvk", desired.GetObjectKind().GroupVersionKind().String())
	case controllerutil.OperationResultUpdated:
		wr.Recorder.Eventf(owner, corev1.EventTypeNormal, "ResourceUpdated", "Updated %s: %s", desired.GetObjectKind().GroupVersionKind().String(), desired.GetName())
		logger.Info(fmt.Sprintf("Updated object '%s'", objectKey), desired.GetName(), "namespace", desired.GetNamespace(), "gvk", desired.GetObjectKind().GroupVersionKind().String())
	}

	return nil
}

func (wr *Reconciler) stampGVK(obj client.Object) error {
	gvks, _, err := wr.Scheme.ObjectKinds(obj)
	if err != nil {
		return fmt.Errorf("cannot find GVK for %T: %w", obj, err)
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no GVK registered for %T", obj)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}
