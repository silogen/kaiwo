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

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	namespaceTemplateFieldOwner            = "aim-namespace-template-controller"
	namespaceTemplateRuntimeConfigIndexKey = ".spec.runtimeConfigName"
)

// AIMServiceTemplateReconciler reconciles a AIMServiceTemplate object
type AIMServiceTemplateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *AIMServiceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the template
	var template aimv1alpha1.AIMServiceTemplate
	if err := r.Get(ctx, req.NamespacedName, &template); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling AIMServiceTemplate", "name", template.Name, "namespace", template.Namespace)

	// Use framework orchestrator with closures
	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMServiceTemplate, aimv1alpha1.AIMServiceTemplateStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &template,
		Recorder:   r.Recorder,
		FieldOwner: namespaceTemplateFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &template)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var o *namespaceTemplateObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*namespaceTemplateObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &template, o)
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *namespaceTemplateObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*namespaceTemplateObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &template, o, errs)
		},

		FinalizeFn: nil, // No external cleanup needed
	})
}

// namespaceTemplateObservation holds observed state
type namespaceTemplateObservation = shared.RuntimeObservation[*servingv1alpha1.ServingRuntime]

func requestsFromNamespaceTemplates(templates []aimv1alpha1.AIMServiceTemplate) []reconcile.Request {
	if len(templates) == 0 {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(templates))
	for _, tpl := range templates {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: tpl.Namespace,
				Name:      tpl.Name,
			},
		})
	}
	return requests
}

// observe gathers current cluster state (read-only)
func (r *AIMServiceTemplateReconciler) observe(ctx context.Context, template *aimv1alpha1.AIMServiceTemplate) (*namespaceTemplateObservation, error) {
	logger := log.FromContext(ctx)
	observation, err := shared.ObserveTemplate(ctx, shared.TemplateObservationOptions[*servingv1alpha1.ServingRuntime]{
		K8sClient: r.Client,
		GetRuntime: func(ctx context.Context) (*servingv1alpha1.ServingRuntime, error) {
			servingRuntime, err := shared.GetServingRuntime(ctx, r.Client, template.Namespace, template.Name)
			if err != nil && !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get ServingRuntime: %w", err)
			}
			return servingRuntime, err
		},
		ShouldCheckDiscoveryJob: len(template.Spec.ModelSources) == 0 &&
			template.Status.Status != aimv1alpha1.AIMTemplateStatusReady &&
			template.Status.Status != aimv1alpha1.AIMTemplateStatusNotAvailable,
		GetDiscoveryJob: func(ctx context.Context) (*batchv1.Job, error) {
			job, err := shared.GetDiscoveryJob(ctx, r.Client, template.Namespace, template.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to get discovery job: %w", err)
			}
			return job, nil
		},
		GetJobNamespace: func() string {
			return template.Namespace
		},
		LookupImage: func(ctx context.Context) (*shared.ImageLookupResult, error) {
			return shared.LookupImageForNamespaceTemplate(ctx, r.Client, template.Namespace, template.Spec.ModelName)
		},
		ResolveRuntimeConfig: func(ctx context.Context) (*shared.RuntimeConfigResolution, error) {
			resolution, err := shared.ResolveRuntimeConfig(ctx, r.Client, template.Namespace, template.Spec.RuntimeConfigName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve AIMRuntimeConfig %q: %w", template.Spec.RuntimeConfigName, err)
			}
			return resolution, nil
		},
		OnRuntimeConfigResolved: func(resolution *shared.RuntimeConfigResolution) {
			if resolution.NamespaceConfig == nil && resolution.ClusterConfig == nil && resolution.Name == shared.DefaultRuntimeConfigName {
				baseutils.Debug(logger, "Default AIMRuntimeConfig not found, proceeding without overrides")
				controllerutils.EmitWarningEvent(r.Recorder, template, "DefaultRuntimeConfigNotFound",
					"Default AIMRuntimeConfig not found, proceeding with controller defaults.")
				return
			}

			baseutils.Debug(logger, "Resolved AIMRuntimeConfig",
				"name", resolution.Name,
				"sources", shared.JoinRuntimeConfigSources(resolution, template.Namespace))

			controllerutils.EmitNormalEvent(r.Recorder, template, "RuntimeConfigResolved",
				fmt.Sprintf("Using AIMRuntimeConfig %q from %s", resolution.Name, shared.JoinRuntimeConfigSources(resolution, template.Namespace)))
		},
		GetImagePullSecrets: func() []corev1.LocalObjectReference {
			return template.Spec.ImagePullSecrets
		},
		GetServiceAccountName: func() string {
			return template.Spec.ServiceAccountName
		},
		GetTemplateCaches: func(ctx context.Context) (*aimv1alpha1.AIMTemplateCacheList, error) {
			var availableCaches aimv1alpha1.AIMTemplateCacheList
			if !template.Spec.Caching.Enabled {
				return nil, nil
			}
			availableCaches = aimv1alpha1.AIMTemplateCacheList{}
			err := r.Client.List(ctx, &availableCaches)
			if err != nil {
				return nil, err
			}
			return &availableCaches, nil
		},
	})
	if err != nil {
		return nil, err
	}

	if observation != nil {
		if err := shared.UpdateTemplateGPUAvailability(ctx, r.Client, template.Spec.AIMServiceTemplateSpecCommon, &observation.TemplateObservation); err != nil {
			return nil, fmt.Errorf("failed to validate GPU availability: %w", err)
		}
	}

	return observation, nil
}

// plan computes desired state (pure function)
func (r *AIMServiceTemplateReconciler) plan(ctx context.Context, template *aimv1alpha1.AIMServiceTemplate, obs *namespaceTemplateObservation) ([]client.Object, error) {
	var observation *shared.TemplateObservation
	if obs != nil {
		observation = &obs.TemplateObservation
	}

	desired := shared.PlanTemplateResources(shared.TemplatePlanContext{
		Ctx:         ctx,
		Client:      r.Client,
		Template:    template,
		APIVersion:  template.APIVersion,
		Kind:        template.Kind,
		Status:      template.Status.Status,
		Observation: observation,
	}, shared.TemplatePlanBuilders{
		BuildRuntime: func(input shared.TemplatePlanInput) client.Object {
			state := shared.BuildTemplateStateFromObservation(
				template.Name,
				template.Namespace,
				template.Spec.AIMServiceTemplateSpecCommon,
				input.Observation,
				input.RuntimeConfigSpec,
				template.Status.DeepCopy(),
			)
			return shared.BuildServingRuntimeFromState(state, input.OwnerReference)
		},
		BuildDiscoveryJob: func(input shared.TemplatePlanInput) client.Object {
			return shared.BuildDiscoveryJob(shared.DiscoveryJobSpec{
				TemplateName:     template.Name,
				Namespace:        template.Namespace,
				ModelID:          template.Spec.ModelName,
				Image:            input.Observation.Image,
				Env:              template.Spec.Env,
				ImagePullSecrets: input.Observation.ImagePullSecrets,
				ServiceAccount:   input.Observation.ServiceAccountName,
				OwnerRef:         input.OwnerReference,
				TemplateSpec:     template.Spec.AIMServiceTemplateSpecCommon,
			})
		},
	})

	if template.Spec.Caching.Enabled && len(template.Status.ModelSources) > 0 && obs.TemplateCaches != nil {
		cacheFound := false

		// Check if we already have a valid cache
		for _, cache := range obs.TemplateCaches.Items {
			for _, owner := range cache.OwnerReferences {
				if owner.UID == template.UID {
					cacheFound = true
				}
			}
		}

		//Caching is enabled, and didn't find ours - create one
		if !cacheFound {
			newCache := buildTemplateCache(template, obs.RuntimeConfig)
			desired = append(desired, newCache)
		}

	}

	return desired, nil
}

func buildTemplateCache(template *aimv1alpha1.AIMServiceTemplate, runtimeConfigResolution *shared.RuntimeConfigResolution) *aimv1alpha1.AIMTemplateCache {
	cBool := true
	return &aimv1alpha1.AIMTemplateCache{
		TypeMeta: metav1.TypeMeta{APIVersion: "aimv1alpha1", Kind: "AIMModelCache"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      template.Name + "-tc",
			Namespace: template.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: template.APIVersion,
					Kind:       template.Kind,
					Name:       template.Name,
					UID:        template.UID,
					Controller: &cBool,
				},
			},
		},
		Spec: aimv1alpha1.AIMTemplateCacheSpec{
			TemplateRef:      template.Name,
			StorageClassName: runtimeConfigResolution.EffectiveSpec.DefaultStorageClassName,
			ModelSources:     template.Status.ModelSources,
			Env:              template.Spec.Caching.Env,
		},
	}
}

// projectStatus computes status from observation + errors (modifies template.Status directly)
func (r *AIMServiceTemplateReconciler) projectStatus(
	ctx context.Context,
	template *aimv1alpha1.AIMServiceTemplate,
	obs *namespaceTemplateObservation,
	errs controllerutils.ReconcileErrors,
) error {
	imageNotFoundMsg := fmt.Sprintf("No AIMModel or AIMClusterModel found for image name %q", template.Spec.ModelName)
	var templateObs *shared.TemplateObservation
	if obs != nil {
		templateObs = &obs.TemplateObservation
	}
	return shared.ProjectTemplateStatus(ctx, r.Client, r.Clientset, r.Recorder, template, templateObs, errs, imageNotFoundMsg)
}

func (r *AIMServiceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMServiceTemplate{}, namespaceTemplateRuntimeConfigIndexKey, func(obj client.Object) []string {
		template, ok := obj.(*aimv1alpha1.AIMServiceTemplate)
		if !ok {
			return nil
		}
		return []string{shared.NormalizeRuntimeConfigName(template.Spec.RuntimeConfigName)}
	}); err != nil {
		return err
	}

	runtimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		runtimeConfig, ok := obj.(*aimv1alpha1.AIMRuntimeConfig)
		if !ok {
			return nil
		}

		var templates aimv1alpha1.AIMServiceTemplateList
		if err := r.List(ctx, &templates,
			client.InNamespace(runtimeConfig.Namespace),
			client.MatchingFields{
				namespaceTemplateRuntimeConfigIndexKey: shared.NormalizeRuntimeConfigName(runtimeConfig.Name),
			},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServiceTemplate for AIMRuntimeConfig",
				"runtimeConfig", runtimeConfig.Name, "namespace", runtimeConfig.Namespace)
			return nil
		}

		return requestsFromNamespaceTemplates(templates.Items)
	})

	clusterRuntimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterConfig, ok := obj.(*aimv1alpha1.AIMClusterRuntimeConfig)
		if !ok {
			return nil
		}

		var templates aimv1alpha1.AIMServiceTemplateList
		if err := r.List(ctx, &templates,
			client.MatchingFields{
				namespaceTemplateRuntimeConfigIndexKey: shared.NormalizeRuntimeConfigName(clusterConfig.Name),
			},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServiceTemplate for AIMClusterRuntimeConfig",
				"runtimeConfig", clusterConfig.Name)
			return nil
		}

		return requestsFromNamespaceTemplates(templates.Items)
	})

	nodeHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		_, ok := obj.(*corev1.Node)
		if !ok {
			return nil
		}

		var templates aimv1alpha1.AIMServiceTemplateList
		if err := r.List(ctx, &templates); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServiceTemplates for Node event")
			return nil
		}

		filtered := make([]aimv1alpha1.AIMServiceTemplate, 0, len(templates.Items))
		for i := range templates.Items {
			if shared.TemplateRequiresGPU(templates.Items[i].Spec.AIMServiceTemplateSpecCommon) {
				filtered = append(filtered, templates.Items[i])
			}
		}

		return requestsFromNamespaceTemplates(filtered)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMServiceTemplate{}).
		Owns(&batchv1.Job{}).
		Owns(&servingv1alpha1.ServingRuntime{}).
		Watches(&aimv1alpha1.AIMRuntimeConfig{}, runtimeConfigHandler).
		Watches(&aimv1alpha1.AIMClusterRuntimeConfig{}, clusterRuntimeConfigHandler).
		Watches(&corev1.Node{}, nodeHandler, builder.WithPredicates(shared.NodeGPUChangePredicate())).
		Named("aim-namespace-template").
		Complete(r)
}
