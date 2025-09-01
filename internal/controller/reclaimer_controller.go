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
	"math/rand"
	"time"

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

func (r *ReclaimerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.reclaimer == nil {
		r.reclaimer = kueue.NewReclaimerLogic(r.Client)
	}

	outcome, err := r.reclaimer.Reclaim(ctx, req.Name)
	if err != nil {
		return ctrl.Result{RequeueAfter: jitterDuration(30*time.Second, 0.2)}, err
	}

	switch outcome {
	case kueue.Idle:
		return ctrl.Result{RequeueAfter: jitterDuration(2*time.Minute, 0.2)}, nil
	case kueue.PendingInsufficient:
		return ctrl.Result{RequeueAfter: jitterDuration(25*time.Second, 0.2)}, nil
	case kueue.EvictedOne:
		return ctrl.Result{RequeueAfter: jitterDuration(6*time.Second, 0.2)}, nil
	case kueue.Settling:
		return ctrl.Result{RequeueAfter: jitterDuration(6*time.Second, 0.2)}, nil
	case kueue.AdmittedOrChanged:
		return ctrl.Result{RequeueAfter: jitterDuration(25*time.Second, 0.2)}, nil
	default:
		return ctrl.Result{RequeueAfter: jitterDuration(45*time.Second, 0.2)}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReclaimerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kueuev1beta1.ClusterQueue{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

// Helper functions

// jitterDuration applies +/- factor jitter to duration
func jitterDuration(d time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return d
	}
	// rand seeded per process; intentional small jitter
	delta := (rand.Float64()*2 - 1) * factor // [-factor, +factor]
	return time.Duration(float64(d) * (1 + delta))
}
