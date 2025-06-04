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

	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
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

func (r *KaiwoServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("kaiwoservice-controller")

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}). // Catch status changes as well
		For(&kaiwo.KaiwoService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&rayv1.RayService{}).
		Owns(&appwrapperv1beta2.AppWrapper{}).
		Named("kaiwoservice").
		Complete(r)
}
