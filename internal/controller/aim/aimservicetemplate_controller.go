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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

const (
	// namespaceTemplateFinalizerName = "aim.silogen.ai/namespace-template-finalizer"
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
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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

	logger.Info("Reconciling AIMServiceTemplate", "name", template.Name, "namespace", template.Namespace)

	// Use framework orchestrator with closures
	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMServiceTemplate, aimv1alpha1.AIMServiceTemplateStatus]{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Object:   &template,
		Recorder: r.Recorder,
		// FinalizerName: namespaceTemplateFinalizerName,
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
	return shared.ObserveTemplate(ctx, shared.TemplateObservationOptions[*servingv1alpha1.ServingRuntime]{
		GetRuntime: func(ctx context.Context) (*servingv1alpha1.ServingRuntime, error) {
			servingRuntime, err := shared.GetServingRuntime(ctx, r.Client, template.Namespace, template.Name)
			if err != nil && !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get ServingRuntime: %w", err)
			}
			return servingRuntime, err
		},
		ShouldCheckDiscoveryJob: template.Status.Status != aimv1alpha1.AIMTemplateStatusAvailable,
		GetDiscoveryJob: func(ctx context.Context) (*batchv1.Job, error) {
			job, err := shared.GetDiscoveryJob(ctx, r.Client, template.Namespace, template.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to get discovery job: %w", err)
			}
			return job, nil
		},
		LookupImage: func(ctx context.Context) (string, error) {
			return shared.LookupImageForNamespaceTemplate(ctx, r.Client, template.Namespace, template.Spec.AIMImageName)
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
				logger.Info("Default AIMRuntimeConfig not found, proceeding without overrides")
				controllerutils.EmitWarningEvent(r.Recorder, template, "DefaultRuntimeConfigNotFound",
					"Default AIMRuntimeConfig not found, proceeding with controller defaults.")
				return
			}

			logger.Info("Resolved AIMRuntimeConfig",
				"name", resolution.Name,
				"sources", shared.JoinRuntimeConfigSources(resolution, template.Namespace),
				"imagePullSecrets", len(resolution.EffectiveSpec.ImagePullSecrets))

			controllerutils.EmitNormalEvent(r.Recorder, template, "RuntimeConfigResolved",
				fmt.Sprintf("Using AIMRuntimeConfig %q from %s", resolution.Name, shared.JoinRuntimeConfigSources(resolution, template.Namespace)))
		},
	})
}

// plan computes desired state (pure function)
func (r *AIMServiceTemplateReconciler) plan(_ context.Context, template *aimv1alpha1.AIMServiceTemplate, obs *namespaceTemplateObservation) ([]client.Object, error) {
	var observation *shared.TemplateObservation
	if obs != nil {
		observation = &obs.TemplateObservation
	}

	engineArgs := shared.ExtractEngineArgs(template.Status.Profile)

	desired := shared.PlanTemplateResources(shared.TemplatePlanContext{
		Template:    template,
		APIVersion:  template.APIVersion,
		Kind:        template.Kind,
		Status:      template.Status.Status,
		Observation: observation,
	}, shared.TemplatePlanBuilders{
		BuildRuntime: func(input shared.TemplatePlanInput) client.Object {
			return shared.BuildServingRuntime(shared.ServingRuntimeSpec{
				Name:             template.Name,
				Namespace:        template.Namespace,
				ModelID:          template.Spec.AIMImageName,
				Image:            input.Observation.Image,
				OwnerRef:         input.OwnerReference,
				ServiceAccount:   input.RuntimeConfigSpec.ServiceAccountName,
				ImagePullSecrets: input.Observation.ImagePullSecrets,
				EngineArgs:       engineArgs,
			})
		},
		BuildDiscoveryJob: func(input shared.TemplatePlanInput) client.Object {
			return shared.BuildDiscoveryJob(shared.DiscoveryJobSpec{
				TemplateName:     template.Name,
				Namespace:        template.Namespace,
				ModelID:          template.Spec.AIMImageName,
				Image:            input.Observation.Image,
				Env:              template.Spec.Env,
				ImagePullSecrets: input.Observation.ImagePullSecrets,
				ServiceAccount:   input.RuntimeConfigSpec.ServiceAccountName,
				OwnerRef:         input.OwnerReference,
				TemplateSpec:     template.Spec.AIMServiceTemplateSpecCommon,
			})
		},
	})

	// TODO: If caching.enabled, create AIMTemplateCache object
	// This will be handled in a future iteration

	return desired, nil
}

// projectStatus computes status from observation + errors (modifies template.Status directly)
func (r *AIMServiceTemplateReconciler) projectStatus(
	ctx context.Context,
	template *aimv1alpha1.AIMServiceTemplate,
	obs *namespaceTemplateObservation,
	errs controllerutils.ReconcileErrors,
) error {
	imageNotFoundMsg := fmt.Sprintf("No AIMImage or AIMClusterImage found for image name %q", template.Spec.AIMImageName)
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMServiceTemplate{}).
		Owns(&batchv1.Job{}).
		Owns(&servingv1alpha1.ServingRuntime{}).
		Watches(&aimv1alpha1.AIMRuntimeConfig{}, runtimeConfigHandler).
		Watches(&aimv1alpha1.AIMClusterRuntimeConfig{}, clusterRuntimeConfigHandler).
		Named("aim-namespace-template").
		Complete(r)
}
