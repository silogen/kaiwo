/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"k8s.io/client-go/tools/record"

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
// +kubebuilder:rbac:groups=ray.io,resources=rayservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch

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
		Owns(&batchv1.Job{},
			builder.WithPredicates(JobStatusChangedPredicate()),
		).
		Owns(&appsv1.Deployment{}).
		Owns(&rayv1.RayService{}).
		Owns(&appwrapperv1beta2.AppWrapper{}).
		Watches(
			&kaiwo.KaiwoJob{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var services kaiwo.KaiwoServiceList
				if err := r.Client.List(ctx, &services); err != nil {
					return nil
				}
				var requests []reconcile.Request
				for _, svc := range services.Items {
					if svc.Spec.Duration != nil && svc.Status.Status == kaiwo.WorkloadStatusRunning {
						requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&svc)})
					}
				}
				return requests
			}),
		).
		Named("kaiwoservice").
		Complete(r)
}
