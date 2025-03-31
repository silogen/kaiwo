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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"k8s.io/client-go/tools/record"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	workloadservice "github.com/silogen/kaiwo/pkg/workloads/service"
)

// KaiwoServiceReconciler reconciles a KaiwoService object
type KaiwoServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoservices/finalizers,verbs=update

func (r *KaiwoServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	baseutils.Debug(logger, "Running KaiwoService reconciliation")

	var kaiwoService v1alpha1.KaiwoService
	if err := r.Get(ctx, req.NamespacedName, &kaiwoService); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get KaiwoService: %w", err)
		}
		baseutils.Debug(logger, "KaiwoService resource %s/%s not found", req.Namespace, req.Name)
		return ctrl.Result{}, nil
	}

	reconciler := workloadservice.NewKaiwoServiceReconciler(&kaiwoService)

	result, err := reconciler.Reconcile(ctx, r.Client, r.Scheme)
	if err != nil {
		r.Recorder.Eventf(
			&kaiwoService,
			corev1.EventTypeWarning,
			"KaiwoServiceReconcileError",
			"Failed to reconcile KaiwoService: %v", err,
		)
		return ctrl.Result{}, fmt.Errorf("failed to reconcile KaiwoService: %v", err)
	}
	return result, nil
}

func (r *KaiwoServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("kaiwoservice-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KaiwoService{}).
		Watches(
			&appsv1.Deployment{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return mapDeploymentToKaiwoService(obj)
			}),
		).
		Watches(
			&rayv1.RayService{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return mapRayServiceToKaiwoService(obj)
			}),
		).
		Watches(
			&appwrapperv1beta2.AppWrapper{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return mapAppWrapperToKaiwoService(obj)
			}),
		).
		Named("kaiwoservice").
		Complete(r)
}

func mapDeploymentToKaiwoService(obj client.Object) []reconcile.Request {
	dep, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil
	}
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      dep.Name,
				Namespace: dep.Namespace,
			},
		},
	}
}

func mapRayServiceToKaiwoService(obj client.Object) []reconcile.Request {
	raySrv, ok := obj.(*rayv1.RayService)
	if !ok {
		return nil
	}
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      raySrv.Name,
				Namespace: raySrv.Namespace,
			},
		},
	}
}

func mapAppWrapperToKaiwoService(obj client.Object) []reconcile.Request {
	appWrpr, ok := obj.(*appwrapperv1beta2.AppWrapper)
	if !ok {
		return nil
	}
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      appWrpr.Name,
				Namespace: appWrpr.Namespace,
			},
		},
	}
}
