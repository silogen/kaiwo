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

    appsv1 "k8s.io/api/apps/v1"
    batchv1 "k8s.io/api/batch/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
    ctrl "sigs.k8s.io/controller-runtime"

    rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
    kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
    baseutils "github.com/silogen/kaiwo/pkg/utils"
    workloadcommon "github.com/silogen/kaiwo/pkg/workloads/common"
    controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

type KaiwoServiceReconciler struct {
    workloadcommon.ReconcilerBase[*kaiwov1alpha1.KaiwoService]

    DownloadJobConfigMap *workloadcommon.DownloadJobConfigMapReconciler
    DownloadJob          *workloadcommon.DownloadJobReconciler
    HuggingFacePVC       *workloadcommon.StorageReconciler
    DataPVC              *workloadcommon.StorageReconciler
    LocalQueue           *workloadcommon.LocalQueueReconciler

    DeploymentReconciler *DeploymentReconciler
    RayServiceReconciler *RayServiceReconciler
}

func NewKaiwoServiceReconciler(kaiwoService *kaiwov1alpha1.KaiwoService) KaiwoServiceReconciler {
    sanitize(kaiwoService)

    objectKey := client.ObjectKeyFromObject(kaiwoService)
    r := KaiwoServiceReconciler{
        ReconcilerBase: workloadcommon.ReconcilerBase[*kaiwov1alpha1.KaiwoService]{
            Object:    kaiwoService,
            ObjectKey: objectKey,
        },
    }
    r.Self = &r

    storageSpec := kaiwoService.Spec.Storage
    if storageSpec != nil && storageSpec.StorageEnabled {
        if storageSpec.HasData() {
            r.DataPVC = workloadcommon.NewStorageReconciler(
                client.ObjectKey{
                    Name:      baseutils.FormatNameWithPostfix(objectKey.Name, workloadcommon.DataStoragePostfix),
                    Namespace: objectKey.Namespace,
                },
                storageSpec.AccessMode,
                storageSpec.StorageClassName,
                storageSpec.Data.StorageSize,
            )
        }
        if storageSpec.HasHfDownloads() {
            r.HuggingFacePVC = workloadcommon.NewStorageReconciler(
                client.ObjectKey{
                    Name:      baseutils.FormatNameWithPostfix(objectKey.Name, workloadcommon.HfStoragePostfix),
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
            r.DownloadJobConfigMap = workloadcommon.NewDownloadJobConfigMapReconciler(downloadObjectKey, storageSpec)
            r.DownloadJob = workloadcommon.NewDownloadJobReconciler(downloadObjectKey, storageSpec, objectKey.Name)
        }
    }

    clusterQueue := baseutils.ValueOrDefault(kaiwoService.Spec.ClusterQueue)
    if clusterQueue == "" {
        clusterQueue = workloadcommon.DefaultLocalQueueName
    }
    r.LocalQueue = workloadcommon.NewLocalQueueReconciler(
        client.ObjectKey{Namespace: objectKey.Namespace, Name: clusterQueue},
    )

    if kaiwoService.Spec.IsRayService() {
        r.RayServiceReconciler = NewRayServiceReconciler(kaiwoService)
    } else {
        r.DeploymentReconciler = NewDeploymentReconciler(kaiwoService)
    }

    return r
}

func sanitize(kaiwoService *kaiwov1alpha1.KaiwoService) {#
    
    if kaiwoService.Labels == nil {
        kaiwoService.Labels = make(map[string]string)
    }

    if baseutils.ValueOrDefault(kaiwoService.Spec.ClusterQueue) == "" {
        kaiwoService.Labels[kaiwov1alpha1.QueueLabel] = controllerutils.DefaultKaiwoQueueConfigName
    } else {
        kaiwoService.Labels[kaiwov1alpha1.QueueLabel] = baseutils.ValueOrDefault(kaiwoService.Spec.ClusterQueue)
    }
}

func (r *KaiwoServiceReconciler) Reconcile(
    ctx context.Context,
    k8sClient client.Client,
    scheme *runtime.Scheme,
    dryRun bool,
) (ctrl.Result, []client.Object, error) {

    logger := log.FromContext(ctx)
    svc := r.Object

    var manifests []client.Object

    storageSpec := svc.Spec.Storage
    var downloadJobResult *ctrl.Result
    var downloadJob *batchv1.Job

    if storageSpec != nil && storageSpec.StorageEnabled {
        if storageSpec.HasData() {
            dataPvc, _, err := r.DataPVC.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
            if err != nil {
                return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile data PVC: %w", err)
            }
            manifests = append(manifests, dataPvc)
        }

        if storageSpec.HasHfDownloads() {
            hfPvc, _, err := r.HuggingFacePVC.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
            if err != nil {
                return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile huggingface PVC: %w", err)
            }
            manifests = append(manifests, hfPvc)
        }

        if storageSpec.HasDownloads() {
            downloadJobConfigMap, _, err := r.DownloadJobConfigMap.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
            if err != nil {
                return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile download configmap: %w", err)
            }
            manifests = append(manifests, downloadJobConfigMap)

            downloadJob, downloadJobResult, err = r.DownloadJob.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
            if err != nil {
                return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile download job: %w", err)
            }
            manifests = append(manifests, downloadJob)
        }
    }

    if downloadJobResult == nil {
        localQueue, _, err := r.LocalQueue.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
        if err != nil {
            return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile local queue: %w", err)
        }
        manifests = append(manifests, localQueue)

        if svc.Spec.IsRayService() {
            rayService, _, err := r.RayServiceReconciler.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
            if err != nil {
                return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile RayService: %w", err)
            }
            manifests = append(manifests, rayService)
        } else {
            deployment, _, err := r.DeploymentReconciler.Reconcile(ctx, k8sClient, scheme, svc, dryRun)
            if err != nil {
                return ctrl.Result{}, nil, fmt.Errorf("failed to reconcile Deployment: %w", err)
            }
            manifests = append(manifests, deployment)
        }
    }

    if dryRun {
        return ctrl.Result{}, manifests, nil
    }

    previousStatus := svc.Status.DeepCopy()
    status, err := r.GatherStatus(ctx, k8sClient, *previousStatus, downloadJob)
    if err != nil {
        return ctrl.Result{}, nil, fmt.Errorf("failed to gather service status: %w", err)
    }

    if reflect.DeepEqual(previousStatus, status) {
        return ctrl.Result{}, nil, nil
    }

    retryAttempts := 3
    for i := 0; i < retryAttempts; i++ {
        if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc); err != nil {
            return ctrl.Result{}, nil, fmt.Errorf("failed to get KaiwoService: %w", err)
        }

        svc.Status = *status
        logger.Info("Updating KaiwoService status", "status", svc.Status.Status)

        if err := k8sClient.Status().Update(ctx, svc); err != nil {
            if errors.IsConflict(err) {
                baseutils.Debug(logger, "Conflict error during KaiwoService update, retrying", "attempt", i+1)
                continue
            }
            return ctrl.Result{}, nil, fmt.Errorf("failed to update KaiwoService status: %w", err)
        }
        return ctrl.Result{}, nil, nil
    }
    return ctrl.Result{}, nil, fmt.Errorf("failed to update KaiwoService status after retries")
}

func (r *KaiwoServiceReconciler) GatherStatus(
    ctx context.Context,
    k8sClient client.Client,
    previousStatus kaiwov1alpha1.KaiwoServiceStatus,
    downloadJob  *batchv1.Job,
) (*kaiwov1alpha1.KaiwoServiceStatus, error) {

    svc := r.Object
    currentStatus := previousStatus.DeepCopy()

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

    var status kaiwov1alpha1.Status

	if currentStatus.StartTime == nil {
		if startTime := getEarliestPodStartTime(ctx, k8sClient, kaiwoJob); startTime != nil {
			currentStatus.StartTime = startTime
		}
	}

    if svc.Spec.IsRayService() {
        var rayService rayv1.RayService
        err := k8sClient.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &rayService)
        if err == nil {
            if rayService.Status.ServiceStatus == rayv1.RayServiceStatusHealthy {
                currentStatus.Phase = kaiwov1alpha1.KaiwoServicePhaseRunning
            } else {
                currentStatus.Phase = kaiwov1alpha1.KaiwoServicePhasePending
            }
        }
    } else {
        var dep appsv1.Deployment
        err := k8sClient.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &dep)
        if err == nil {
            if dep.Status.ReadyReplicas == dep.Status.Replicas {
                currentStatus.Phase = kaiwov1alpha1.KaiwoServicePhaseRunning
            } else {
                currentStatus.Phase = kaiwov1alpha1.KaiwoServicePhasePending
            }
        }
    }

    return currentStatus, nil
}
