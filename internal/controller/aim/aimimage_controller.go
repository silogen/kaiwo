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

package aim

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	framework "github.com/silogen/kaiwo/internal/controller/framework"
)

const (
	aimImageFieldOwner = "aim-image-controller"
)

// AIMImageReconciler reconciles AIMImage objects and manages base image caching
type AIMImageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

func (r *AIMImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AIMImage to ensure it exists (or was deleted)
	var image aimv1alpha1.AIMImage
	if err := r.Get(ctx, req.NamespacedName, &image); err != nil {
		if errors.IsNotFound(err) {
			// Image was deleted, still reconcile to update DaemonSet
			logger.Info("AIMImage deleted, reconciling image cache", "name", req.Name, "namespace", req.Namespace)
		} else {
			return ctrl.Result{}, err
		}
	} else {
		logger.Info("Reconciling AIMImage", "name", image.Name, "namespace", image.Namespace)
	}

	// Use framework orchestrator for observe → plan → apply
	return framework.ReconcileWithoutStatus(ctx, framework.ReconcileWithoutStatusSpec{
		Client:     r.Client,
		Scheme:     r.Scheme,
		FieldOwner: aimImageFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return shared.ObserveImageCaching(ctx, r.Client)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			o, _ := obs.(*shared.ImageCacheObservation)
			return shared.PlanImageCaching(o)
		},
	})
}

func (r *AIMImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMImage{}).
		Named("aim-image").
		Complete(r)
}
