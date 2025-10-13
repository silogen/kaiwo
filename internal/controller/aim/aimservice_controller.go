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

	"github.com/silogen/kaiwo/internal/controller/framework"

	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	aimServiceFieldOwner       = "aim-service-controller"
	aimServiceTemplateIndexKey = ".spec.templateRef"
)

type templateScope string

const (
	templateScopeNone      templateScope = ""
	templateScopeNamespace templateScope = "namespace"
	templateScopeCluster   templateScope = "cluster"
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

func (r *AIMServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var service aimv1alpha1.AIMService
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling AIMService", "name", service.Name, "namespace", service.Namespace)

	return framework.Reconcile(ctx, framework.ReconcileSpec[*aimv1alpha1.AIMService, aimv1alpha1.AIMServiceStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &service,
		Recorder:   r.Recorder,
		FieldOwner: aimServiceFieldOwner,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &service)
		},
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var observation *aimServiceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*aimServiceObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &service, observation)
		},
		ProjectFn: func(ctx context.Context, obs any, errs framework.ReconcileErrors) error {
			var observation *aimServiceObservation
			if obs != nil {
				var ok bool
				observation, ok = obs.(*aimServiceObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &service, observation, errs)
		},
	})
}

type aimServiceObservation struct {
	TemplateName           string
	Scope                  templateScope
	TemplateAvailable      bool
	TemplateOwnedByService bool
	ShouldCreateTemplate   bool
	EffectiveRuntimeConfig *aimv1alpha1.AIMEffectiveRuntimeConfig
	InferenceService       *servingv1beta1.InferenceService
	HTTPRoute              *gatewayapiv1.HTTPRoute
	TemplateStatus         *aimv1alpha1.AIMServiceTemplateStatus
	TemplateSpecCommon     aimv1alpha1.AIMServiceTemplateSpecCommon
	TemplateNamespace      string
}

func (o *aimServiceObservation) templateFound() bool {
	return o != nil && o.Scope != templateScopeNone
}

func (o *aimServiceObservation) runtimeName() string {
	if o == nil {
		return ""
	}
	return o.TemplateName
}

func (r *AIMServiceReconciler) observe(ctx context.Context, service *aimv1alpha1.AIMService) (*aimServiceObservation, error) {
	obs := &aimServiceObservation{
		TemplateName: resolveTemplateName(service),
		Scope:        templateScopeNone,
	}

	// Lookup namespace-scoped template first.
	if obs.TemplateName != "" {
		var namespaceTemplate aimv1alpha1.AIMServiceTemplate
		err := r.Get(ctx, types.NamespacedName{
			Namespace: service.Namespace,
			Name:      obs.TemplateName,
		}, &namespaceTemplate)
		switch {
		case err == nil:
			obs.Scope = templateScopeNamespace
			obs.TemplateAvailable = namespaceTemplate.Status.Status == aimv1alpha1.AIMTemplateStatusAvailable
			obs.TemplateOwnedByService = hasOwnerReference(namespaceTemplate.GetOwnerReferences(), service.UID)
			if namespaceTemplate.Status.EffectiveRuntimeConfig != nil {
				obs.EffectiveRuntimeConfig = namespaceTemplate.Status.EffectiveRuntimeConfig.DeepCopy()
			}
			obs.TemplateStatus = namespaceTemplate.Status.DeepCopy()
			obs.TemplateSpecCommon = namespaceTemplate.Spec.AIMServiceTemplateSpecCommon
			obs.TemplateNamespace = namespaceTemplate.Namespace
		case apierrors.IsNotFound(err):
			// Fall through to cluster lookup.
		default:
			return nil, fmt.Errorf("failed to get AIMServiceTemplate %s/%s: %w", service.Namespace, obs.TemplateName, err)
		}
	}

	// Fallback to cluster-scoped template if namespace template not found.
	if obs.Scope == templateScopeNone && obs.TemplateName != "" {
		var clusterTemplate aimv1alpha1.AIMClusterServiceTemplate
		err := r.Get(ctx, client.ObjectKey{Name: obs.TemplateName}, &clusterTemplate)
		switch {
		case err == nil:
			obs.Scope = templateScopeCluster
			obs.TemplateAvailable = clusterTemplate.Status.Status == aimv1alpha1.AIMTemplateStatusAvailable
			if clusterTemplate.Status.EffectiveRuntimeConfig != nil {
				obs.EffectiveRuntimeConfig = clusterTemplate.Status.EffectiveRuntimeConfig.DeepCopy()
			}
			obs.TemplateStatus = clusterTemplate.Status.DeepCopy()
			obs.TemplateSpecCommon = clusterTemplate.Spec.AIMServiceTemplateSpecCommon
			obs.TemplateNamespace = ""
		case apierrors.IsNotFound(err):
			obs.ShouldCreateTemplate = true
		default:
			return nil, fmt.Errorf("failed to get AIMClusterServiceTemplate %s: %w", obs.TemplateName, err)
		}
	}

	// Fetch existing InferenceService (if any).
	var inferenceService servingv1beta1.InferenceService
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: service.Namespace,
		Name:      service.Name,
	}, &inferenceService); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get InferenceService %s/%s: %w", service.Namespace, service.Name, err)
		}
	} else {
		obs.InferenceService = &inferenceService
	}

	routingEnabled := service.Spec.Routing != nil && service.Spec.Routing.Enabled
	if routingEnabled {
		routeName := shared.InferenceServiceRouteName(service.Name)
		var route gatewayapiv1.HTTPRoute
		if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: service.Namespace}, &route); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get HTTPRoute %s/%s: %w", service.Namespace, routeName, err)
			}
		} else {
			obs.HTTPRoute = &route
		}
	}

	// If neither namespace nor cluster template was found, mark for creation.
	if !obs.templateFound() {
		obs.ShouldCreateTemplate = true
	}

	return obs, nil
}

func (r *AIMServiceReconciler) plan(_ context.Context, service *aimv1alpha1.AIMService, obs *aimServiceObservation) ([]client.Object, error) {
	var desired []client.Object

	if obs == nil {
		return desired, nil
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
	if obs.ShouldCreateTemplate || (obs.Scope == templateScopeNamespace && obs.TemplateOwnedByService) {
		template := buildDerivedTemplate(service, obs.TemplateName, ownerRef)
		desired = append(desired, template)
	}

	// Only create/update the InferenceService once the template is available.
	if obs.TemplateAvailable {
		routePath := routePathPrefix(service)
		templateState := aimstate.NewTemplateState(aimstate.TemplateState{
			Name:              obs.TemplateName,
			Namespace:         obs.TemplateNamespace,
			SpecCommon:        obs.TemplateSpecCommon,
			RuntimeConfigSpec: aimv1alpha1.AIMRuntimeConfigSpec{},
			Status:            obs.TemplateStatus,
		})
		serviceState := aimstate.NewServiceState(service, templateState, aimstate.ServiceStateOptions{
			RuntimeName: obs.runtimeName(),
			RoutePath:   routePath,
		})
		inferenceService := shared.BuildInferenceService(serviceState, ownerRef)
		desired = append(desired, inferenceService)

		if serviceState.Routing.Enabled && serviceState.Routing.GatewayRef != nil {
			route := shared.BuildInferenceServiceHTTPRoute(serviceState, ownerRef)
			desired = append(desired, route)
		}
	}

	return desired, nil
}

func evaluateHTTPRouteStatus(route *gatewayapiv1.HTTPRoute) (bool, string, string) {
	if route == nil {
		return false, aimv1alpha1.AIMServiceReasonConfiguringRoute, "HTTPRoute not found"
	}
	status := route.Status
	if len(status.Parents) == 0 {
		return false, aimv1alpha1.AIMServiceReasonConfiguringRoute, "HTTPRoute has no parent status"
	}
	for _, parent := range status.Parents {
		for _, condition := range parent.Conditions {
			if condition.Status == metav1.ConditionFalse {
				reason := condition.Reason
				if reason == "" {
					reason = aimv1alpha1.AIMServiceReasonRouteFailed
				}
				message := condition.Message
				if message == "" {
					message = "Gateway reported HTTPRoute condition false"
				}
				return false, reason, message
			}
		}
	}
	return true, aimv1alpha1.AIMServiceReasonRouteReady, "HTTPRoute is ready"
}

func routePathPrefix(service *aimv1alpha1.AIMService) string {
	return fmt.Sprintf("/%s/%s", service.Namespace, string(service.UID))
}

func evaluateRoutingStatus(
	service *aimv1alpha1.AIMService,
	obs *aimServiceObservation,
	status *aimv1alpha1.AIMServiceStatus,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) (enabled bool, ready bool) {
	routingEnabled := service.Spec.Routing != nil && service.Spec.Routing.Enabled
	if !routingEnabled {
		setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRouteReady, "Routing disabled")
		return false, true
	}

	status.Routing = &aimv1alpha1.AIMServiceRoutingStatus{
		Path: routePathPrefix(service),
	}

	if service.Spec.Routing.GatewayRef == nil {
		setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonRouteFailed,
			"routing.gatewayRef must be specified when routing is enabled")
		status.Status = aimv1alpha1.AIMServiceStatusFailed
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRouteFailed,
			"Routing gateway reference is missing")
		return true, false
	}

	if obs == nil || obs.HTTPRoute == nil {
		setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonConfiguringRoute,
			"Waiting for HTTPRoute to be created")
		return true, false
	}

	ready, reason, message := evaluateHTTPRouteStatus(obs.HTTPRoute)
	conditionStatus := metav1.ConditionFalse
	if ready {
		conditionStatus = metav1.ConditionTrue
	}
	setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, conditionStatus, reason, message)
	if !ready && reason == aimv1alpha1.AIMServiceReasonRouteFailed {
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	}
	return true, ready
}

func (r *AIMServiceReconciler) projectStatus(
	_ context.Context,
	service *aimv1alpha1.AIMService,
	obs *aimServiceObservation,
	errs framework.ReconcileErrors,
) error {
	status := &service.Status
	status.EffectiveRuntimeConfig = nil
	status.Routing = nil

	if obs != nil && obs.EffectiveRuntimeConfig != nil {
		status.EffectiveRuntimeConfig = obs.EffectiveRuntimeConfig.DeepCopy()
	}

	// Helper to update status conditions.
	setCondition := func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
		cond := metav1.Condition{
			Type:               conditionType,
			Status:             conditionStatus,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: service.Generation,
			LastTransitionTime: metav1.Now(),
		}
		meta.SetStatusCondition(&status.Conditions, cond)
	}

	// Default cache condition based on spec.CacheModel.
	if !service.Spec.CacheModel {
		setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCacheWarm, "Caching not requested")
	} else {
		setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonWaitingForCache, "Waiting for cache warm-up")
	}

	routingEnabled, routingReady := evaluateRoutingStatus(service, obs, status, setCondition)

	if errs.HasError() {
		status.Status = aimv1alpha1.AIMServiceStatusFailed

		reason := aimv1alpha1.AIMServiceReasonValidationFailed
		message := "Reconciliation failed"
		switch {
		case errs.ObserveErr != nil:
			message = fmt.Sprintf("Observation failed: %v", errs.ObserveErr)
		case errs.PlanErr != nil:
			message = fmt.Sprintf("Planning failed: %v", errs.PlanErr)
		case errs.ApplyErr != nil:
			reason = aimv1alpha1.AIMServiceReasonRuntimeFailed
			message = fmt.Sprintf("Apply failed: %v", errs.ApplyErr)
		}

		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
		setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, reason, "Template resolution pending due to reconciliation failure")
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, message)
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Reconciliation halted due to failure")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
		return nil
	}

	// Clear failure condition when reconciliation succeeds.
	if !routingEnabled || routingReady {
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonResolved, "No active failures")
	}

	if obs == nil || !obs.templateFound() {
		status.Status = aimv1alpha1.AIMServiceStatusPending
		setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound,
			fmt.Sprintf("AIMServiceTemplate %q not found; will create default template", resolveTemplateName(service)))
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime, "Waiting for template creation")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Waiting for template to be created")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Template missing")
		return nil
	}

	setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonResolved,
		fmt.Sprintf("Resolved template %q", obs.TemplateName))

	// Check if template is degraded
	if obs.TemplateStatus != nil && obs.TemplateStatus.Status == aimv1alpha1.AIMTemplateStatusDegraded {
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
		templateReason := "TemplateDegraded"
		templateMessage := fmt.Sprintf("Template %q is degraded", obs.TemplateName)

		// Extract failure reason from template conditions if available
		for _, cond := range obs.TemplateStatus.Conditions {
			if cond.Type == "Failure" && cond.Status == metav1.ConditionTrue {
				templateMessage = fmt.Sprintf("Template %q is degraded: %s", obs.TemplateName, cond.Message)
				if cond.Reason != "" {
					templateReason = cond.Reason
				}
				break
			}
		}

		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, templateReason, templateMessage)
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, templateReason,
			fmt.Sprintf("Cannot create InferenceService: %s", templateMessage))
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, templateReason,
			"Service is degraded due to template issues")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, templateReason,
			"Service cannot be ready due to degraded template")
		return nil
	}

	if !obs.TemplateAvailable {
		status.Status = aimv1alpha1.AIMServiceStatusStarting
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			fmt.Sprintf("Template %q is not yet Available", obs.TemplateName))
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"Waiting for template discovery to complete")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"Template is not available")
		return nil
	}

	if service.Spec.CacheModel {
		setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCacheWarm, "Template caching enabled")
	}

	if obs.InferenceService == nil {
		if status.Status != aimv1alpha1.AIMServiceStatusFailed {
			status.Status = aimv1alpha1.AIMServiceStatusStarting
		}
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"Waiting for InferenceService creation")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"Reconciling InferenceService resources")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"InferenceService not yet created")
		return nil
	}

	if obs.InferenceService.Status.IsReady() && routingReady {
		status.Status = aimv1alpha1.AIMServiceStatusRunning
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRuntimeReady,
			"InferenceService is ready")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonRuntimeReady,
			"Service is running")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRuntimeReady,
			"AIMService is ready to serve traffic")
		return nil
	}

	if status.Status != aimv1alpha1.AIMServiceStatusFailed && status.Status != aimv1alpha1.AIMServiceStatusDegraded {
		status.Status = aimv1alpha1.AIMServiceStatusStarting
	}
	reason := aimv1alpha1.AIMServiceReasonCreatingRuntime
	message := "Waiting for InferenceService to become ready"
	if obs.InferenceService.Status.ModelStatus.LastFailureInfo != nil {
		reason = aimv1alpha1.AIMServiceReasonRuntimeFailed
		message = obs.InferenceService.Status.ModelStatus.LastFailureInfo.Message
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	}
	if routingEnabled && !routingReady && reason == aimv1alpha1.AIMServiceReasonCreatingRuntime {
		reason = aimv1alpha1.AIMServiceReasonConfiguringRoute
		message = "Waiting for HTTPRoute to become ready"
	}

	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, reason, "InferenceService reconciliation in progress")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)

	return nil
}

func (r *AIMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &aimv1alpha1.AIMService{}, aimServiceTemplateIndexKey, func(obj client.Object) []string {
		service, ok := obj.(*aimv1alpha1.AIMService)
		if !ok {
			return nil
		}
		if service.Spec.TemplateRef == "" {
			return nil
		}
		return []string{service.Spec.TemplateRef}
	}); err != nil {
		return err
	}

	templateHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		template, ok := obj.(*aimv1alpha1.AIMServiceTemplate)
		if !ok {
			return nil
		}

		var services aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &services,
			client.InNamespace(template.Namespace),
			client.MatchingFields{aimServiceTemplateIndexKey: template.Name},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMServiceTemplate", "template", template.Name)
			return nil
		}

		return requestsForServices(services.Items)
	})

	clusterTemplateHandler := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		clusterTemplate, ok := obj.(*aimv1alpha1.AIMClusterServiceTemplate)
		if !ok {
			return nil
		}

		var services aimv1alpha1.AIMServiceList
		if err := r.List(ctx, &services,
			client.MatchingFields{aimServiceTemplateIndexKey: clusterTemplate.Name},
		); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list AIMServices for AIMClusterServiceTemplate", "template", clusterTemplate.Name)
			return nil
		}

		return requestsForServices(services.Items)
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

func buildDerivedTemplate(service *aimv1alpha1.AIMService, templateName string, ownerRef metav1.OwnerReference) *aimv1alpha1.AIMServiceTemplate {
	template := &aimv1alpha1.AIMServiceTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: aimv1alpha1.GroupVersion.String(),
			Kind:       "AIMServiceTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            templateName,
			Namespace:       service.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: aimv1alpha1.AIMServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
				AIMImageName:         service.Spec.AIMImageName,
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{},
				RuntimeConfigName:    shared.NormalizeRuntimeConfigName(service.Spec.RuntimeConfigName),
			},
			Env:              shared.CopyEnvVars(service.Spec.Env),
			ImagePullSecrets: shared.CopyPullSecrets(service.Spec.ImagePullSecrets),
		},
	}

	if service.Spec.Overrides != nil {
		template.Spec.AIMRuntimeParameters = service.Spec.Overrides.AIMRuntimeParameters
	}

	if service.Spec.CacheModel {
		template.Spec.Caching = &aimv1alpha1.AIMTemplateCachingConfig{
			Enabled: service.Spec.CacheModel,
			Env:     shared.CopyEnvVars(service.Spec.Env),
		}
	}

	return template
}

func resolveTemplateName(service *aimv1alpha1.AIMService) string {
	if service.Spec.TemplateRef != "" {
		return service.Spec.TemplateRef
	}
	return service.Name
}

func hasOwnerReference(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func requestsForServices(services []aimv1alpha1.AIMService) []reconcile.Request {
	if len(services) == 0 {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(services))
	for _, svc := range services {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: svc.Namespace,
				Name:      svc.Name,
			},
		})
	}
	return requests
}
