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
	stderrors "errors"
	"fmt"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	clusterTemplateFieldOwner            = "aim-cluster-template-controller"
	clusterTemplateRuntimeConfigIndexKey = ".spec.runtimeConfigName"
)

// AIMClusterServiceTemplateReconciler reconciles a AIMClusterServiceTemplate object
type AIMClusterServiceTemplateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimruntimeconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=clusterservingruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *AIMClusterServiceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the template
	var template aimv1alpha1.AIMClusterServiceTemplate
	if err := r.Get(ctx, req.NamespacedName, &template); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling AIMClusterServiceTemplate", "name", template.Name)

	// Use framework orchestrator with closures
	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMClusterServiceTemplate, aimv1alpha1.AIMServiceTemplateStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &template,
		Recorder:   r.Recorder,
		FieldOwner: clusterTemplateFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &template)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var o *clusterTemplateObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*clusterTemplateObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &template, o)
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *clusterTemplateObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*clusterTemplateObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &template, o, errs)
		},

		FinalizeFn: nil, // No external cleanup needed
	})
}

// clusterTemplateObservation holds observed state
type clusterTemplateObservation = shared.RuntimeObservation[*servingv1alpha1.ClusterServingRuntime]

func requestsFromClusterTemplates(templates []aimv1alpha1.AIMClusterServiceTemplate) []reconcile.Request {
	if len(templates) == 0 {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(templates))
	for _, tpl := range templates {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: tpl.Name}})
	}
	return requests
}

// observe gathers current cluster state (read-only)
func (r *AIMClusterServiceTemplateReconciler) observe(ctx context.Context, template *aimv1alpha1.AIMClusterServiceTemplate) (*clusterTemplateObservation, error) {
	logger := log.FromContext(ctx)
	operatorNamespace := shared.GetOperatorNamespace()
	observation, err := shared.ObserveTemplate(ctx, shared.TemplateObservationOptions[*servingv1alpha1.ClusterServingRuntime]{
		K8sClient: r.Client,
		GetRuntime: func(ctx context.Context) (*servingv1alpha1.ClusterServingRuntime, error) {
			runtime, err := shared.GetClusterServingRuntime(ctx, r.Client, template.Name)
			if err != nil && !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get ClusterServingRuntime: %w", err)
			}
			return runtime, err
		},
		ShouldCheckDiscoveryJob: len(template.Spec.ModelSources) == 0 &&
			template.Status.Status != aimv1alpha1.AIMTemplateStatusReady &&
			template.Status.Status != aimv1alpha1.AIMTemplateStatusNotAvailable,
		GetDiscoveryJob: func(ctx context.Context) (*batchv1.Job, error) {
			job, err := shared.GetDiscoveryJob(ctx, r.Client, operatorNamespace, template.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to get discovery job: %w", err)
			}
			return job, nil
		},
		GetJobNamespace: func() string {
			return operatorNamespace
		},
		LookupImage: func(ctx context.Context) (*shared.ImageLookupResult, error) {
			return shared.LookupImageForClusterTemplate(ctx, r.Client, template.Spec.ModelName)
		},
		ResolveRuntimeConfig: func(ctx context.Context) (*shared.RuntimeConfigResolution, error) {
			resolution, err := shared.ResolveRuntimeConfig(ctx, r.Client, operatorNamespace, template.Spec.RuntimeConfigName)
			if err != nil {
				if stderrors.Is(err, shared.ErrRuntimeConfigNotFound) {
					baseutils.Debug(logger, "Namespaced AIMRuntimeConfig not found for cluster template, proceeding without overrides",
						"name", template.Spec.RuntimeConfigName,
						"operatorNamespace", operatorNamespace)
					return nil, nil
				}
				return nil, fmt.Errorf("failed to resolve AIMRuntimeConfig %q: %w", template.Spec.RuntimeConfigName, err)
			}
			return resolution, nil
		},
		OnRuntimeConfigResolved: func(resolution *shared.RuntimeConfigResolution) {
			if resolution.NamespaceConfig == nil && resolution.ClusterConfig == nil && resolution.Name == shared.DefaultRuntimeConfigName {
				baseutils.Debug(logger, "Default AIMRuntimeConfig not found for cluster template, proceeding without overrides")
				controllerutils.EmitWarningEvent(r.Recorder, template, "DefaultRuntimeConfigNotFound",
					"Default AIMRuntimeConfig not found, proceeding with controller defaults.")
				return
			}

			baseutils.Debug(logger, "Resolved AIMRuntimeConfig",
				"name", resolution.Name,
				"sources", shared.JoinRuntimeConfigSources(resolution, operatorNamespace))

			controllerutils.EmitNormalEvent(r.Recorder, template, "RuntimeConfigResolved",
				fmt.Sprintf("Using AIMRuntimeConfig %q from %s", resolution.Name, shared.JoinRuntimeConfigSources(resolution, operatorNamespace)))
		},
		GetImagePullSecrets: func() []corev1.LocalObjectReference {
			return template.Spec.ImagePullSecrets
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
func (r *AIMClusterServiceTemplateReconciler) plan(ctx context.Context, template *aimv1alpha1.AIMClusterServiceTemplate, obs *clusterTemplateObservation) ([]client.Object, error) {
	var observation *shared.TemplateObservation
	if obs != nil {
		observation = &obs.TemplateObservation
	}

	operatorNamespace := shared.GetOperatorNamespace()

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
			base := aimstate.TemplateState{
				Name:              template.Name,
				Namespace:         "",
				SpecCommon:        template.Spec.AIMServiceTemplateSpecCommon,
				RuntimeConfigSpec: input.RuntimeConfigSpec,
				Status:            template.Status.DeepCopy(),
			}
			if input.Observation != nil {
				base.Image = input.Observation.Image
				base.ImagePullSecrets = input.Observation.ImagePullSecrets
			}
			templateState := aimstate.NewTemplateState(base)
			return shared.BuildClusterServingRuntime(templateState, input.OwnerReference)
		},
		BuildDiscoveryJob: func(input shared.TemplatePlanInput) client.Object {
			return shared.BuildDiscoveryJob(shared.DiscoveryJobSpec{
				TemplateName:     template.Name,
				Namespace:        operatorNamespace,
				ModelID:          template.Spec.ModelName,
				Image:            input.Observation.Image,
				TemplateSpec:     template.Spec.AIMServiceTemplateSpecCommon,
				ImagePullSecrets: input.Observation.ImagePullSecrets,
				ServiceAccount:   input.RuntimeConfigSpec.ServiceAccountName,
				OwnerRef:         input.OwnerReference,
			})
		},
	})

	return desired, nil
}

// projectStatus computes status from observation + errors (modifies template.Status directly)
func (r *AIMClusterServiceTemplateReconciler) projectStatus(
	ctx context.Context,
	template *aimv1alpha1.AIMClusterServiceTemplate,
	obs *clusterTemplateObservation,
	errs controllerutils.ReconcileErrors,
) error {
	imageNotFoundMsg := fmt.Sprintf("No AIMClusterModel found for image name %q", template.Spec.ModelName)
	var templateObs *shared.TemplateObservation
	if obs != nil {
		templateObs = &obs.TemplateObservation
	}
	return shared.ProjectTemplateStatus(ctx, r.Client, r.Clientset, r.Recorder, template, templateObs, errs, imageNotFoundMsg)
}

func (r *AIMClusterServiceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMClusterServiceTemplate{}, clusterTemplateRuntimeConfigIndexKey, func(obj client.Object) []string {
		template, ok := obj.(*aimv1alpha1.AIMClusterServiceTemplate)
		if !ok {
			return nil
		}
		return []string{shared.NormalizeRuntimeConfigName(template.Spec.RuntimeConfigName)}
	}); err != nil {
		return err
	}

	clusterRuntimeConfigHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterConfig, ok := obj.(*aimv1alpha1.AIMClusterRuntimeConfig)
		if !ok {
			return nil
		}

		var templates aimv1alpha1.AIMClusterServiceTemplateList
		if err := r.List(ctx, &templates,
			client.MatchingFields{
				clusterTemplateRuntimeConfigIndexKey: shared.NormalizeRuntimeConfigName(clusterConfig.Name),
			},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMClusterServiceTemplate for AIMClusterRuntimeConfig",
				"runtimeConfig", clusterConfig.Name)
			return nil
		}

		return requestsFromClusterTemplates(templates.Items)
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

		var templates aimv1alpha1.AIMClusterServiceTemplateList
		if err := r.List(ctx, &templates,
			client.MatchingFields{
				clusterTemplateRuntimeConfigIndexKey: shared.NormalizeRuntimeConfigName(runtimeConfig.Name),
			},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMClusterServiceTemplate for AIMRuntimeConfig",
				"runtimeConfig", runtimeConfig.Name, "namespace", runtimeConfig.Namespace)
			return nil
		}

		return requestsFromClusterTemplates(templates.Items)
	})

	nodeHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		_, ok := obj.(*corev1.Node)
		if !ok {
			return nil
		}

		var templates aimv1alpha1.AIMClusterServiceTemplateList
		if err := r.List(ctx, &templates); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMClusterServiceTemplates for Node event")
			return nil
		}

		filtered := make([]aimv1alpha1.AIMClusterServiceTemplate, 0, len(templates.Items))
		for i := range templates.Items {
			if shared.TemplateRequiresGPU(templates.Items[i].Spec.AIMServiceTemplateSpecCommon) {
				filtered = append(filtered, templates.Items[i])
			}
		}

		return requestsFromClusterTemplates(filtered)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMClusterServiceTemplate{}).
		Owns(&batchv1.Job{}).
		Owns(&servingv1alpha1.ClusterServingRuntime{}).
		Watches(&aimv1alpha1.AIMClusterRuntimeConfig{}, clusterRuntimeConfigHandler).
		Watches(&aimv1alpha1.AIMRuntimeConfig{}, runtimeConfigHandler).
		Watches(&corev1.Node{}, nodeHandler, builder.WithPredicates(shared.NodeGPUChangePredicate())).
		Named("aim-cluster-template").
		Complete(r)
}
