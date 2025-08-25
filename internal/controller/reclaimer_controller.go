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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=clusterqueues,verbs=get;list;watch
// +kubebuilder:rbac:groups=visibility.kueue.x-k8s.io,resources=clusterqueues/pendingworkloads;localqueues/pendingworkloads,verbs=get;list;watch

func (r *ReclaimerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("ReclaimerReconciler").WithValues("clusterQueue", req.Name)

	// Initialize reclaimer if not already done
	if r.reclaimer == nil {
		r.reclaimer = kueue.NewReclaimerLogic(r.Client)
	}

	// Perform reclamation for this ClusterQueue
	if err := r.reclaimer.Reclaim(ctx, req.Name); err != nil {
		logger.Error(err, "Failed to perform reclamation")
		// Requeue with backoff for retries
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Requeue after a reasonable interval to check for new pending workloads
	// This ensures we periodically check even if no events trigger reconciliation
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReclaimerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch workloads to trigger reconciliation when workloads change state
	workloadPredicate := predicate.Funcs{
		// Only react to workloads that are pending or have the expired label
		CreateFunc: func(e event.CreateEvent) bool {
			wl, ok := e.Object.(*kueuev1beta1.Workload)
			if !ok {
				return false
			}
			return isWorkloadPending(wl) || isWorkloadExpired(wl)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWl, oldOk := e.ObjectOld.(*kueuev1beta1.Workload)
			newWl, newOk := e.ObjectNew.(*kueuev1beta1.Workload)
			if !oldOk || !newOk {
				return false
			}

			// React to state changes:
			// 1. Workload becomes pending
			// 2. Workload gets the expired label
			// 3. Admitted workload becomes inactive (evicted)
			oldPending := isWorkloadPending(oldWl)
			newPending := isWorkloadPending(newWl)
			oldExpired := isWorkloadExpired(oldWl)
			newExpired := isWorkloadExpired(newWl)
			oldActive := isWorkloadActive(oldWl)
			newActive := isWorkloadActive(newWl)

			return (!oldPending && newPending) || // became pending
				(!oldExpired && newExpired) || // became expired
				(oldActive && !newActive) // became inactive (evicted)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// React to workload deletion as it frees resources
			return true
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&kueuev1beta1.ClusterQueue{}).
		Watches(
			&kueuev1beta1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToClusterQueue),
			builder.WithPredicates(workloadPredicate),
		).
		Complete(r)
}

// mapWorkloadToClusterQueue maps a workload to its ClusterQueue for reconciliation
func (r *ReclaimerReconciler) mapWorkloadToClusterQueue(ctx context.Context, obj client.Object) []reconcile.Request {
	wl, ok := obj.(*kueuev1beta1.Workload)
	if !ok {
		return nil
	}

	// Extract ClusterQueue name from workload labels
	clusterQueueName := wl.Labels["kueue.x-k8s.io/queue-name"]
	if clusterQueueName == "" {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{Name: clusterQueueName},
		},
	}
}

// Helper functions

func isWorkloadPending(wl *kueuev1beta1.Workload) bool {
	for _, condition := range wl.Status.Conditions {
		if condition.Type == kueuev1beta1.WorkloadAdmitted {
			return condition.Status != "True"
		}
	}
	// If no admission condition exists, consider it pending
	return true
}

func isWorkloadExpired(wl *kueuev1beta1.Workload) bool {
	if wl.Labels == nil {
		return false
	}
	return wl.Labels["kaiwo.silogen.ai/expired"] == "true"
}

func isWorkloadActive(wl *kueuev1beta1.Workload) bool {
	return wl.Spec.Active == nil || *wl.Spec.Active
}