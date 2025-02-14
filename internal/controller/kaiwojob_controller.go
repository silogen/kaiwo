/*
Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// KaiwoJobReconciler reconciles a KaiwoJob object
type KaiwoJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/finalizers,verbs=update

func (r *KaiwoJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the KaiwoJob instance
	var kaiwoJob kaiwov1alpha1.KaiwoJob
	if err := r.Get(ctx, req.NamespacedName, &kaiwoJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get KaiwoJob")
			return ctrl.Result{}, err
		}
		logger.Info("Job does not exist, it might have been deleted")
		return ctrl.Result{}, nil
	}

	if err := controllerutils.ReconcileStorage(r.Client, r.Scheme, ctx, &kaiwoJob, &kaiwoJob.Spec.Storage); err != nil {
		return ctrl.Result{}, err
	}

	result, err := controllerutils.ReconcileDownloadJob(r.Client, r.Scheme, ctx, &kaiwoJob, &kaiwoJob.Spec.Storage)
	if err != nil {
		return ctrl.Result{}, err
	}
	// If the download job has not completed, a re-reconciliation request may be returned
	if result != nil {
		return *result, nil
	}

	if kaiwoJob.Spec.Labels == nil {
		kaiwoJob.Spec.Labels = make(map[string]string)
	}

	kaiwoJob.Spec.Labels[kaiwov1alpha1.UserLabel] = baseutils.SanitizeStringForKubernetes(kaiwoJob.Spec.User)
	if kaiwoJob.Spec.ClusterQueue == "" {
		kaiwoJob.Spec.Labels[kaiwov1alpha1.QueueLabel] = DefaultKaiwoQueueConfigName
	} else {
		kaiwoJob.Spec.Labels[kaiwov1alpha1.QueueLabel] = kaiwoJob.Spec.ClusterQueue
	}

	if err := controllerutils.CreateLocalQueue(ctx, r.Client, kaiwoJob.Spec.Labels[kaiwov1alpha1.QueueLabel], kaiwoJob.ObjectMeta.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	if kaiwoJob.Spec.RayJob == nil || !kaiwoJob.Spec.Ray {
		return r.reconcileK8sJob(ctx, &kaiwoJob)
	} else if kaiwoJob.Spec.RayJob != nil || kaiwoJob.Spec.Ray {
		return r.reconcileRayJob(ctx, &kaiwoJob)
	}

	jobInvalidErr := fmt.Errorf("KaiwoJob does not specify a valid Job or RayJob")
	logger.Error(jobInvalidErr, "KaiwoJob is misconfigured", "KaiwoJob", kaiwoJob.Name)
	return ctrl.Result{}, jobInvalidErr
}

func (r *KaiwoJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwov1alpha1.KaiwoJob{}).
		Named("kaiwojob").
		Complete(r)
}
