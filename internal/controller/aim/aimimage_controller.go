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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
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
	imageFieldOwner              = "aim-image-controller"
	imageRuntimeConfigIndexKey   = ".spec.runtimeConfigName"
	clusterImageRuntimeConfigKey = ".spec.runtimeConfigName"
)

// AIMImageReconciler reconciles namespace-scoped AIMImage objects.
type AIMImageReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches/status,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AIMImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var image aimv1alpha1.AIMImage
	if err := r.Get(ctx, req.NamespacedName, &image); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMImage", "namespace", image.Namespace, "name", image.Name)

	return framework2.Reconcile(ctx, framework2.ReconcileSpec[*aimv1alpha1.AIMImage, aimv1alpha1.AIMImageStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &image,
		Recorder:   r.Recorder,
		FieldOwner: imageFieldOwner,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &image)
		},
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			imageObs, _ := obs.(*imageObservation)
			return r.plan(ctx, &image, imageObs)
		},
		ProjectFn: func(ctx context.Context, obs any, errs framework2.ReconcileErrors) error {
			imageObs, _ := obs.(*imageObservation)
			return r.projectStatus(ctx, &image, imageObs, errs)
		},
	})
}

type imageObservation struct {
	RuntimeConfig *shared.RuntimeConfigResolution
	CacheEnabled  bool
	CacheName     string
}

func (r *AIMImageReconciler) observe(ctx context.Context, image *aimv1alpha1.AIMImage) (*imageObservation, error) {
	logger := log.FromContext(ctx)
	configName := shared.NormalizeRuntimeConfigName(image.Spec.RuntimeConfigName)
	resolution, err := shared.ResolveRuntimeConfig(ctx, r.Client, image.Namespace, configName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve runtime config %q: %w", configName, err)
	}

	obs := &imageObservation{
		RuntimeConfig: resolution,
	}

	if resolution.NamespaceConfig == nil && resolution.ClusterConfig == nil {
		if configName != shared.DefaultRuntimeConfigName {
			framework2.EmitWarningEvent(r.Recorder, image, "RuntimeConfigNotFound",
				fmt.Sprintf("AIMRuntimeConfig %q not found", configName))
			return nil, fmt.Errorf("AIMRuntimeConfig %q not found", configName)
		}

		logger.Info("Default AIMRuntimeConfig not found, proceeding without overrides")
		framework2.EmitWarningEvent(r.Recorder, image, "DefaultRuntimeConfigNotFound",
			"Default AIMRuntimeConfig not found, proceeding with defaults")
	} else {
		framework2.EmitNormalEvent(r.Recorder, image, "RuntimeConfigResolved",
			fmt.Sprintf("Resolved runtime config %q", configName))
	}

	cacheEnabled := isCacheEnabled(resolution.EffectiveSpec)
	obs.CacheEnabled = cacheEnabled
	if cacheEnabled {
		obs.CacheName = shared.BaseImageCacheName(image.Spec.Image)
	}

	return obs, nil
}

func (r *AIMImageReconciler) plan(_ context.Context, image *aimv1alpha1.AIMImage, obs *imageObservation) ([]client.Object, error) {
	if obs == nil || !obs.CacheEnabled {
		return nil, nil
	}

	cache := buildBaseImageCache(image.Spec.Image, obs.CacheName, obs.RuntimeConfig.EffectiveSpec)
	return []client.Object{cache}, nil
}

func (r *AIMImageReconciler) projectStatus(_ context.Context, image *aimv1alpha1.AIMImage, obs *imageObservation, errs framework2.ReconcileErrors) error {
	imageStatus := &image.Status

	if obs != nil && obs.RuntimeConfig != nil {
		imageStatus.EffectiveRuntimeConfig = obs.RuntimeConfig.EffectiveStatus
	} else {
		imageStatus.EffectiveRuntimeConfig = nil
	}

	var condition metav1.Condition

	if errs.ObserveErr != nil {
		condition = framework2.NewCondition(
			aimv1alpha1.AIMImageConditionRuntimeResolved,
			metav1.ConditionFalse,
			aimv1alpha1.AIMImageReasonRuntimeConfigMissing,
			errs.ObserveErr.Error(),
		)
	} else if obs != nil && obs.RuntimeConfig != nil && obs.RuntimeConfig.NamespaceConfig == nil && obs.RuntimeConfig.ClusterConfig == nil && shared.NormalizeRuntimeConfigName(image.Spec.RuntimeConfigName) == shared.DefaultRuntimeConfigName {
		condition = framework2.NewCondition(
			aimv1alpha1.AIMImageConditionRuntimeResolved,
			metav1.ConditionFalse,
			aimv1alpha1.AIMImageReasonDefaultRuntimeConfigMissing,
			"Default AIMRuntimeConfig not found; proceeding with defaults",
		)
	} else {
		condition = framework2.NewCondition(
			aimv1alpha1.AIMImageConditionRuntimeResolved,
			metav1.ConditionTrue,
			aimv1alpha1.AIMImageReasonRuntimeResolved,
			"Runtime configuration resolved",
		)
	}

	meta.SetStatusCondition(&imageStatus.Conditions, condition)
	return nil
}

func (r *AIMImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	r.Recorder = mgr.GetEventRecorderFor("aim-image-controller")

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMImage{}, imageRuntimeConfigIndexKey, func(obj client.Object) []string {
		image, ok := obj.(*aimv1alpha1.AIMImage)
		if !ok {
			return nil
		}
		return []string{shared.NormalizeRuntimeConfigName(image.Spec.RuntimeConfigName)}
	}); err != nil {
		return err
	}

	runtimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		runtimeConfig, ok := obj.(*aimv1alpha1.AIMRuntimeConfig)
		if !ok {
			return nil
		}

		var images aimv1alpha1.AIMImageList
		if err := r.List(ctx, &images,
			client.InNamespace(runtimeConfig.Namespace),
			client.MatchingFields{
				imageRuntimeConfigIndexKey: shared.NormalizeRuntimeConfigName(runtimeConfig.Name),
			}); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMImage for AIMRuntimeConfig",
				"runtimeConfig", runtimeConfig.Name, "namespace", runtimeConfig.Namespace)
			return nil
		}

		return requestsFromImages(images.Items)
	})

	clusterRuntimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterConfig, ok := obj.(*aimv1alpha1.AIMClusterRuntimeConfig)
		if !ok {
			return nil
		}

		var images aimv1alpha1.AIMImageList
		if err := r.List(ctx, &images,
			client.MatchingFields{
				imageRuntimeConfigIndexKey: shared.NormalizeRuntimeConfigName(clusterConfig.Name),
			}); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMImage for AIMClusterRuntimeConfig",
				"runtimeConfig", clusterConfig.Name)
			return nil
		}

		return requestsFromImages(images.Items)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMImage{}).
		Watches(&aimv1alpha1.AIMRuntimeConfig{}, runtimeConfigHandler).
		Watches(&aimv1alpha1.AIMClusterRuntimeConfig{}, clusterRuntimeConfigHandler).
		Named("aim-image").
		Complete(r)
}

// AIMClusterImageReconciler reconciles cluster-scoped AIMClusterImage objects.
type AIMClusterImageReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimbaseimagecaches/status,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AIMClusterImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var image aimv1alpha1.AIMClusterImage
	if err := r.Get(ctx, req.NamespacedName, &image); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMClusterImage", "name", image.Name)

	return framework2.Reconcile(ctx, framework2.ReconcileSpec[*aimv1alpha1.AIMClusterImage, aimv1alpha1.AIMImageStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &image,
		Recorder:   r.Recorder,
		FieldOwner: imageFieldOwner,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &image)
		},
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			clusterObs, _ := obs.(*imageObservation)
			return r.plan(ctx, &image, clusterObs)
		},
		ProjectFn: func(ctx context.Context, obs any, errs framework2.ReconcileErrors) error {
			clusterObs, _ := obs.(*imageObservation)
			return r.projectStatus(ctx, &image, clusterObs, errs)
		},
	})
}

func (r *AIMClusterImageReconciler) observe(ctx context.Context, image *aimv1alpha1.AIMClusterImage) (*imageObservation, error) {
	logger := log.FromContext(ctx)
	operatorNamespace := shared.GetOperatorNamespace()
	configName := shared.NormalizeRuntimeConfigName(image.Spec.RuntimeConfigName)

	resolution, err := shared.ResolveRuntimeConfig(ctx, r.Client, operatorNamespace, configName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve runtime config %q: %w", configName, err)
	}

	obs := &imageObservation{
		RuntimeConfig: resolution,
	}

	if resolution.NamespaceConfig == nil && resolution.ClusterConfig == nil {
		if configName != shared.DefaultRuntimeConfigName {
			framework2.EmitWarningEvent(r.Recorder, image, "RuntimeConfigNotFound",
				fmt.Sprintf("AIMRuntimeConfig %q not found in operator namespace %q", configName, operatorNamespace))
			return nil, fmt.Errorf("AIMRuntimeConfig %q not found", configName)
		}

		logger.Info("Default AIMRuntimeConfig not found for cluster image, proceeding without overrides")
		framework2.EmitWarningEvent(r.Recorder, image, "DefaultRuntimeConfigNotFound",
			"Default AIMRuntimeConfig not found, proceeding with defaults")
	} else {
		framework2.EmitNormalEvent(r.Recorder, image, "RuntimeConfigResolved",
			fmt.Sprintf("Resolved runtime config %q", configName))
	}

	cacheEnabled := isCacheEnabled(resolution.EffectiveSpec)
	obs.CacheEnabled = cacheEnabled
	if cacheEnabled {
		obs.CacheName = shared.BaseImageCacheName(image.Spec.Image)
	}

	return obs, nil
}

func (r *AIMClusterImageReconciler) plan(_ context.Context, image *aimv1alpha1.AIMClusterImage, obs *imageObservation) ([]client.Object, error) {
	if obs == nil || !obs.CacheEnabled {
		return nil, nil
	}

	cache := buildBaseImageCache(image.Spec.Image, obs.CacheName, obs.RuntimeConfig.EffectiveSpec)
	return []client.Object{cache}, nil
}

func (r *AIMClusterImageReconciler) projectStatus(_ context.Context, image *aimv1alpha1.AIMClusterImage, obs *imageObservation, errs framework2.ReconcileErrors) error {
	imageStatus := &image.Status

	if obs != nil && obs.RuntimeConfig != nil {
		imageStatus.EffectiveRuntimeConfig = obs.RuntimeConfig.EffectiveStatus
	} else {
		imageStatus.EffectiveRuntimeConfig = nil
	}

	var condition metav1.Condition

	if errs.ObserveErr != nil {
		condition = framework2.NewCondition(
			aimv1alpha1.AIMImageConditionRuntimeResolved,
			metav1.ConditionFalse,
			aimv1alpha1.AIMImageReasonRuntimeConfigMissing,
			errs.ObserveErr.Error(),
		)
	} else if obs != nil && obs.RuntimeConfig != nil && obs.RuntimeConfig.NamespaceConfig == nil && obs.RuntimeConfig.ClusterConfig == nil && shared.NormalizeRuntimeConfigName(image.Spec.RuntimeConfigName) == shared.DefaultRuntimeConfigName {
		condition = framework2.NewCondition(
			aimv1alpha1.AIMImageConditionRuntimeResolved,
			metav1.ConditionFalse,
			aimv1alpha1.AIMImageReasonDefaultRuntimeConfigMissing,
			"Default AIMRuntimeConfig not found; proceeding with defaults",
		)
	} else {
		condition = framework2.NewCondition(
			aimv1alpha1.AIMImageConditionRuntimeResolved,
			metav1.ConditionTrue,
			aimv1alpha1.AIMImageReasonRuntimeResolved,
			"Runtime configuration resolved",
		)
	}

	meta.SetStatusCondition(&imageStatus.Conditions, condition)
	return nil
}

func (r *AIMClusterImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	r.Recorder = mgr.GetEventRecorderFor("aim-cluster-image-controller")

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMClusterImage{}, clusterImageRuntimeConfigKey, func(obj client.Object) []string {
		image, ok := obj.(*aimv1alpha1.AIMClusterImage)
		if !ok {
			return nil
		}
		return []string{shared.NormalizeRuntimeConfigName(image.Spec.RuntimeConfigName)}
	}); err != nil {
		return err
	}

	clusterRuntimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterConfig, ok := obj.(*aimv1alpha1.AIMClusterRuntimeConfig)
		if !ok {
			return nil
		}

		var images aimv1alpha1.AIMClusterImageList
		if err := r.List(ctx, &images,
			client.MatchingFields{
				clusterImageRuntimeConfigKey: shared.NormalizeRuntimeConfigName(clusterConfig.Name),
			}); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMClusterImage for AIMClusterRuntimeConfig",
				"runtimeConfig", clusterConfig.Name)
			return nil
		}

		return requestsFromClusterImages(images.Items)
	})

	runtimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		runtimeConfig, ok := obj.(*aimv1alpha1.AIMRuntimeConfig)
		if !ok {
			return nil
		}

		operatorNamespace := shared.GetOperatorNamespace()
		if runtimeConfig.Namespace != operatorNamespace {
			return nil
		}

		var images aimv1alpha1.AIMClusterImageList
		if err := r.List(ctx, &images,
			client.MatchingFields{
				clusterImageRuntimeConfigKey: shared.NormalizeRuntimeConfigName(runtimeConfig.Name),
			}); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMClusterImage for AIMRuntimeConfig",
				"runtimeConfig", runtimeConfig.Name)
			return nil
		}

		return requestsFromClusterImages(images.Items)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterImage{}).
		Watches(&aimv1alpha1.AIMClusterRuntimeConfig{}, clusterRuntimeConfigHandler).
		Watches(&aimv1alpha1.AIMRuntimeConfig{}, runtimeConfigHandler).
		Named("aim-cluster-image").
		Complete(r)
}

func isCacheEnabled(spec aimv1alpha1.AIMRuntimeConfigSpec) bool {
	return spec.CacheBaseImages != nil && ptr.Deref(spec.CacheBaseImages, false)
}

func requestsFromImages(images []aimv1alpha1.AIMImage) []reconcile.Request {
	if len(images) == 0 {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(images))
	for _, img := range images {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: img.Namespace,
				Name:      img.Name,
			},
		})
	}
	return requests
}

func requestsFromClusterImages(images []aimv1alpha1.AIMClusterImage) []reconcile.Request {
	if len(images) == 0 {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(images))
	for _, img := range images {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: img.Name,
			},
		})
	}
	return requests
}

func buildBaseImageCache(imageName, cacheName string, runtimeSpec aimv1alpha1.AIMRuntimeConfigSpec) *aimv1alpha1.AIMBaseImageCache {
	cache := &aimv1alpha1.AIMBaseImageCache{
		TypeMeta: metav1.TypeMeta{
			APIVersion: aimv1alpha1.GroupVersion.String(),
			Kind:       "AIMBaseImageCache",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: cacheName,
		},
	}

	cache.Spec.Image = imageName
	cache.Spec.ServiceAccountName = runtimeSpec.ServiceAccountName
	if len(runtimeSpec.ImagePullSecrets) > 0 {
		cache.Spec.ImagePullSecrets = append([]corev1.LocalObjectReference(nil), runtimeSpec.ImagePullSecrets...)
	}

	return cache
}
