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
	"sort"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	framework2 "github.com/silogen/kaiwo/internal/controller/framework"
)

const (
	baseImageCacheFieldOwner = "aim-base-image-cache-controller"
)

// AIMBaseImageCacheReconciler manages AIMBaseImageCache resources.
type AIMBaseImageCacheReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AIMBaseImageCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cache aimv1alpha1.AIMBaseImageCache
	if err := r.Get(ctx, req.NamespacedName, &cache); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMBaseImageCache", "name", cache.Name)

	return framework2.Reconcile(ctx, framework2.ReconcileSpec[*aimv1alpha1.AIMBaseImageCache, aimv1alpha1.AIMBaseImageCacheStatus]{
		Client:        r.Client,
		Scheme:        r.Scheme,
		Object:        &cache,
		Recorder:      r.Recorder,
		FieldOwner:    baseImageCacheFieldOwner,
		FinalizerName: shared.BaseImageCacheFinalizer,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &cache)
		},
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			cacheObs, _ := obs.(*baseImageCacheObservation)
			return r.plan(ctx, &cache, cacheObs)
		},
		ProjectFn: func(ctx context.Context, obs any, errs framework2.ReconcileErrors) error {
			cacheObs, _ := obs.(*baseImageCacheObservation)
			return r.projectStatus(ctx, &cache, cacheObs, errs)
		},
		FinalizeFn: func(ctx context.Context, obs any) error {
			cacheObs, _ := obs.(*baseImageCacheObservation)
			return r.finalize(ctx, cacheObs)
		},
	})
}

type baseImageCacheObservation struct {
	DaemonSet         *appsv1.DaemonSet
	References        []aimv1alpha1.AIMBaseImageCacheReference
	OperatorNamespace string
}

func (r *AIMBaseImageCacheReconciler) observe(ctx context.Context, cache *aimv1alpha1.AIMBaseImageCache) (*baseImageCacheObservation, error) {
	logger := log.FromContext(ctx)
	obs := &baseImageCacheObservation{
		OperatorNamespace: shared.GetOperatorNamespace(),
	}

	daemonSet := &appsv1.DaemonSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: obs.OperatorNamespace}, daemonSet); err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get DaemonSet %q: %w", cache.Name, err)
		}
	} else {
		obs.DaemonSet = daemonSet
	}

	refs, err := r.resolveReferences(ctx, cache, logger)
	if err != nil {
		return nil, err
	}
	obs.References = refs

	return obs, nil
}

func (r *AIMBaseImageCacheReconciler) plan(ctx context.Context, cache *aimv1alpha1.AIMBaseImageCache, obs *baseImageCacheObservation) ([]client.Object, error) {
	if obs == nil {
		return nil, nil
	}

	if len(obs.References) == 0 {
		if obs.DaemonSet != nil {
			if err := r.Delete(ctx, obs.DaemonSet); err != nil && !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to delete DaemonSet %q: %w", obs.DaemonSet.Name, err)
			}
		}
		return nil, nil
	}

	ds := shared.BuildBaseImageCacheDaemonSet(cache, obs.OperatorNamespace)
	return []client.Object{ds}, nil
}

func (r *AIMBaseImageCacheReconciler) projectStatus(_ context.Context, cache *aimv1alpha1.AIMBaseImageCache, obs *baseImageCacheObservation, errs framework2.ReconcileErrors) error {
	status := &cache.Status

	if obs != nil {
		status.RefCount = int32(len(obs.References))
		status.Refs = obs.References

		if obs.DaemonSet != nil {
			status.NodesTotal = obs.DaemonSet.Status.DesiredNumberScheduled
			status.NodesCached = obs.DaemonSet.Status.NumberReady
			status.NodesFailed = obs.DaemonSet.Status.NumberUnavailable
		} else {
			status.NodesTotal = 0
			status.NodesCached = 0
			status.NodesFailed = 0
		}
	}

	readyCondition := framework2.NewCondition(framework2.ConditionTypeReady, metav1.ConditionFalse, "Idle", "No DaemonSet present")
	progressingCondition := framework2.NewCondition(framework2.ConditionTypeProgressing, metav1.ConditionFalse, "Idle", "Cache is idle")
	failureCondition := framework2.NewCondition(framework2.ConditionTypeFailure, metav1.ConditionFalse, "NoFailure", "")

	if errs.ObserveErr != nil {
		failureCondition = framework2.NewCondition(framework2.ConditionTypeFailure, metav1.ConditionTrue, "ObservationFailed", errs.ObserveErr.Error())
		meta.SetStatusCondition(&status.Conditions, failureCondition)
		meta.SetStatusCondition(&status.Conditions, readyCondition)
		meta.SetStatusCondition(&status.Conditions, progressingCondition)
		return nil
	}

	if obs != nil && len(obs.References) == 0 {
		readyCondition = framework2.NewCondition(framework2.ConditionTypeReady, metav1.ConditionFalse, "NoReferences", "No resources reference this cache")
		progressingCondition = framework2.NewCondition(framework2.ConditionTypeProgressing, metav1.ConditionFalse, "NoReferences", "Cache not required")
		meta.SetStatusCondition(&status.Conditions, readyCondition)
		meta.SetStatusCondition(&status.Conditions, progressingCondition)
		meta.SetStatusCondition(&status.Conditions, framework2.NewCondition(framework2.ConditionTypeFailure, metav1.ConditionFalse, "NoFailure", ""))
		return nil
	}

	if obs != nil && obs.DaemonSet != nil {
		dsStatus := obs.DaemonSet.Status
		if dsStatus.DesiredNumberScheduled > 0 && dsStatus.NumberReady == dsStatus.DesiredNumberScheduled {
			readyCondition = framework2.NewCondition(framework2.ConditionTypeReady, metav1.ConditionTrue, "DaemonSetReady", "Base image cached on all targeted nodes")
			progressingCondition = framework2.NewCondition(framework2.ConditionTypeProgressing, metav1.ConditionFalse, "DaemonSetReady", "Cache is ready")
		} else {
			readyCondition = framework2.NewCondition(framework2.ConditionTypeReady, metav1.ConditionFalse, "DaemonSetProgressing", "DaemonSet is warming up")
			progressingCondition = framework2.NewCondition(framework2.ConditionTypeProgressing, metav1.ConditionTrue, "DaemonSetProgressing", "DaemonSet is warming up")
		}
	}

	meta.SetStatusCondition(&status.Conditions, readyCondition)
	meta.SetStatusCondition(&status.Conditions, progressingCondition)
	meta.SetStatusCondition(&status.Conditions, failureCondition)
	return nil
}

func (r *AIMBaseImageCacheReconciler) finalize(ctx context.Context, obs *baseImageCacheObservation) error {
	if obs == nil || obs.DaemonSet == nil {
		return nil
	}
	if err := r.Delete(ctx, obs.DaemonSet); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete DaemonSet %q during finalization: %w", obs.DaemonSet.Name, err)
	}
	return nil
}

func (r *AIMBaseImageCacheReconciler) resolveReferences(ctx context.Context, cache *aimv1alpha1.AIMBaseImageCache, logger logr.Logger) ([]aimv1alpha1.AIMBaseImageCacheReference, error) {
	type configKey struct {
		Namespace string
		Name      string
	}

	cacheMap := make(map[configKey]*shared.RuntimeConfigResolution)
	resolve := func(namespace, name string) (*shared.RuntimeConfigResolution, error) {
		key := configKey{Namespace: namespace, Name: shared.NormalizeRuntimeConfigName(name)}
		if res, ok := cacheMap[key]; ok {
			return res, nil
		}
		res, err := shared.ResolveRuntimeConfig(ctx, r.Client, namespace, name)
		if err != nil {
			return nil, err
		}
		cacheMap[key] = res
		return res, nil
	}

	refs := make([]aimv1alpha1.AIMBaseImageCacheReference, 0)

	var images aimv1alpha1.AIMImageList
	if err := r.List(ctx, &images); err != nil {
		return nil, fmt.Errorf("failed to list AIMImage: %w", err)
	}

	for _, img := range images.Items {
		res, err := resolve(img.Namespace, img.Spec.RuntimeConfigName)
		if err != nil {
			logger.Error(err, "failed to resolve runtime config for AIMImage", "namespace", img.Namespace, "name", img.Name)
			continue
		}
		if res == nil || !isCacheEnabled(res.EffectiveSpec) {
			continue
		}
		if shared.BaseImageCacheName(img.Spec.Image) != cache.Name {
			continue
		}
		refs = append(refs, aimv1alpha1.AIMBaseImageCacheReference{
			Kind:      "AIMImage",
			Name:      img.Name,
			Namespace: img.Namespace,
			UID:       img.UID,
		})
	}

	var clusterImages aimv1alpha1.AIMClusterImageList
	if err := r.List(ctx, &clusterImages); err != nil {
		return nil, fmt.Errorf("failed to list AIMClusterImage: %w", err)
	}

	operatorNamespace := shared.GetOperatorNamespace()
	for _, img := range clusterImages.Items {
		res, err := resolve(operatorNamespace, img.Spec.RuntimeConfigName)
		if err != nil {
			logger.Error(err, "failed to resolve runtime config for AIMClusterImage", "name", img.Name)
			continue
		}
		if res == nil || !isCacheEnabled(res.EffectiveSpec) {
			continue
		}
		if shared.BaseImageCacheName(img.Spec.Image) != cache.Name {
			continue
		}
		refs = append(refs, aimv1alpha1.AIMBaseImageCacheReference{
			Kind: "AIMClusterImage",
			Name: img.Name,
			UID:  img.UID,
		})
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		if refs[i].Namespace != refs[j].Namespace {
			return refs[i].Namespace < refs[j].Namespace
		}
		return refs[i].Name < refs[j].Name
	})

	return refs, nil
}

func (r *AIMBaseImageCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("aim-base-image-cache-controller")

	imageHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		image, ok := obj.(*aimv1alpha1.AIMImage)
		if !ok {
			return nil
		}
		if image.Spec.Image == "" {
			return nil
		}
		name := shared.BaseImageCacheName(image.Spec.Image)
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name}}}
	})

	clusterImageHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		image, ok := obj.(*aimv1alpha1.AIMClusterImage)
		if !ok {
			return nil
		}
		if image.Spec.Image == "" {
			return nil
		}
		name := shared.BaseImageCacheName(image.Spec.Image)
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name}}}
	})

	daemonSetHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		daemonSet, ok := obj.(*appsv1.DaemonSet)
		if !ok {
			return nil
		}
		name := daemonSet.Labels[shared.BaseImageCacheLabel]
		if name == "" {
			name = daemonSet.Name
		}
		if name == "" {
			return nil
		}
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name}}}
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMBaseImageCache{}).
		Watches(&appsv1.DaemonSet{}, daemonSetHandler).
		Watches(&aimv1alpha1.AIMImage{}, imageHandler).
		Watches(&aimv1alpha1.AIMClusterImage{}, clusterImageHandler).
		Named("aim-base-image-cache").
		Complete(r)
}
