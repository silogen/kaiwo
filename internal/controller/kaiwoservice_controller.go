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

	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
// +kubebuilder:rbac:groups=config.kaiwo.silogen.ai,resources=kaiwoconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *KaiwoServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	baseutils.Debug(logger, "Running KaiwoService reconciliation")

	var kaiwoService kaiwo.KaiwoService
	if err := r.Get(ctx, req.NamespacedName, &kaiwoService); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get KaiwoService: %w", err)
		}
		baseutils.Debug(logger, "KaiwoService resource %s/%s not found", req.Namespace, req.Name)
		return ctrl.Result{}, nil
	}

	workloadHandler := workloadservice.NewKaiwoServiceHandler(r.Scheme, &kaiwoService)
	reconciler := common.Reconciler{
		WorkloadHandler: workloadHandler,
		StorageHandler:  common.NewStorageHandler(workloadHandler),
		Client:          r.Client,
		Scheme:          r.Scheme,
		Recorder:        r.Recorder,
	}

	if result, err := reconciler.Reconcile(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile KaiwoService: %w", err)
	} else {
		return result, err
	}
}

func AppWrapperChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*appwrapperv1beta2.AppWrapper)
			newObj := e.ObjectNew.(*appwrapperv1beta2.AppWrapper)

			if !equality.Semantic.DeepEqual(oldObj.Status.Conditions, newObj.Status.Conditions) {
				return true
			}

			return oldObj.Status.Phase != newObj.Status.Phase
		},
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

func DeploymentChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*appsv1.Deployment)
			newObj := e.ObjectNew.(*appsv1.Deployment)

			if oldObj.Status.AvailableReplicas != newObj.Status.AvailableReplicas {
				return true
			}

			if !equality.Semantic.DeepEqual(oldObj.Status.Conditions, newObj.Status.Conditions) {
				return true
			}

			return false
		},
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

func (r *KaiwoServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("kaiwoservice-controller")

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}). // Catch status changes as well
		For(&kaiwo.KaiwoService{}).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(DeploymentChangedPredicate())).
		Owns(&rayv1.RayService{}).
		Owns(&appwrapperv1beta2.AppWrapper{}, builder.WithPredicates(AppWrapperChangedPredicate())).
		Owns(&batchv1.Job{}, builder.WithPredicates(JobStatusChangedPredicate())).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ConfigMap{}).
		Named("kaiwoservice").
		Complete(r)
}
