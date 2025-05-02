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

package workloadjob

import (
	"context"
	"fmt"
	"reflect"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	common "github.com/silogen/kaiwo/pkg/workloads/common"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type KaiwoJobReconciler struct {
	common.ReconcilerBase[*kaiwo.KaiwoJob]
	DownloadJobConfigMap *workloadutils.DownloadJobConfigMapReconciler
	DownloadJob          *workloadutils.DownloadJobReconciler
	HuggingFacePVC       *common.StorageReconciler
	DataPVC              *common.StorageReconciler
	LocalQueue           *common.LocalQueueReconciler
	BatchJob             *BatchJobReconciler
	RayJob               *RayJobReconciler
}

func NewKaiwoJobReconciler(ctx context.Context, kaiwoJob *kaiwo.KaiwoJob) KaiwoJobReconciler {
	config := controllerutils.ConfigFromContext(ctx)
	sanitize(kaiwoJob, config)

	objectKey := client.ObjectKeyFromObject(kaiwoJob)

	reconciler := KaiwoJobReconciler{
		ReconcilerBase: common.ReconcilerBase[*kaiwo.KaiwoJob]{
			Object:    kaiwoJob,
			ObjectKey: objectKey,
		},
	}
	reconciler.Self = &reconciler

	storageSpec := kaiwoJob.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {
		if storageSpec.HasData() {
			reconciler.DataPVC = common.NewStorageReconciler(
				config.Storage,
				client.ObjectKey{
					Name:      baseutils.FormatNameWithPostfix(objectKey.Name, common.DataStoragePostfix),
					Namespace: objectKey.Namespace,
				},
				storageSpec.AccessMode,
				storageSpec.StorageClassName,
				storageSpec.Data.StorageSize,
			)
		}
		if storageSpec.HasHfDownloads() {
			reconciler.HuggingFacePVC = common.NewStorageReconciler(
				config.Storage,
				client.ObjectKey{
					Name:      baseutils.FormatNameWithPostfix(objectKey.Name, common.HfStoragePostfix),
					Namespace: objectKey.Namespace,
				},
				storageSpec.AccessMode,
				storageSpec.StorageClassName,
				storageSpec.HuggingFace.StorageSize,
			)
		}
		if storageSpec.HasDownloads() {
			downloadObjectKey := client.ObjectKey{
				Namespace: objectKey.Namespace,
				Name:      baseutils.FormatNameWithPostfix(objectKey.Name, "download"),
			}
			reconciler.DownloadJobConfigMap = workloadutils.NewDownloadJobConfigMapReconciler(downloadObjectKey, storageSpec)
			reconciler.DownloadJob = workloadutils.NewDownloadJobReconciler(downloadObjectKey, storageSpec, objectKey.Name, kaiwoJob.Spec.Env)
		}
	}

	clusterQueue := kaiwoJob.Spec.ClusterQueue
	if clusterQueue == "" {
		clusterQueue = common.DefaultClusterQueueName
	}
	reconciler.LocalQueue = common.NewLocalQueueReconciler(client.ObjectKey{Namespace: objectKey.Namespace, Name: clusterQueue})

	if kaiwoJob.Spec.IsBatchJob() {
		reconciler.BatchJob = NewBatchJobReconciler(kaiwoJob)
	} else if kaiwoJob.Spec.IsRayJob() {
		reconciler.RayJob = NewRayJobReconciler(kaiwoJob)
	} else {
		panic("Unknown Kaiwo job spec")
	}

	return reconciler
}

func sanitize(kaiwoJob *kaiwo.KaiwoJob, config controllerutils.KaiwoConfigContext) {
	storageSpec := kaiwoJob.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {

		// Ensure mount paths are set
		if storageSpec.Data != nil && storageSpec.Data.IsRequested() && storageSpec.Data.MountPath == "" {
			// logger.Info("Storage storage mount path not set, using default:" + defaultDataMountPath)
			storageSpec.Data.MountPath = config.Storage.DefaultDataMountPath
		}
		if storageSpec.HuggingFace != nil && storageSpec.HuggingFace.IsRequested() && storageSpec.HuggingFace.MountPath == "" {
			// logger.Info("Hugging Face storage mount path not set, using default:" + defaultHfMountPath)
			storageSpec.HuggingFace.MountPath = config.Storage.DefaultHfMountPath
		}
	}

	if kaiwoJob.Labels == nil {
		kaiwoJob.Labels = make(map[string]string)
	}

	if kaiwoJob.Spec.ClusterQueue == "" {
		kaiwoJob.Labels[common.QueueLabel] = common.DefaultClusterQueueName
	} else {
		kaiwoJob.Labels[common.QueueLabel] = kaiwoJob.Spec.ClusterQueue
	}
}

// Reconcile reconciles the kaiwo job to ensure each resource exists and is in the desired state
func (r *KaiwoJobReconciler) Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	kaiwoJob := r.Object

	storageSpec := kaiwoJob.Spec.Storage

	var downloadJob *batchv1.Job
	var downloadJobResult *ctrl.Result

	if k8sClient != nil {
		if err := controllerutils.EnsureNamespaceKueueManaged(ctx, k8sClient, r.ObjectKey.Namespace); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure namespace is Kueue managed: %w", err)
		}
	}

	if storageSpec != nil && storageSpec.StorageEnabled {

		if storageSpec.HasData() {
			_, _, err := r.DataPVC.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile data PVC: %w", err)
			}
		}

		if storageSpec.HasHfDownloads() {
			// Add HuggingFace PVC
			_, _, err := r.HuggingFacePVC.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile HuggingFace PVC: %w", err)
			}
		}

		if storageSpec.HasDownloads() {
			_, _, err := r.DownloadJobConfigMap.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile DownloadJobConfigMap: %w", err)
			}

			downloadJob, downloadJobResult, err = r.DownloadJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile DownloadJob: %w", err)
			}
			if downloadJobResult != nil {
				if downloadJobResult.Requeue || downloadJobResult.RequeueAfter > 0 {
					return *downloadJobResult, nil
				}
			}
		}
	}

	if downloadJobResult == nil {

		_, _, err := r.LocalQueue.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile local queue: %w", err)
		}

		if kaiwoJob.Spec.IsBatchJob() {
			_, _, err := r.BatchJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile BatchJob: %w", err)
			}
		} else if kaiwoJob.Spec.IsRayJob() {
			_, _, err := r.RayJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile RayJob: %w", err)
			}
		} else {
			panic("Unsupported job configuration")
		}
	}

	previousStatus := kaiwoJob.Status.DeepCopy()
	status, err := r.GatherStatus(ctx, k8sClient, *previousStatus, downloadJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to gather status: %w", err)
	}

	if reflect.DeepEqual(previousStatus, status) {
		if status.Status == kaiwo.StatusPending {
			logger.Info("Still pending, requeuing...")
			return ctrl.Result{RequeueAfter: common.DefaultRequeueDuration}, nil
		}
		return ctrl.Result{}, nil
	}

	retryAttempts := 3
	for i := 0; i < retryAttempts; i++ {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kaiwoJob), kaiwoJob); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get kaiwoJob: %w", err)
		}

		kaiwoJob.Status = *status
		logger.Info(fmt.Sprintf("Updating status to: %s", string(kaiwoJob.Status.Status)), "status", kaiwoJob.Status)

		if err := k8sClient.Status().Update(ctx, kaiwoJob); err != nil {
			if errors.IsConflict(err) {
				baseutils.Debug(logger, "Conflict error during KaiwoJob update, retrying", "attempt", i+1)
				continue
			}
			return ctrl.Result{}, fmt.Errorf("failed to update kaiwoJob status: %w", err)
		}

		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, fmt.Errorf("failed to update kaiwoJob status")
}

func (r *KaiwoJobReconciler) GatherStatus(ctx context.Context, k8sClient client.Client, previousStatus kaiwo.KaiwoJobStatus, downloadJob *batchv1.Job) (*kaiwo.KaiwoJobStatus, error) {
	kaiwoJob := r.Object

	currentStatus := previousStatus.DeepCopy()

	// Set default condition if none is found
	cond := meta.FindStatusCondition(r.Object.Status.Conditions, kaiwo.KaiwoResourceUtilizationType)
	if cond == nil {
		meta.SetStatusCondition(&r.Object.Status.Conditions, metav1.Condition{
			Type:    kaiwo.KaiwoResourceUtilizationType,
			Status:  metav1.ConditionFalse,
			Reason:  string(kaiwo.ResourceUtilizationUnknown),
			Message: "Resource utilization currently unknown",
		})
	}

	// Check if download job has failed
	if downloadJob != nil {
		if downloadJob.Status.Failed > 0 {
			currentStatus.Status = kaiwo.StatusFailed
			return currentStatus, nil
		} else if downloadJob.Status.Succeeded == 0 {
			currentStatus.Status = kaiwo.StatusPending
			// Download job still ongoing
			return currentStatus, nil
		}
	}

	jobSucceeded, jobFailed, err := checkJobCompletion(ctx, k8sClient, kaiwoJob)
	if err != nil {
		return nil, fmt.Errorf("failed to check job completion: %w", err)
	}

	var status kaiwo.Status

	if currentStatus.StartTime == nil {
		if startTime := workloadutils.GetEarliestPodStartTime(ctx, k8sClient, kaiwoJob.Name, kaiwoJob.Namespace); startTime != nil {
			currentStatus.StartTime = startTime
		}
	}

	if jobSucceeded || jobFailed {
		if jobSucceeded {
			status = kaiwo.StatusComplete
		} else {
			status = kaiwo.StatusFailed
		}
		if currentStatus.CompletionTime == nil {
			currentStatus.CompletionTime = baseutils.Pointer(metav1.Now())
		}
		if currentStatus.StartTime != nil {
			currentStatus.Duration = int64(currentStatus.CompletionTime.Time.Sub(currentStatus.StartTime.Time).Seconds())
		}
	} else {
		startTime, latestStatus, err := workloadutils.CheckPodStatus(ctx, k8sClient, kaiwoJob.Name, kaiwoJob.Namespace, currentStatus.StartTime)
		if err != nil {
			return nil, fmt.Errorf("error fetching start time and status: %w", err)
		}
		if startTime != nil {
			currentStatus.StartTime = startTime
			if currentStatus.CompletionTime != nil {
				currentStatus.Duration = int64(kaiwoJob.Status.CompletionTime.Time.Sub(kaiwoJob.Status.StartTime.Time).Seconds())
			}
		}
		status = latestStatus
	}

	if status != kaiwo.StatusNew {
		currentStatus.Status = status
	}

	return currentStatus, nil
}

func checkJobCompletion(ctx context.Context, k8sClient client.Client, kaiwoJob *kaiwo.KaiwoJob) (bool, bool, error) {
	logger := log.FromContext(ctx)
	var jobSucceeded, jobFailed bool

	if kaiwoJob.Spec.IsBatchJob() {
		var job batchv1.Job
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &job); err == nil {
			failedJobs := job.Status.Failed
			succeededJobs := job.Status.Succeeded

			if job.Status.UncountedTerminatedPods != nil {
				failedJobs += int32(len((*job.Status.UncountedTerminatedPods).Failed))
				succeededJobs += int32(len((*job.Status.UncountedTerminatedPods).Succeeded))
			}

			jobFailed = failedJobs > 0
			jobSucceeded = succeededJobs > 0
		} else if !errors.IsNotFound(err) {
			return false, false, baseutils.LogErrorf(logger, "failed to check job status", err)
		}
	} else if kaiwoJob.Spec.IsRayJob() {
		var rayJob rayv1.RayJob
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &rayJob); err == nil {
			switch rayJob.Status.JobStatus {
			case rayv1.JobStatusFailed:
				jobFailed = true
			case rayv1.JobStatusSucceeded:
				jobSucceeded = true
			}
		} else if !errors.IsNotFound(err) {
			return false, false, baseutils.LogErrorf(logger, "failed to check ray job status", err)
		}
	} else {
		return false, false, baseutils.LogErrorf(logger, "KaiwoJob does not specify a valid Job or RayJob", nil)
	}

	return jobSucceeded, jobFailed, nil
}
