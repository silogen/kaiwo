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

	"k8s.io/apimachinery/pkg/api/equality"

	"sigs.k8s.io/controller-runtime/pkg/builder"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

	workloadHandler := workloadjob.NewKaiwoJobHandler(r.Scheme, &kaiwoJob)
	reconciler := common.Reconciler{
		WorkloadHandler: workloadHandler,
		StorageHandler:  common.NewStorageHandler(workloadHandler),
		Client:          r.Client,
		Scheme:          r.Scheme,
		Recorder:        r.Recorder,
	}

	if result, err := reconciler.Reconcile(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile KaiwoJob: %w", err)
	} else {
		return result, err
	}
}

// JobStatusChangedPredicate returns true only if Failed, Succeeded
// or the lengths of UncountedTerminatedPods.Failed / .Succeeded have changed.
func JobStatusChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJ := e.ObjectOld.(*batchv1.Job)
			newJ := e.ObjectNew.(*batchv1.Job)

			if oldJ.Status.Active != newJ.Status.Active {
				return true
			}

			if oldJ.Status.Ready != newJ.Status.Ready {
				return true
			}

			if !equality.Semantic.DeepEqual(oldJ.Status.Conditions, newJ.Status.Conditions) {
				return true
			}

			return false
		},
		// you can ignore Create/Delete/Generic if you only care about status‐updates
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

func (r *KaiwoJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("kaiwojob-controller")
	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}). // Catch status changes as well
		For(&kaiwo.KaiwoJob{}).
		Owns(&batchv1.Job{},
			builder.WithPredicates(JobStatusChangedPredicate()),
		).
		Owns(&rayv1.RayJob{}).
		Watches(
			&kaiwo.KaiwoService{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var jobs kaiwo.KaiwoJobList
				if err := r.Client.List(ctx, &jobs); err != nil {
					return nil
				}
				var requests []reconcile.Request
				for _, job := range jobs.Items {
					if job.Spec.Duration != nil && job.Status.Status == kaiwo.WorkloadStatusRunning {
						requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&job)})
					}
				}
				return requests
			}),
		).
		Named("kaiwojob").
		Complete(r)
}
