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

package workloadservice

import (
	"context"
	"fmt"
	"reflect"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

type KaiwoServiceReconciler struct {
	common.ReconcilerBase[*kaiwo.KaiwoService]

	DownloadJobConfigMap *workloadutils.DownloadJobConfigMapReconciler
	DownloadJob          *workloadutils.DownloadJobReconciler
	HuggingFacePVC       *common.StorageReconciler
	DataPVC              *common.StorageReconciler
	LocalQueue           *common.LocalQueueReconciler

	DeploymentReconciler *DeploymentReconciler
	RayServiceReconciler *RayServiceReconciler
}

func NewKaiwoServiceReconciler(ctx context.Context, kaiwoService *kaiwo.KaiwoService) KaiwoServiceReconciler {
	config := controllerutils.ConfigFromContext(ctx)
	sanitize(kaiwoService, config)

	objectKey := client.ObjectKeyFromObject(kaiwoService)
	r := KaiwoServiceReconciler{
		ReconcilerBase: common.ReconcilerBase[*kaiwo.KaiwoService]{
			Object:    kaiwoService,
			ObjectKey: objectKey,
		},
	}
	r.Self = &r

	storageSpec := kaiwoService.Spec.Storage
	if storageSpec != nil && storageSpec.StorageEnabled {
		if storageSpec.HasData() {
			r.DataPVC = common.NewStorageReconciler(
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
			r.HuggingFacePVC = common.NewStorageReconciler(
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
			r.DownloadJobConfigMap = workloadutils.NewDownloadJobConfigMapReconciler(downloadObjectKey, storageSpec)
			r.DownloadJob = workloadutils.NewDownloadJobReconciler(downloadObjectKey, storageSpec, objectKey.Name, kaiwoService.Spec.Env)
		}
	}

	clusterQueue := kaiwoService.Spec.ClusterQueue
	if clusterQueue == "" {
		clusterQueue = common.DefaultClusterQueueName
	}
	r.LocalQueue = common.NewLocalQueueReconciler(
		client.ObjectKey{Namespace: objectKey.Namespace, Name: clusterQueue},
	)

	if kaiwoService.Spec.IsRayService() {
		r.RayServiceReconciler = NewRayServiceReconciler(kaiwoService)
	} else {
		r.DeploymentReconciler = NewDeploymentReconciler(kaiwoService)
	}

	return r
}

func sanitize(kaiwoService *kaiwo.KaiwoService, config controllerutils.KaiwoConfigContext) {
	storageSpec := kaiwoService.Spec.Storage

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

	if kaiwoService.Labels == nil {
		kaiwoService.Labels = make(map[string]string)
	}

	if kaiwoService.Spec.ClusterQueue == "" {
		kaiwoService.Labels[common.QueueLabel] = common.DefaultClusterQueueName
	} else {
		kaiwoService.Labels[common.QueueLabel] = kaiwoService.Spec.ClusterQueue
	}
}

func (r *KaiwoServiceReconciler) Reconcile(
	ctx context.Context,
	k8sClient client.Client,
	scheme *runtime.Scheme,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	svc := r.Object

	storageSpec := svc.Spec.Storage
	var downloadJobResult *ctrl.Result
	var downloadJob *batchv1.Job

	if k8sClient != nil {
		if err := controllerutils.EnsureNamespaceKueueManaged(ctx, k8sClient, r.ObjectKey.Namespace); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure namespace is Kueue managed: %w", err)
		}
	}

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

	if storageSpec != nil && storageSpec.StorageEnabled {
		if storageSpec.HasData() {
			_, _, err := r.DataPVC.Reconcile(ctx, k8sClient, scheme, svc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile data PVC: %w", err)
			}
		}

		if storageSpec.HasHfDownloads() {
			_, _, err := r.HuggingFacePVC.Reconcile(ctx, k8sClient, scheme, svc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile huggingface PVC: %w", err)
			}
		}

		if storageSpec.HasDownloads() {
			_, _, err := r.DownloadJobConfigMap.Reconcile(ctx, k8sClient, scheme, svc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile download configmap: %w", err)
			}

			downloadJob, downloadJobResult, err = r.DownloadJob.Reconcile(ctx, k8sClient, scheme, svc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile download job: %w", err)
			}
			if downloadJobResult != nil {
				if downloadJobResult.Requeue || downloadJobResult.RequeueAfter > 0 {
					return *downloadJobResult, nil
				}
			}
		}
	}

	if downloadJobResult == nil {
		_, _, err := r.LocalQueue.Reconcile(ctx, k8sClient, scheme, svc)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile local queue: %w", err)
		}

		if svc.Spec.IsRayService() {
			_, _, err := r.RayServiceReconciler.Reconcile(ctx, k8sClient, scheme, svc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile RayService: %w", err)
			}
		} else {
			_, _, err := r.DeploymentReconciler.Reconcile(ctx, k8sClient, scheme, svc)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile Deployment: %w", err)
			}
		}
	}

	previousStatus := svc.Status.DeepCopy()
	status, err := r.GatherStatus(ctx, k8sClient, *previousStatus, downloadJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to gather service status: %w", err)
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
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get KaiwoService: %w", err)
		}

		svc.Status = *status
		logger.Info("Updating KaiwoService status", "status", svc.Status.Status)

		if err := k8sClient.Status().Update(ctx, svc); err != nil {
			if errors.IsConflict(err) {
				baseutils.Debug(logger, "Conflict error during KaiwoService update, retrying", "attempt", i+1)
				continue
			}
			return ctrl.Result{}, fmt.Errorf("failed to update KaiwoService status: %w", err)
		}
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, fmt.Errorf("failed to update KaiwoService status after retries")
}

func (r *KaiwoServiceReconciler) GatherStatus(
	ctx context.Context,
	k8sClient client.Client,
	previousStatus kaiwo.KaiwoServiceStatus,
	downloadJob *batchv1.Job,
) (*kaiwo.KaiwoServiceStatus, error) {
	svc := r.Object
	currentStatus := previousStatus.DeepCopy()

	// 1. Check download job first. If it's ongoing or failed, that decides overall state.
	if downloadJob != nil {
		if downloadJob.Status.Failed > 0 {
			currentStatus.Status = kaiwo.StatusFailed
			return currentStatus, nil
		} else if downloadJob.Status.Succeeded == 0 {
			// Not succeeded yet => Pending
			currentStatus.Status = kaiwo.StatusPending
			return currentStatus, nil
		}
		// If the download job is succeeded (>0) we continue checking RayService or Deployment
	}

	// 2. Fill in startTime if it’s not set yet
	if currentStatus.StartTime == nil {
		if startTime := workloadutils.GetEarliestPodStartTime(
			ctx,
			k8sClient,
			svc.Name,
			svc.Namespace,
		); startTime != nil {
			currentStatus.StartTime = startTime
		}
	}

	// 3. If RayService
	if svc.Spec.IsRayService() {
		var rayService rayv1.RayService
		err := k8sClient.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &rayService)
		if err != nil {
			if errors.IsNotFound(err) {
				currentStatus.Status = kaiwo.StatusPending
				return currentStatus, nil
			}
			currentStatus.Status = kaiwo.StatusFailed
			return currentStatus, nil
		}

		if meta.IsStatusConditionTrue(rayService.Status.Conditions, string(rayv1.RayServiceReady)) {
			currentStatus.Status = kaiwo.StatusRunning
			return currentStatus, nil
		}

		for _, appStat := range rayService.Status.ActiveServiceStatus.Applications {
			if appStat.Status == "UNHEALTHY" || appStat.Status == "DEPLOY_FAILED" {
				currentStatus.Status = kaiwo.StatusFailed
				return currentStatus, nil
			}
		}

	} else {
		var dep appsv1.Deployment
		err := k8sClient.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &dep)
		if err != nil {
			if errors.IsNotFound(err) {
				// maybe the Deployment is not created yet
				currentStatus.Status = kaiwo.StatusPending
			} else {
				currentStatus.Status = kaiwo.StatusFailed
			}
			return currentStatus, nil
		}

		if isDeploymentFailed(dep) {
			currentStatus.Status = kaiwo.StatusFailed
			return currentStatus, nil
		}

		if dep.Status.ReadyReplicas == dep.Status.Replicas && dep.Status.Replicas > 0 {
			currentStatus.Status = kaiwo.StatusRunning
			return currentStatus, nil
		}
	}
	_, latestStatus, err := workloadutils.CheckPodStatus(ctx, k8sClient, svc.Name, svc.Namespace, currentStatus.StartTime)
	currentStatus.Status = latestStatus
	return currentStatus, err
}

func isDeploymentFailed(dep appsv1.Deployment) bool {
	for _, cond := range dep.Status.Conditions {
		if cond.Type == appsv1.DeploymentProgressing &&
			cond.Reason == "ProgressDeadlineExceeded" &&
			cond.Status == corev1.ConditionFalse {
			return true
		}
	}
	return false
}
