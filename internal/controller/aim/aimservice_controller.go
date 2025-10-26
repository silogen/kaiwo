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
	"strings"

	"github.com/silogen/kaiwo/internal/controller/aim/routingconfig"

	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	aimServiceFieldOwner       = "aim-service-controller"
	aimServiceTemplateIndexKey = ".spec.templateRef"
)

// AIMServiceReconciler reconciles AIMService resources into KServe InferenceServices.
type AIMServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimservicetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimclusterservicetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices/status,verbs=get
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *AIMServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var service aimv1alpha1.AIMService
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling AIMService", "name", service.Name, "namespace", service.Namespace)

	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMService, aimv1alpha1.AIMServiceStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &service,
		Recorder:   r.Recorder,
		FieldOwner: aimServiceFieldOwner,
		ObserveFn: func(ctx context.Context) (any, error) {
			obs, err := r.observe(ctx, &service)
			if err != nil {
				logger.Error(err, "Observe failed")
			} else {
				logger.Info("Observe completed")
			}
			return obs, err
		},
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var observation *shared.ServiceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*shared.ServiceObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			objs := r.plan(ctx, &service, observation)

			return objs, nil
		},
		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var observation *shared.ServiceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*shared.ServiceObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &service, observation, errs)
		},
	})
}

func (r *AIMServiceReconciler) observe(ctx context.Context, service *aimv1alpha1.AIMService) (*shared.ServiceObservation, error) {
	logger := log.FromContext(ctx)
	resolution, selectionStatus, err := shared.ResolveTemplateNameForService(ctx, r.Client, service)
	if err != nil {
		return nil, err
	}

	baseutils.Debug(logger, "Template resolution complete",
		"finalName", resolution.FinalName,
		"baseName", resolution.BaseName,
		"derived", resolution.Derived,
		"autoSelected", selectionStatus.AutoSelected,
		"candidateCount", selectionStatus.CandidateCount)

	obs := &shared.ServiceObservation{
		TemplateName:              resolution.FinalName,
		BaseTemplateName:          resolution.BaseName,
		Scope:                     shared.TemplateScopeNone,
		AutoSelectedTemplate:      selectionStatus.AutoSelected,
		TemplateSelectionReason:   selectionStatus.SelectionReason,
		TemplateSelectionMessage:  selectionStatus.SelectionMessage,
		TemplateSelectionCount:    selectionStatus.CandidateCount,
		TemplatesExistButNotReady: selectionStatus.TemplatesExistButNotReady,
		ImageReady:                selectionStatus.ImageReady,
		ImageReadyReason:          selectionStatus.ImageReadyReason,
		ImageReadyMessage:         selectionStatus.ImageReadyMessage,
	}

	// Observe template based on whether it's derived or not
	if resolution.Derived {
		obs.TemplateNamespace = service.Namespace
		baseutils.Debug(logger, "Observing derived template", "templateName", resolution.FinalName)
		if err := shared.ObserveDerivedTemplate(ctx, r.Client, service, resolution, obs); err != nil {
			return nil, err
		}
	} else if resolution.FinalName != "" {
		baseutils.Debug(logger, "Observing non-derived template", "templateName", resolution.FinalName, "scope", resolution.Scope)
		if err := shared.ObserveNonDerivedTemplate(ctx, r.Client, service, resolution.FinalName, resolution.Scope, obs); err != nil {
			return nil, err
		}
	}

	// Set template namespace if creating a new template
	if obs.ShouldCreateTemplate && obs.TemplateNamespace == "" {
		obs.TemplateNamespace = service.Namespace
	}

	// Only auto-create templates when overrides are specified (derived templates).
	// If no template can be resolved and no overrides are specified, the service should degrade.
	// This prevents magic template creation and enforces explicit configuration.
	if !obs.TemplateFound() && resolution.Derived {
		obs.ShouldCreateTemplate = true
		baseutils.Debug(logger, "Will create derived template", "templateName", obs.TemplateName)
	}

	// Resolve route path if routing is enabled via service or runtime defaults
	routingConfig := routingconfig.Resolve(service, obs.RuntimeConfigSpec.Routing)
	if routingConfig.Enabled && obs.TemplateFound() {
		baseutils.Debug(logger, "Routing is enabled, resolving route path")
		if routePath, err := shared.ResolveServiceRoutePath(service, obs.RuntimeConfigSpec); err != nil {
			obs.PathTemplateErr = err
			baseutils.Debug(logger, "Route path resolution failed", "error", err)
		} else {
			obs.RoutePath = routePath
			baseutils.Debug(logger, "Route path resolved", "path", routePath)
		}
	}

	return obs, nil
}

func (r *AIMServiceReconciler) plan(ctx context.Context, service *aimv1alpha1.AIMService, obs *shared.ServiceObservation) []client.Object {
	logger := log.FromContext(ctx)
	var desired []client.Object

	if obs == nil {
		return desired
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         service.APIVersion,
		Kind:               service.Kind,
		Name:               service.Name,
		UID:                service.UID,
		Controller:         baseutils.Pointer(true),
		BlockOwnerDeletion: baseutils.Pointer(true),
	}

	// Manage namespace-scoped template if we created it or need to create it.
	if obs.ShouldCreateTemplate || (obs.Scope == shared.TemplateScopeNamespace && obs.TemplateOwnedByService) {
		baseutils.Debug(logger, "Planning to manage derived template",
			"shouldCreate", obs.ShouldCreateTemplate,
			"ownedByService", obs.TemplateOwnedByService)
		var baseSpec *aimv1alpha1.AIMServiceTemplateSpec
		if obs.TemplateSpec != nil {
			baseSpec = obs.TemplateSpec.DeepCopy()
		}
		// Get resolved model name from observation
		resolvedModelName := ""
		if obs.ResolvedImage != nil {
			resolvedModelName = obs.ResolvedImage.Name
		}
		template := shared.BuildDerivedTemplate(service, obs.TemplateName, resolvedModelName, baseSpec)
		desired = append(desired, template)
	}

	// Only create/update the InferenceService once the template is available.
	if obs.TemplateAvailable && obs.RuntimeConfigErr == nil {
		baseutils.Debug(logger, "Template is available, planning InferenceService")
		routePath := shared.DefaultRoutePath(service)
		if obs.PathTemplateErr == nil && obs.RoutePath != "" {
			routePath = obs.RoutePath
		}
		templateState := aimstate.NewTemplateState(aimstate.TemplateState{
			Name:              obs.TemplateName,
			Namespace:         obs.TemplateNamespace,
			SpecCommon:        obs.TemplateSpecCommon,
			ImageResources:    obs.ImageResources,
			RuntimeConfigSpec: obs.RuntimeConfigSpec,
			Status:            obs.TemplateStatus,
		})

		// Only proceed if we have a model source (discovery must have succeeded and populated ModelSources)
		if templateState.ModelSource != nil {
			baseutils.Debug(logger, "Model source available, building InferenceService")
			serviceState := aimstate.NewServiceState(service, templateState, aimstate.ServiceStateOptions{
				RuntimeName: obs.RuntimeName(),
				RoutePath:   routePath,
			})
			inferenceService := shared.BuildInferenceService(serviceState, ownerRef)
			desired = append(desired, inferenceService)

			if serviceState.Routing.Enabled && serviceState.Routing.GatewayRef != nil && obs.PathTemplateErr == nil {
				baseutils.Debug(logger, "Routing enabled, building HTTPRoute",
					"gateway", serviceState.Routing.GatewayRef.Name,
					"path", routePath)
				route := shared.BuildInferenceServiceHTTPRoute(serviceState, ownerRef)
				desired = append(desired, route)
			}
		} else {
			baseutils.Debug(logger, "Model source not available, skipping InferenceService creation")
		}
		// If ModelSource is nil, the status projection will handle showing that discovery
		// succeeded but produced no usable model sources.
	} else {
		baseutils.Debug(logger, "Template not available or runtime config error, skipping InferenceService",
			"templateAvailable", obs.TemplateAvailable,
			"hasRuntimeConfigErr", obs.RuntimeConfigErr != nil)
	}

	return desired
}

func (r *AIMServiceReconciler) projectStatus(
	ctx context.Context,
	service *aimv1alpha1.AIMService,
	obs *shared.ServiceObservation,
	errs controllerutils.ReconcileErrors,
) error {
	// Fetch InferenceService and HTTPRoute for status evaluation
	var inferenceService *servingv1beta1.InferenceService
	{
		var is servingv1beta1.InferenceService
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: service.Namespace,
			Name:      service.Name,
		}, &is); err == nil {
			inferenceService = &is
		}
	}

	var httpRoute *gatewayapiv1.HTTPRoute
	if service.Spec.Routing != nil && service.Spec.Routing.Enabled {
		routeName := shared.InferenceServiceRouteName(service.Name)
		var route gatewayapiv1.HTTPRoute
		if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: service.Namespace}, &route); err == nil {
			httpRoute = &route
		}
	}

	// Delegate status projection to shared function
	shared.ProjectServiceStatus(service, obs, inferenceService, httpRoute, errs)
	return nil
}

func (r *AIMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMService{}, aimServiceTemplateIndexKey, func(obj client.Object) []string {
		service, ok := obj.(*aimv1alpha1.AIMService)
		if !ok {
			return nil
		}
		resolved := strings.TrimSpace(service.Spec.TemplateRef)
		if resolved == "" {
			if service.Status.ResolvedTemplate != nil {
				resolved = strings.TrimSpace(service.Status.ResolvedTemplate.Name)
			}
		}
		if resolved == "" {
			return nil
		}
		return []string{resolved}
	}); err != nil {
		return err
	}

	templateHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		template, ok := obj.(*aimv1alpha1.AIMServiceTemplate)
		if !ok {
			return nil
		}

		// Find services that reference this template by name (explicit templateRef or already resolved)
		var servicesWithRef aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &servicesWithRef,
			client.InNamespace(template.Namespace),
			client.MatchingFields{aimServiceTemplateIndexKey: template.Name},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMServiceTemplate", "template", template.Name)
			return nil
		}

		// Also find services doing auto-selection with the same image name in the same namespace
		var servicesWithImage aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &servicesWithImage, client.InNamespace(template.Namespace)); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices in namespace for image matching")
			return nil
		}

		// Combine results, filtering for services with matching image that don't have explicit templateRef
		serviceMap := make(map[string]aimv1alpha1.AIMService)
		for _, svc := range servicesWithRef.Items {
			serviceMap[svc.Namespace+"/"+svc.Name] = svc
		}

		for _, svc := range servicesWithImage.Items {
			// Skip if already included via template name index
			key := svc.Namespace + "/" + svc.Name
			if _, exists := serviceMap[key]; exists {
				continue
			}

			// Include if doing auto-selection (no templateRef) and matches resolved image
			if strings.TrimSpace(svc.Spec.TemplateRef) == "" {
				// Try to get model name from status (if already resolved)
				var modelName string
				if svc.Status.ResolvedImage != nil {
					modelName = svc.Status.ResolvedImage.Name
				} else if svc.Spec.Model.Ref != nil {
					// Use ref directly if specified
					modelName = strings.TrimSpace(*svc.Spec.Model.Ref)
				}
				// Note: model.image case won't match until first reconciliation resolves the model

				if modelName != "" && modelName == strings.TrimSpace(template.Spec.ModelName) {
					serviceMap[key] = svc
				}
			}
		}

		// Convert map to slice
		services := make([]aimv1alpha1.AIMService, 0, len(serviceMap))
		for _, svc := range serviceMap {
			services = append(services, svc)
		}

		return shared.RequestsForServices(services)
	})

	clusterTemplateHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterTemplate, ok := obj.(*aimv1alpha1.AIMClusterServiceTemplate)
		if !ok {
			return nil
		}

		// Find services that reference this template by name (explicit templateRef or already resolved)
		var servicesWithRef aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &servicesWithRef,
			client.MatchingFields{aimServiceTemplateIndexKey: clusterTemplate.Name},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMClusterServiceTemplate", "template", clusterTemplate.Name)
			return nil
		}

		// Also find services doing auto-selection with the same image name
		// These won't be in the index until they resolve a template
		var servicesWithImage aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &servicesWithImage); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list all AIMServices for image matching")
			return nil
		}

		// Combine results, filtering for services with matching image that don't have explicit templateRef
		serviceMap := make(map[string]aimv1alpha1.AIMService)
		for _, svc := range servicesWithRef.Items {
			serviceMap[svc.Namespace+"/"+svc.Name] = svc
		}

		for _, svc := range servicesWithImage.Items {
			// Skip if already included via template name index
			key := svc.Namespace + "/" + svc.Name
			if _, exists := serviceMap[key]; exists {
				continue
			}

			// Include if doing auto-selection (no templateRef) and matches resolved image
			if strings.TrimSpace(svc.Spec.TemplateRef) == "" {
				// Try to get model name from status (if already resolved)
				var modelName string
				if svc.Status.ResolvedImage != nil {
					modelName = svc.Status.ResolvedImage.Name
				} else if svc.Spec.Model.Ref != nil {
					// Use ref directly if specified
					modelName = strings.TrimSpace(*svc.Spec.Model.Ref)
				}
				// Note: model.image case won't match until first reconciliation resolves the model

				if modelName != "" && modelName == strings.TrimSpace(clusterTemplate.Spec.ModelName) {
					serviceMap[key] = svc
				}
			}
		}

		// Convert map to slice
		services := make([]aimv1alpha1.AIMService, 0, len(serviceMap))
		for _, svc := range serviceMap {
			services = append(services, svc)
		}

		return shared.RequestsForServices(services)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMService{}).
		Owns(&servingv1beta1.InferenceService{}).
		Owns(&aimv1alpha1.AIMServiceTemplate{}).
		Owns(&gatewayapiv1.HTTPRoute{}).
		Watches(&aimv1alpha1.AIMServiceTemplate{}, templateHandler).
		Watches(&aimv1alpha1.AIMClusterServiceTemplate{}, clusterTemplateHandler).
		Named("aim-service").
		Complete(r)
}
