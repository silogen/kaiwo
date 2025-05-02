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
	"strings"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	workloadjob "github.com/silogen/kaiwo/pkg/workloads/job"
)

// KaiwoJobReconciler reconciles a KaiwoJob object
type KaiwoJobReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=config.kaiwo.silogen.ai,resources=kaiwoconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *KaiwoJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	baseutils.Debug(logger, "Running reconciliation")

	// Fetch the KaiwoJob instance
	var kaiwoJob kaiwo.KaiwoJob
	if err := r.Get(ctx, req.NamespacedName, &kaiwoJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, baseutils.LogErrorf(logger, "failed to get KaiwoJob", err)
		}
		baseutils.Debug(logger, "KaiwoJob resource %s/%s not found", kaiwoJob.Namespace, kaiwoJob.Name)
		return ctrl.Result{}, nil
	}

	if kaiwoJob.Status.Status == kaiwo.StatusFailed {
		baseutils.Debug(logger, "Skipping reconciliation, as status is failed")
		return ctrl.Result{}, nil
	} else if kaiwoJob.Status.Status == kaiwo.StatusComplete {
		baseutils.Debug(logger, "Skipping reconciliation, as status is complete")
		return ctrl.Result{}, nil
	}

	ctx, err := controllerutils.GetContextWithConfig(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch config: %w", err)
	}

	reconciler := workloadjob.NewKaiwoJobReconciler(ctx, &kaiwoJob)

	result, err := reconciler.Reconcile(ctx, r.Client, r.Scheme)
	if err != nil {
		r.Recorder.Eventf(
			&kaiwoJob,
			corev1.EventTypeWarning,
			"KaiwoJobReconcileError",
			"Failed to reconcile KaiwoJob: %v", err,
		)
		return ctrl.Result{}, fmt.Errorf("failed to run reconciliation: %v", err)
	}
	return result, nil
}

func (r *KaiwoJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("kaiwojob-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwo.KaiwoJob{}).
		Watches(
			&batchv1.Job{}, // Watching batchv1.Job
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return mapJobToKaiwoJob(obj)
			}),
		).
		Watches(
			&rayv1.RayJob{}, // Watching RayJob
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return mapRayJobToKaiwoJob(obj)
			}),
		).
		Named("kaiwojob").
		Complete(r)
}

func mapJobToKaiwoJob(obj client.Object) []reconcile.Request {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil
	}

	// If the job is a Download job, fetch the owning kaiwo job name
	name := job.Name
	if value, ok := job.Labels[common.KaiwoTypeLabel]; ok && value == workloadutils.KaiwoDownloadTypeLabelValue {
		name = strings.TrimSuffix(name, "-download")
	}

	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{
			// Map download jobs back to their owning Kaiwo Jobs
			Name:      name,
			Namespace: job.Namespace,
		}},
	}
}

func mapRayJobToKaiwoJob(obj client.Object) []reconcile.Request {
	rayJob, ok := obj.(*rayv1.RayJob)
	if !ok {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{
			Name:      rayJob.Name,
			Namespace: rayJob.Namespace,
		}},
	}
}
