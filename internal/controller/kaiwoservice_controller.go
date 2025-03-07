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

    rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
    kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
    "github.com/silogen/kaiwo/pkg/utils"
    workloadservice "github.com/silogen/kaiwo/pkg/workloads/service"
    "k8s.io/client-go/tools/record"
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
    utils.Debug(logger, "Running KaiwoService reconciliation")

    // Fetch the KaiwoService instance
    var kaiwoService kaiwov1alpha1.KaiwoService
    if err := r.Get(ctx, req.NamespacedName, &kaiwoService); err != nil {
        // If not found, or truly missing, just return
        if client.IgnoreNotFound(err) != nil {
            return ctrl.Result{}, fmt.Errorf("failed to get KaiwoService: %w", err)
        }
        utils.Debug(logger, "KaiwoService resource %s/%s not found", req.Namespace, req.Name)
        return ctrl.Result{}, nil
    }

    // If you want to skip if the service is in some terminal status, you could do so here.
    // But you mentioned you "aren't interested in completion," so presumably no skip.

    // Hand off to our "workloadservice" reconciler:
    reconciler := workloadservice.NewKaiwoServiceReconciler(&kaiwoService)

    // Perform the reconcile logic (no completion logic needed)
    result, err := reconciler.Reconcile(ctx, r.Client, r.Scheme, false /* dryRun */)
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
    // Create an event recorder for generating K8s events
    r.Recorder = mgr.GetEventRecorderFor("kaiwoservice-controller")

    return ctrl.NewControllerManagedBy(mgr).
        For(&kaiwov1alpha1.KaiwoService{}).
        // Watch Deployments in case they belong to our KaiwoService
        Watches(
            &appsv1.Deployment{},
            handler.EnqueueRequestsFromMapFunc(mapDeploymentToKaiwoService),
        ).
        // Watch RayServices in case they belong to our KaiwoService
        Watches(
            &rayv1.RayService{},
            handler.EnqueueRequestsFromMapFunc(mapRayServiceToKaiwoService),
        ).
        Named("kaiwoservice").
        Complete(r)
}

// mapDeploymentToKaiwoService extracts the KaiwoService name/namespace from a Deployment
func mapDeploymentToKaiwoService(obj client.Object) []reconcile.Request {
    dep, ok := obj.(*appsv1.Deployment)
    if !ok {
        return nil
    }
    // If you rely on them having the same name, or a label, or an OwnerReference, adapt accordingly:
    return []reconcile.Request{
        {
            NamespacedName: client.ObjectKey{
                Name:      dep.Name,
                Namespace: dep.Namespace,
            },
        },
    }
}

// mapRayServiceToKaiwoService extracts the KaiwoService name/namespace from a RayService
func mapRayServiceToKaiwoService(obj client.Object) []reconcile.Request {
    raySrv, ok := obj.(*rayv1.RayService)
    if !ok {
        return nil
    }
    // Similarly, adapt to your logic for mapping to the KaiwoService
    return []reconcile.Request{
        {
            NamespacedName: client.ObjectKey{
                Name:      raySrv.Name,
                Namespace: raySrv.Namespace,
            },
        },
    }
}

