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
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"

	"github.com/silogen/kaiwo/internal/controller/aim/routingconfig"

	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	// AIMCacheBasePath is the base directory where AIM expects to find cached models
	AIMCacheBasePath = "/workspace/model-cache"
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
		ModelResolutionErr:        selectionStatus.ModelResolutionErr,
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

	// Check InferenceService pods for image pull errors
	// Only check if we have a valid runtime name (template resolution succeeded)
	if obs.RuntimeName() != "" {
		baseutils.Debug(logger, "Checking InferenceService pods for image pull errors",
			"serviceName", service.Name)
		// Use the same naming function that we use when creating the InferenceService
		isvcName := shared.GenerateInferenceServiceName(service.Name, service.Namespace)
		obs.InferenceServicePodImageError = shared.CheckInferenceServicePodImagePullStatus(
			ctx, r.Client, isvcName, service.Namespace)
		if obs.InferenceServicePodImageError != nil {
			baseutils.Debug(logger, "Found InferenceService pod image pull error",
				"errorType", obs.InferenceServicePodImageError.Type,
				"container", obs.InferenceServicePodImageError.Container)
		}
	}

	//

	// If caching is enabled, observe template cache and available model caches for mounting
	if service.Spec.CacheModel {
		templateCaches := aimv1alpha1.AIMTemplateCacheList{}
		err := r.List(ctx, &templateCaches)
		if err != nil {
			return nil, err
		}
		for _, templateCache := range templateCaches.Items {
			if templateCache.Name == obs.TemplateName+"-tc" {
				obs.TemplateCache = &templateCache
				break
			}
		}

		modelCaches := aimv1alpha1.AIMModelCacheList{}
		err = r.List(ctx, &modelCaches)
		if err != nil {
			return nil, err
		}
		obs.ModelCaches = &modelCaches
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

		serviceState := aimstate.NewServiceState(service, templateState, aimstate.ServiceStateOptions{
			RuntimeName: obs.RuntimeName(),
			RoutePath:   routePath,
		})

		modelsReady := templateState.ModelSource != nil
		templateCacheReady := obs.TemplateCache != nil && obs.TemplateCache.Status.Status == aimv1alpha1.AIMTemplateCacheStatusAvailable

		// Map to track modelCache -> modelName for proper mounting
		type cacheMount struct {
			cache     aimv1alpha1.AIMModelCache
			modelName string
		}
		modelCachesToMount := []cacheMount{}
		if modelsReady && service.Spec.CacheModel && templateCacheReady {
			// We know our models, verify that they are cached
		SEARCH:
			for _, model := range templateState.Status.ModelSources {
				for _, modelCache := range obs.ModelCaches.Items {
					// Select first modelCache that matches sourceURI and is Available
					if model.SourceURI == modelCache.Spec.SourceURI && modelCache.Status.Status == aimv1alpha1.AIMModelCacheStatusAvailable {
						modelCachesToMount = append(modelCachesToMount, cacheMount{
							cache:     modelCache,
							modelName: model.Name,
						})
						continue SEARCH
					}
				}
				// We searched for an Available cache, but didn't find one. models aren't ready for use
				modelsReady = false
			}
		}

		serviceReady := modelsReady && (!service.Spec.CacheModel || templateCacheReady)

		// Only create InferenceService if we have a model source and cache (discovery must have succeeded and populated ModelSources)
		if serviceReady {
			serviceState := aimstate.NewServiceState(service, templateState, aimstate.ServiceStateOptions{
				RuntimeName: obs.RuntimeName(),
				RoutePath:   routePath,
			})
			inferenceService := shared.BuildInferenceService(serviceState, ownerRef)

			// If we have caches to mount, set the AIM_CACHE_PATH env var and mount them
			if len(modelCachesToMount) > 0 {
				inferenceService.Spec.Predictor.Model.Env = append(
					inferenceService.Spec.Predictor.Model.Env,
					v1.EnvVar{
						Name:  "AIM_CACHE_PATH",
						Value: AIMCacheBasePath,
					})

				for _, cm := range modelCachesToMount {
					addModelCacheMount(inferenceService, cm.cache, cm.modelName)
				}
			}

			desired = append(desired, inferenceService)
		} else {
			baseutils.Debug(logger, "Model source not available, skipping InferenceService creation")
		}

		// Create HTTPRoute if routing is enabled, regardless of model source availability
		if serviceState.Routing.Enabled && serviceState.Routing.GatewayRef != nil && obs.PathTemplateErr == nil {
			baseutils.Debug(logger, "Routing enabled, building HTTPRoute",
				"gateway", serviceState.Routing.GatewayRef.Name,
				"path", routePath)
			route := shared.BuildInferenceServiceHTTPRoute(serviceState, ownerRef)
			desired = append(desired, route)
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

func addModelCacheMount(inferenceService *servingv1beta1.InferenceService, modelCache aimv1alpha1.AIMModelCache, modelName string) {
	// Sanitize volume name to be RFC1123 compliant (max 63 chars, lowercase alphanumeric or '-')
	volumeName := baseutils.MakeRFC1123Compliant(modelCache.Name)

	// Add the PVC volume for the model cache
	inferenceService.Spec.Predictor.Volumes = append(inferenceService.Spec.Predictor.Volumes, v1.Volume{
		Name: volumeName,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: modelCache.Status.PersistentVolumeClaim,
			},
		},
	})

	// Mount at the AIM cache base path + model name (using filepath.Join for safe path construction)
	// e.g., /workspace/model-cache/meta-llama/Llama-3.1-8B
	mountPath := filepath.Join(AIMCacheBasePath, modelName)

	inferenceService.Spec.Predictor.Model.VolumeMounts = append(
		inferenceService.Spec.Predictor.Model.VolumeMounts,
		v1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
		})
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
		// Use the same naming function that we use when creating the InferenceService
		isvcName := shared.GenerateInferenceServiceName(service.Name, service.Namespace)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: service.Namespace,
			Name:      isvcName,
		}, &is); err == nil {
			inferenceService = &is
		}
	}

	var httpRoute *gatewayapiv1.HTTPRoute
	// Check if routing is enabled via service spec or runtime config
	var runtimeRouting *aimv1alpha1.AIMRuntimeRoutingConfig
	if obs != nil {
		runtimeRouting = obs.RuntimeConfigSpec.Routing
	}
	routingConfig := routingconfig.Resolve(service, runtimeRouting)
	if routingConfig.Enabled {
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

// findServicesByTemplate finds services that reference a template by name or share the same model
func (r *AIMServiceReconciler) findServicesByTemplate(
	ctx context.Context,
	templateName string,
	templateNamespace string,
	modelName string,
	isClusterScoped bool,
) []aimv1alpha1.AIMService {
	// Find services that reference this template by name (explicit templateRef or already resolved)
	var servicesWithRef aimv1alpha1.AIMServiceList
	listOpts := []client.ListOption{client.MatchingFields{aimServiceTemplateIndexKey: templateName}}
	if !isClusterScoped {
		listOpts = append(listOpts, client.InNamespace(templateNamespace))
	}

	if err := r.List(ctx, &servicesWithRef, listOpts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for template", "template", templateName)
		return nil
	}

	// Also find services doing auto-selection with the same image name
	var servicesWithImage aimv1alpha1.AIMServiceList
	imageListOpts := []client.ListOption{}
	if !isClusterScoped {
		imageListOpts = append(imageListOpts, client.InNamespace(templateNamespace))
	}

	if err := r.List(ctx, &servicesWithImage, imageListOpts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for image matching")
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
			svcModelName := r.getServiceModelName(&svc)
			if svcModelName != "" && svcModelName == strings.TrimSpace(modelName) {
				serviceMap[key] = svc
			}
		}
	}

	// Convert map to slice
	services := make([]aimv1alpha1.AIMService, 0, len(serviceMap))
	for _, svc := range serviceMap {
		services = append(services, svc)
	}

	return services
}

// getServiceModelName extracts the model name from a service
func (r *AIMServiceReconciler) getServiceModelName(svc *aimv1alpha1.AIMService) string {
	if svc.Status.ResolvedImage != nil {
		return svc.Status.ResolvedImage.Name
	}
	if svc.Spec.Model.Ref != nil {
		return strings.TrimSpace(*svc.Spec.Model.Ref)
	}
	return ""
}

// templateHandlerFunc returns a handler function for AIMServiceTemplate watches
func (r *AIMServiceReconciler) templateHandlerFunc() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		template, ok := obj.(*aimv1alpha1.AIMServiceTemplate)
		if !ok {
			return nil
		}

		services := r.findServicesByTemplate(ctx, template.Name, template.Namespace, template.Spec.ModelName, false)
		return shared.RequestsForServices(services)
	}
}

// clusterTemplateHandlerFunc returns a handler function for AIMClusterServiceTemplate watches
func (r *AIMServiceReconciler) clusterTemplateHandlerFunc() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterTemplate, ok := obj.(*aimv1alpha1.AIMClusterServiceTemplate)
		if !ok {
			return nil
		}

		services := r.findServicesByTemplate(ctx, clusterTemplate.Name, "", clusterTemplate.Spec.ModelName, true)
		return shared.RequestsForServices(services)
	}
}

func (r *AIMServiceReconciler) templateCacheHandlerFunc() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		templateCache, ok := obj.(*aimv1alpha1.AIMTemplateCache)
		if !ok {
			return nil
		}

		var services aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &services,
			client.InNamespace(templateCache.Namespace),
			client.MatchingFields{aimServiceTemplateIndexKey: templateCache.Spec.TemplateRef},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMServiceTemplate", "template", templateCache.Name)
			return nil
		}

		return shared.RequestsForServices(services.Items)
	}
}

// findServicesByModel finds services that reference a model
func (r *AIMServiceReconciler) findServicesByModel(ctx context.Context, model *aimv1alpha1.AIMModel) []aimv1alpha1.AIMService {
	logger := ctrl.LoggerFrom(ctx)

	// Only trigger reconciliation for auto-created models
	if model.Labels[shared.LabelAutoCreated] != "true" {
		logger.V(1).Info("Skipping model - not auto-created", "model", model.Name)
		return nil
	}

	// Find services using this model
	var services aimv1alpha1.AIMServiceList
	if err := r.List(ctx, &services, client.InNamespace(model.Namespace)); err != nil {
		logger.Error(err, "failed to list AIMServices for AIMModel", "model", model.Name)
		return nil
	}

	var matchingServices []aimv1alpha1.AIMService
	for i := range services.Items {
		svc := &services.Items[i]
		if r.serviceUsesModel(svc, model, logger) {
			matchingServices = append(matchingServices, *svc)
		}
	}

	return matchingServices
}

// serviceUsesModel checks if a service uses the specified model
func (r *AIMServiceReconciler) serviceUsesModel(svc *aimv1alpha1.AIMService, model *aimv1alpha1.AIMModel, logger logr.Logger) bool {
	// Check if service uses this model by:
	// 1. Explicit ref (spec.model.ref)
	if svc.Spec.Model.Ref != nil && *svc.Spec.Model.Ref == model.Name {
		return true
	}
	// 2. Image URL that resolves to this model (check status)
	if svc.Status.ResolvedImage != nil && svc.Status.ResolvedImage.Name == model.Name {
		return true
	}
	// 3. Image URL in spec (need to check if it would resolve to this model)
	// This is the case when service was just created and status not yet set
	if svc.Spec.Model.Image != nil {
		logger.V(1).Info("Service has image URL but no resolved image yet",
			"service", svc.Name,
			"image", *svc.Spec.Model.Image,
			"model", model.Name)
		// For now, add all services with image URLs in the same namespace
		// The service reconciliation will properly resolve and filter
		return true
	}
	return false
}

// modelHandlerFunc returns a handler function for AIMModel watches
func (r *AIMServiceReconciler) modelHandlerFunc() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		model, ok := obj.(*aimv1alpha1.AIMModel)
		if !ok {
			return nil
		}

		matchingServices := r.findServicesByModel(ctx, model)
		return shared.RequestsForServices(matchingServices)
	}
}

// modelPredicate returns a predicate for AIMModel watches
func (r *AIMServiceReconciler) modelPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Don't trigger on creation - model status is empty initially
			ctrl.Log.V(1).Info("AIMModel create event (skipped)", "model", e.Object.GetName())
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldModel, ok := e.ObjectOld.(*aimv1alpha1.AIMModel)
			if !ok {
				return false
			}
			newModel, ok := e.ObjectNew.(*aimv1alpha1.AIMModel)
			if !ok {
				return false
			}
			// Trigger if status changed
			statusChanged := oldModel.Status.Status != newModel.Status.Status
			if statusChanged {
				ctrl.Log.Info("AIMModel status changed - triggering reconciliation",
					"model", newModel.Name,
					"namespace", newModel.Namespace,
					"oldStatus", oldModel.Status.Status,
					"newStatus", newModel.Status.Status)
			} else {
				ctrl.Log.V(1).Info("AIMModel update (no status change)",
					"model", newModel.Name,
					"status", newModel.Status.Status)
			}
			return statusChanged
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			ctrl.Log.V(1).Info("AIMModel delete event (skipped)", "model", e.Object.GetName())
			return false
		},
	}
}

// findServicesByClusterModel finds services that reference a cluster model
func (r *AIMServiceReconciler) findServicesByClusterModel(ctx context.Context, clusterModel *aimv1alpha1.AIMClusterModel) []aimv1alpha1.AIMService {
	logger := ctrl.LoggerFrom(ctx)

	// Note: Unlike namespace-scoped models, cluster models are never auto-created by the system.
	// They are manually created infrastructure resources, so we reconcile services for all cluster model changes.

	// Find services across all namespaces using this cluster model
	var services aimv1alpha1.AIMServiceList
	if err := r.List(ctx, &services); err != nil {
		logger.Error(err, "failed to list AIMServices for AIMClusterModel", "model", clusterModel.Name)
		return nil
	}

	var matchingServices []aimv1alpha1.AIMService
	for i := range services.Items {
		svc := &services.Items[i]
		if r.serviceUsesClusterModel(svc, clusterModel, logger) {
			matchingServices = append(matchingServices, *svc)
		}
	}

	return matchingServices
}

// serviceUsesClusterModel checks if a service uses the specified cluster model
func (r *AIMServiceReconciler) serviceUsesClusterModel(svc *aimv1alpha1.AIMService, clusterModel *aimv1alpha1.AIMClusterModel, logger logr.Logger) bool {
	// Check if service uses this cluster model by:
	// 1. Explicit ref (spec.model.ref)
	if svc.Spec.Model.Ref != nil && *svc.Spec.Model.Ref == clusterModel.Name {
		return true
	}
	// 2. Image URL that resolves to this cluster model (check status)
	if svc.Status.ResolvedImage != nil && svc.Status.ResolvedImage.Name == clusterModel.Name {
		return true
	}
	// 3. Image URL in spec (need to check if it would resolve to this cluster model)
	// This is the case when service was just created and status not yet set
	if svc.Spec.Model.Image != nil {
		logger.V(1).Info("Service has image URL but no resolved image yet",
			"service", svc.Name,
			"namespace", svc.Namespace,
			"image", *svc.Spec.Model.Image,
			"clusterModel", clusterModel.Name)
		// For cluster models, add all services with image URLs across all namespaces
		// The service reconciliation will properly resolve and filter
		return true
	}
	return false
}

func (r *AIMServiceReconciler) clusterModelHandlerFunc() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterModel, ok := obj.(*aimv1alpha1.AIMClusterModel)
		if !ok {
			return nil
		}

		matchingServices := r.findServicesByClusterModel(ctx, clusterModel)
		return shared.RequestsForServices(matchingServices)
	}
}

// clusterModelPredicate returns a predicate for AIMClusterModel watches
func (r *AIMServiceReconciler) clusterModelPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Don't trigger on creation - model status is empty initially
			ctrl.Log.V(1).Info("AIMClusterModel create event (skipped)", "model", e.Object.GetName())
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldModel, ok := e.ObjectOld.(*aimv1alpha1.AIMClusterModel)
			if !ok {
				return false
			}
			newModel, ok := e.ObjectNew.(*aimv1alpha1.AIMClusterModel)
			if !ok {
				return false
			}
			// Trigger if status changed
			statusChanged := oldModel.Status.Status != newModel.Status.Status
			if statusChanged {
				ctrl.Log.Info("AIMClusterModel status changed - triggering reconciliation",
					"model", newModel.Name,
					"oldStatus", oldModel.Status.Status,
					"newStatus", newModel.Status.Status)
			} else {
				ctrl.Log.V(1).Info("AIMClusterModel update (no status change)",
					"model", newModel.Name,
					"status", newModel.Status.Status)
			}
			return statusChanged
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			ctrl.Log.V(1).Info("AIMClusterModel delete event (skipped)", "model", e.Object.GetName())
			return false
		},
	}
}

func (r *AIMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMService{}, aimServiceTemplateIndexKey, r.templateIndexFunc); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMService{}).
		Owns(&servingv1beta1.InferenceService{}).
		Owns(&aimv1alpha1.AIMServiceTemplate{}).
		Owns(&gatewayapiv1.HTTPRoute{}).
		Watches(&aimv1alpha1.AIMServiceTemplate{}, handler.EnqueueRequestsFromMapFunc(r.templateHandlerFunc())).
		Watches(&aimv1alpha1.AIMClusterServiceTemplate{}, handler.EnqueueRequestsFromMapFunc(r.clusterTemplateHandlerFunc())).
		Watches(&aimv1alpha1.AIMModel{}, handler.EnqueueRequestsFromMapFunc(r.modelHandlerFunc()), builder.WithPredicates(r.modelPredicate())).
		Watches(&aimv1alpha1.AIMClusterModel{}, handler.EnqueueRequestsFromMapFunc(r.clusterModelHandlerFunc()), builder.WithPredicates(r.clusterModelPredicate())).
		Watches(&aimv1alpha1.AIMTemplateCache{}, handler.EnqueueRequestsFromMapFunc(r.templateCacheHandlerFunc())).
		Named("aim-service").
		Complete(r)
}

// templateIndexFunc provides the index function for template references
func (r *AIMServiceReconciler) templateIndexFunc(obj client.Object) []string {
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
}
