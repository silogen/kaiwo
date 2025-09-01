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

package controller

import (
	"context"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	config2 "github.com/silogen/kaiwo/pkg/runtime/config"

	"github.com/silogen/kaiwo/pkg/api"
	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/silogen/kaiwo/pkg/platform/kueue"
)

// ReclaimerReconciler reconciles workload reclamation per ClusterQueue
type ReclaimerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	reclaimer *kueue.ReclaimerLogic
}

// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=clusterqueues,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=visibility.kueue.x-k8s.io,resources=clusterqueues/pendingworkloads;localqueues/pendingworkloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=localqueues,verbs=get;list;watch
// Kaiwo CRD access (read + we may write status from the Reclaimer)
/// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoworkloads,verbs=get;list;watch
/// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoworkloads/status,verbs=get;update;patch

func (r *ReclaimerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.reclaimer == nil {
		r.reclaimer = kueue.NewReclaimerLogic(r.Client)
	}

	logger := log.FromContext(ctx)
	outcome, err := r.reclaimer.Reclaim(ctx, req.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch outcome {
	case kueue.PendingInsufficient:
		logger.Info("Insufficient expired workloads to evict")
	case kueue.Evicted:
		logger.Info("Evicted expired workloads")
	case kueue.Settling:
		baseutils.Debug(logger, "Reclaimer is settling")
	default:
	}
	return ctrl.Result{}, nil
}

func (r *ReclaimerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kueuev1beta1.ClusterQueue{}).
		// Workload -> ClusterQueue (react to pending/admitted/active flips)
		Watches(
			&kueuev1beta1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToClusterQueue()),
			builder.WithPredicates(workloadPredicates()),
		).
		// Kaiwo CR -> ClusterQueue (react to Expired/AdmissionPrep/Evicted flips)
		Watches(
			&v1alpha1.KaiwoJob{},
			handler.EnqueueRequestsFromMapFunc(r.mapKaiwoToClusterQueue()),
			builder.WithPredicates(kaiwoPredicates()),
		).
		Watches(
			&v1alpha1.KaiwoService{},
			handler.EnqueueRequestsFromMapFunc(r.mapKaiwoToClusterQueue()),
			builder.WithPredicates(kaiwoPredicates()),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func (r *ReclaimerReconciler) mapWorkloadToClusterQueue() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		wl, ok := obj.(*kueuev1beta1.Workload)
		if !ok {
			return nil
		}
		lqName := string(wl.Spec.QueueName)
		if lqName == "" {
			return nil
		}
		var lq kueuev1beta1.LocalQueue
		if err := r.Get(ctx, types.NamespacedName{Namespace: wl.Namespace, Name: lqName}, &lq); err != nil || lq.Spec.ClusterQueue == "" {
			return nil
		}
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: string(lq.Spec.ClusterQueue)}}}
	}
}

// Map Kaiwo -> its Workload (via Status.WorkloadRef) -> LocalQueue -> ClusterQueue
func (r *ReclaimerReconciler) mapKaiwoToClusterQueue() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		workload := objToKaiwoWorkload(obj)

		clusterQueueName := workload.GetCommonSpec().ClusterQueue
		if clusterQueueName == "" {
			ctx, _ = config2.GetContextWithConfig(ctx, r.Client)
			cfg := config2.ConfigFromContext(ctx)
			clusterQueueName = cfg.DefaultClusterQueueName
		}

		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: clusterQueueName}}}
	}
}

func objToKaiwoWorkload(obj client.Object) api.KaiwoWorkload {
	if kaiwoJob, ok := obj.(*v1alpha1.KaiwoJob); ok {
		return kaiwoJob
	} else if kaiwoService, ok := obj.(*v1alpha1.KaiwoService); ok {
		return kaiwoService
	} else {
		panic("Unknown object")
	}
}

func workloadPredicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			wl, ok := e.Object.(*kueuev1beta1.Workload)
			if !ok {
				return false
			}
			// New pending WL matters (might become HoL)
			return !isAdmitted(wl)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWl, ok1 := e.ObjectOld.(*kueuev1beta1.Workload)
			newWl, ok2 := e.ObjectNew.(*kueuev1beta1.Workload)
			if !ok1 || !ok2 {
				return false
			}
			// Admission flip (pending <-> admitted)
			if isAdmitted(oldWl) != isAdmitted(newWl) {
				return true
			}
			// Active flip (evicted, finished)
			if isActive(oldWl) != isActive(newWl) {
				return true
			}
			// We no longer key off workload labels/annotations
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool { return true },
	}
}

func kaiwoPredicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWorkload := objToKaiwoWorkload(e.ObjectOld)
			newWorkload := objToKaiwoWorkload(e.ObjectNew)

			oldConditions := oldWorkload.GetCommonStatusSpec().Conditions
			newConditions := newWorkload.GetCommonStatusSpec().Conditions
			// React when these conditions flip (TTL expiry or gating/eviction progress)
			return cond(oldConditions, "Expired") != cond(newConditions, "Expired") ||
				cond(oldConditions, "AdmissionPrep") != cond(newConditions, "AdmissionPrep") ||
				cond(oldConditions, "Evicted") != cond(newConditions, "Evicted")
		},
		DeleteFunc: func(e event.DeleteEvent) bool { return true },
	}
}

func cond(conds []metav1.Condition, t string) metav1.ConditionStatus {
	if c := meta.FindStatusCondition(conds, t); c != nil {
		return c.Status
	}
	return metav1.ConditionUnknown
}

func isAdmitted(wl *kueuev1beta1.Workload) bool {
	return meta.IsStatusConditionTrue(wl.Status.Conditions, kueuev1beta1.WorkloadAdmitted)
}

func isActive(wl *kueuev1beta1.Workload) bool {
	return wl.Spec.Active == nil || *wl.Spec.Active
}
