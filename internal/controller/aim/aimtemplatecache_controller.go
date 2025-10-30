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
	"fmt"
	"maps"
	"slices"

	"k8s.io/client-go/tools/record"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AIMModelCacheReconciler reconciles a AIMModelCache object
type AIMTemplateCacheReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// RBAC markers
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimtemplatecaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimtemplatecaches/status,verbs=get;update;patch
func (r *AIMTemplateCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Fetch CR
	var tc aimv1alpha1.AIMTemplateCache
	if err := r.Get(ctx, req.NamespacedName, &tc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMTemplateCache, aimv1alpha1.AIMTemplateCacheStatus]{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Object:   &tc,
		Recorder: r.Recorder,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &tc)
		},
		FieldOwner: modelCacheFieldOwner,
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var o *templateCacheObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*templateCacheObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &tc, o)
		},
		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *templateCacheObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*templateCacheObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &tc, o, errs)
		},
		FinalizeFn: nil,
	})

}

// observation holds read-only snapshot of dependent resources and derived flags
type templateCacheObservation struct {
	AllCachesAvailable bool
	MissingCaches      []aimv1alpha1.AIMModelSource
	CacheStatus        map[string]aimv1alpha1.AIMModelCacheStatusEnum
}

func cmpModelCacheStatus(a aimv1alpha1.AIMModelCacheStatusEnum, b aimv1alpha1.AIMModelCacheStatusEnum) int {
	order := map[aimv1alpha1.AIMModelCacheStatusEnum]int{
		aimv1alpha1.AIMModelCacheStatusFailed:      0,
		aimv1alpha1.AIMModelCacheStatusPending:     1,
		aimv1alpha1.AIMModelCacheStatusProgressing: 2,
		aimv1alpha1.AIMModelCacheStatusAvailable:   3,
	}
	if order[a] > order[b] {
		return 1
	}
	return -1
}

func (r *AIMTemplateCacheReconciler) observe(ctx context.Context, tc *aimv1alpha1.AIMTemplateCache) (*templateCacheObservation, error) {
	var obs templateCacheObservation
	// Fetch available caches in the namespace
	var caches = aimv1alpha1.AIMModelCacheList{}
	if err := r.Client.List(ctx, &caches, client.InNamespace(tc.Namespace)); err != nil {
		return nil, err
	}

	obs.CacheStatus = map[string]aimv1alpha1.AIMModelCacheStatusEnum{}

	// Loop through our needed model sources and check with what's available in our namespace.
	// If multiple are available, select the one
	for _, model := range tc.Spec.ModelSources {
		bestStatus := aimv1alpha1.AIMModelCacheStatusPending
		for _, cached := range caches.Items {

			// ModelCache is a match if it has the same SourceURI and a StorageClass matching our config
			if cached.Spec.SourceURI == model.SourceURI &&
				(tc.Spec.StorageClassName == "" || tc.Spec.StorageClassName == cached.Spec.StorageClassName) {
				if cmpModelCacheStatus(bestStatus, cached.Status.Status) < 0 {
					bestStatus = cached.Status.Status
				}
			}
		}
		obs.CacheStatus[model.Name] = bestStatus
		if bestStatus == aimv1alpha1.AIMModelCacheStatusPending {
			obs.MissingCaches = append(obs.MissingCaches, model)
		}

	}

	return &obs, nil
}

func BuildMissingModelCaches(tc *aimv1alpha1.AIMTemplateCache, obs *templateCacheObservation) (caches []*aimv1alpha1.AIMModelCache) {
	for _, cache := range obs.MissingCaches {
		caches = append(caches,
			&aimv1alpha1.AIMModelCache{
				TypeMeta: metav1.TypeMeta{APIVersion: "aimv1alpha1", Kind: "AIMModelCache"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cache.Name + "-cache",
					Namespace: tc.Namespace,
					Labels:    map[string]string{"template-created": "true"}, // Can be cleaned up if no templates are referencing it
				},
				Spec: aimv1alpha1.AIMModelCacheSpec{
					StorageClassName: tc.Spec.StorageClassName,
					SourceURI:        cache.SourceURI,
					Size:             cache.Size,
					Env:              tc.Spec.Env,
				},
			},
		)

	}
	return
}

func (r *AIMTemplateCacheReconciler) plan(_ context.Context, tc *aimv1alpha1.AIMTemplateCache, obs *templateCacheObservation) (desired []client.Object, err error) {

	for _, mc := range BuildMissingModelCaches(tc, obs) {
		desired = append(desired, mc)
	}

	return
}

func (r *AIMTemplateCacheReconciler) projectStatus(_ context.Context, tc *aimv1alpha1.AIMTemplateCache, obs *templateCacheObservation, errs controllerutils.ReconcileErrors) (err error) {
	var conditions []metav1.Condition

	//Report any outstanding errors to report from previous controller actions
	if errs.HasError() {

		if errs.ObserveErr != nil {
			conditions = append(conditions, controllerutils.NewCondition(
				controllerutils.ConditionTypeFailure,
				metav1.ConditionTrue,
				controllerutils.ReasonFailed,
				fmt.Sprintf("We have observation errors: %v", errs.ObserveErr),
			))
		}

		if errs.ApplyErr != nil {
			conditions = append(conditions, controllerutils.NewCondition(
				controllerutils.ConditionTypeFailure,
				metav1.ConditionTrue,
				controllerutils.ReasonFailed,
				fmt.Sprintf("Apply failed: %v", errs.ApplyErr),
			))
		}
	}

	statusValues := slices.Collect(maps.Values(obs.CacheStatus))
	worstCacheStatus := slices.MaxFunc(statusValues, cmpModelCacheStatus)
	tc.Status.Status = aimv1alpha1.AIMTemplateCacheStatusEnum(worstCacheStatus)
	for _, cond := range conditions {
		meta.SetStatusCondition(&tc.Status.Conditions, cond)
	}

	if obs.AllCachesAvailable {
		tc.Status.Status = aimv1alpha1.AIMTemplateCacheStatusAvailable
	}

	return
}

func requestsFromTemplateCaches(templateCaches []aimv1alpha1.AIMTemplateCache) []reconcile.Request {
	if len(templateCaches) == 0 {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(templateCaches))
	for _, tc := range templateCaches {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: tc.Namespace,
				Name:      tc.Name,
			},
		})
	}
	return requests
}

func (r *AIMTemplateCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {

	modelCacheHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		modelCache, ok := obj.(*aimv1alpha1.AIMModelCache)
		if !ok {
			return nil
		}

		var templateCaches aimv1alpha1.AIMTemplateCacheList
		if err := r.List(ctx, &templateCaches,
			client.InNamespace(modelCache.Namespace),
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMTemplateCaches for AIMModelCaches",
				"runtimeConfig", modelCache.Name, "namespace", modelCache.Namespace)
			return nil
		}

		return requestsFromTemplateCaches(templateCaches.Items)
	})

	r.Recorder = mgr.GetEventRecorderFor("aimtemplatecache-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMTemplateCache{}).
		Watches(&aimv1alpha1.AIMModelCache{}, modelCacheHandler).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Named("aimtemplatecache-controller").
		Complete(r)
}
