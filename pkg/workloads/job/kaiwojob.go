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

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	workloadcommon "github.com/silogen/kaiwo/pkg/workloads/common"
	workloadshared "github.com/silogen/kaiwo/pkg/workloads/common"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type KaiwoJobReconciler struct {
	workloadshared.ReconcilerBase[*kaiwov1alpha1.KaiwoJob]
	DownloadJobConfigMap *workloadshared.DownloadJobConfigMapReconciler
	DownloadJob          *workloadshared.DownloadJobReconciler
	HuggingFacePVC       *workloadshared.StorageReconciler
	DataPVC              *workloadshared.StorageReconciler
	LocalQueue           *workloadshared.LocalQueueReconciler
	BatchJob             *BatchJobReconciler
	RayJob               *RayJobReconciler
}

func NewKaiwoJobReconciler(kaiwoJob *kaiwov1alpha1.KaiwoJob) KaiwoJobReconciler {
	sanitize(kaiwoJob)

	objectKey := client.ObjectKeyFromObject(kaiwoJob)

	reconciler := KaiwoJobReconciler{
		ReconcilerBase: workloadshared.ReconcilerBase[*kaiwov1alpha1.KaiwoJob]{
			Object:    kaiwoJob,
			ObjectKey: objectKey,
		},
	}
	reconciler.Self = &reconciler

	storageSpec := kaiwoJob.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {
		if storageSpec.HasData() {
			reconciler.DataPVC = workloadshared.NewStorageReconciler(
				client.ObjectKey{
					Name:      baseutils.FormatNameWithPostfix(objectKey.Name, workloadshared.DataStoragePostfix),
					Namespace: objectKey.Namespace,
				},
				storageSpec.AccessMode,
				storageSpec.StorageClassName,
				storageSpec.Data.StorageSize,
			)
		}
		if storageSpec.HasHfDownloads() {
			reconciler.HuggingFacePVC = workloadshared.NewStorageReconciler(
				client.ObjectKey{
					Name:      baseutils.FormatNameWithPostfix(objectKey.Name, workloadshared.HfStoragePostfix),
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
			reconciler.DownloadJobConfigMap = workloadshared.NewDownloadJobConfigMapReconciler(downloadObjectKey, storageSpec)
			reconciler.DownloadJob = workloadshared.NewDownloadJobReconciler(downloadObjectKey, storageSpec, objectKey.Name, baseutils.ValueOrDefault(kaiwoJob.Spec.Env))
		}
	}

	clusterQueue := baseutils.ValueOrDefault(kaiwoJob.Spec.ClusterQueue)
	if clusterQueue == "" {
		clusterQueue = workloadshared.DefaultLocalQueueName
	}
	reconciler.LocalQueue = workloadshared.NewLocalQueueReconciler(client.ObjectKey{Namespace: objectKey.Namespace, Name: clusterQueue})

	if kaiwoJob.Spec.IsBatchJob() {
		reconciler.BatchJob = NewBatchJobReconciler(kaiwoJob)
	} else if kaiwoJob.Spec.IsRayJob() {
		reconciler.RayJob = NewRayJobReconciler(kaiwoJob)
	} else {
		panic("Unknown Kaiwo job spec")
	}

	return reconciler
}

func sanitize(kaiwoJob *kaiwov1alpha1.KaiwoJob) {
	storageSpec := kaiwoJob.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {

		// Ensure mount paths are set
		if storageSpec.Data != nil && storageSpec.Data.IsRequested() && storageSpec.Data.MountPath == "" {
			// logger.Info("Data storage mount path not set, using default:" + defaultDataMountPath)
			storageSpec.Data.MountPath = workloadshared.DefaultDataMountPath
		}
		if storageSpec.HuggingFace != nil && storageSpec.HuggingFace.IsRequested() && storageSpec.HuggingFace.MountPath == "" {
			// logger.Info("Hugging Face storage mount path not set, using default:" + defaultHfMountPath)
			storageSpec.HuggingFace.MountPath = workloadshared.DefaultHfMountPath
		}
	}

	if kaiwoJob.Labels == nil {
		kaiwoJob.Labels = make(map[string]string)
	}

	if baseutils.ValueOrDefault(kaiwoJob.Spec.ClusterQueue) == "" {
		kaiwoJob.Labels[kaiwov1alpha1.QueueLabel] = controllerutils.DefaultKaiwoQueueConfigName
	} else {
		kaiwoJob.Labels[kaiwov1alpha1.QueueLabel] = baseutils.ValueOrDefault(kaiwoJob.Spec.ClusterQueue)
	}
}

// Reconcile reconciles the kaiwo job to ensure each resource exists and is in the desired state
func (r *KaiwoJobReconciler) Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, dryRun bool) (ctrl.Result, []client.Object, error) {
	logger := log.FromContext(ctx)

	kaiwoJob := r.Object

	var manifests []client.Object

	storageSpec := kaiwoJob.Spec.Storage

	var downloadJob *batchv1.Job
	var downloadJobResult *ctrl.Result
	if storageSpec != nil && storageSpec.StorageEnabled {

		if storageSpec.HasData() {
			dataPvc, _, err := r.DataPVC.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile data PVC: %w", err)
			}
			manifests = append(manifests, dataPvc)
		}

		if storageSpec.HasHfDownloads() {
			// Add HuggingFace PVC
			hfPvc, _, err := r.HuggingFacePVC.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile HuggingFace PVC: %w", err)
			}
			manifests = append(manifests, hfPvc)
		}

		if storageSpec.HasDownloads() {
			downloadJobConfigMap, _, err := r.DownloadJobConfigMap.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile DownloadJobConfigMap: %w", err)
			}
			manifests = append(manifests, downloadJobConfigMap)

			downloadJob, downloadJobResult, err = r.DownloadJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile DownloadJob: %w", err)
			}
			manifests = append(manifests, downloadJob)
		}
	}

	if downloadJobResult == nil {

		localQueue, _, err := r.LocalQueue.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
		if err != nil {
			return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile local queue: %w", err)
		}
		manifests = append(manifests, localQueue)

		if kaiwoJob.Spec.IsBatchJob() {
			batchJob, _, err := r.BatchJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile BatchJob: %w", err)
			}
			manifests = append(manifests, batchJob)
		} else if kaiwoJob.Spec.IsRayJob() {
			rayJob, _, err := r.RayJob.Reconcile(ctx, k8sClient, scheme, kaiwoJob, dryRun)
			if err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile RayJob: %w", err)
			}
			manifests = append(manifests, rayJob)
		} else {
			panic("Unsupported job configuration")
		}
	}

	if dryRun {
		return ctrl.Result{}, manifests, nil
	}

	previousStatus := kaiwoJob.Status.DeepCopy()
	status, err := r.GatherStatus(ctx, k8sClient, *previousStatus, downloadJob)
	if err != nil {
		return ctrl.Result{}, nil, fmt.Errorf("failed to gather status: %w", err)
	}

	if reflect.DeepEqual(previousStatus, status) {
		return ctrl.Result{}, nil, nil
	}

	retryAttempts := 3
	for i := 0; i < retryAttempts; i++ {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kaiwoJob), kaiwoJob); err != nil {
			return ctrl.Result{}, nil, fmt.Errorf("failed to get kaiwoJob: %w", err)
		}

		kaiwoJob.Status = *status
		logger.Info(fmt.Sprintf("Updating status to: %s", string(kaiwoJob.Status.Status)), "status", kaiwoJob.Status)

		if err := k8sClient.Status().Update(ctx, kaiwoJob); err != nil {
			if errors.IsConflict(err) {
				baseutils.Debug(logger, "Conflict error during KaiwoJob update, retrying", "attempt", i+1)
				continue
			}
			return ctrl.Result{}, nil, fmt.Errorf("failed to update kaiwoJob status: %w", err)
		}

		return ctrl.Result{}, nil, nil
	}
	return ctrl.Result{}, nil, fmt.Errorf("failed to update kaiwoJob status")
}

func (r *KaiwoJobReconciler) GatherStatus(ctx context.Context, k8sClient client.Client, previousStatus kaiwov1alpha1.KaiwoJobStatus, downloadJob *batchv1.Job) (*kaiwov1alpha1.KaiwoJobStatus, error) {
	kaiwoJob := r.Object

	currentStatus := previousStatus.DeepCopy()

	// Check if download job has failed
	if downloadJob != nil {
		if downloadJob.Status.Failed > 0 {
			currentStatus.Status = kaiwov1alpha1.StatusFailed
			return currentStatus, nil
		} else if downloadJob.Status.Succeeded == 0 {
			currentStatus.Status = kaiwov1alpha1.StatusPending
			// Download job still ongoing
			return currentStatus, nil
		}
	}

	jobSucceeded, jobFailed, err := checkJobCompletion(ctx, k8sClient, kaiwoJob)
	if err != nil {
		return nil, fmt.Errorf("failed to check job completion: %w", err)
	}

	var status kaiwov1alpha1.Status

	if currentStatus.StartTime == nil {
		if startTime := workloadcommon.GetEarliestPodStartTime(ctx, k8sClient, kaiwoJob.Name, kaiwoJob.Namespace); startTime != nil {
			currentStatus.StartTime = startTime
		}
	}

	if jobSucceeded || jobFailed {
		if jobSucceeded {
			status = kaiwov1alpha1.StatusComplete
		} else {
			status = kaiwov1alpha1.StatusFailed
		}
		if currentStatus.CompletionTime == nil {
			currentStatus.CompletionTime = baseutils.Pointer(metav1.Now())
		}
		if currentStatus.StartTime != nil {
			currentStatus.Duration = int64(currentStatus.CompletionTime.Time.Sub(currentStatus.StartTime.Time).Seconds())
		}
	} else {
		startTime, latestStatus, err := workloadcommon.CheckPodStatus(ctx, k8sClient, kaiwoJob.Name, kaiwoJob.Namespace, currentStatus.StartTime)
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

	if status != kaiwov1alpha1.StatusNew {
		currentStatus.Status = status
	}

	return currentStatus, nil
}

func checkJobCompletion(ctx context.Context, k8sClient client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob) (bool, bool, error) {
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
