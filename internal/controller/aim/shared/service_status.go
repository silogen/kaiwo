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

package shared

import (
	"fmt"
	"strings"

	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/routingconfig"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

// EvaluateHTTPRouteStatus checks the HTTPRoute status and returns readiness state.
func EvaluateHTTPRouteStatus(route *gatewayapiv1.HTTPRoute) (bool, string, string) {
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

// EvaluateRoutingStatus checks routing configuration and updates status accordingly.
// Returns (enabled, ready, hasFatalError) to indicate if routing is enabled, if it's ready, and if there's a terminal error.
func EvaluateRoutingStatus(
	service *aimv1alpha1.AIMService,
	obs *ServiceObservation,
	status *aimv1alpha1.AIMServiceStatus,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) (enabled bool, ready bool, hasFatalError bool) {
	var runtimeRouting *aimv1alpha1.AIMRuntimeRoutingConfig
	if obs != nil {
		runtimeRouting = obs.RuntimeConfigSpec.Routing
	}

	resolved := routingconfig.Resolve(service, runtimeRouting)
	if !resolved.Enabled {
		setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRouteReady, "Routing disabled")
		return false, true, false
	}

	routePath := DefaultRoutePath(service)
	if obs != nil && obs.RoutePath != "" {
		routePath = obs.RoutePath
	}

	status.Routing = &aimv1alpha1.AIMServiceRoutingStatus{
		Path: routePath,
	}

	if resolved.GatewayRef == nil {
		message := "routing.gatewayRef must be specified via AIMService or runtime config when routing is enabled"
		setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonRouteFailed, message)
		status.Status = aimv1alpha1.AIMServiceStatusFailed
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRouteFailed,
			"Routing gateway reference is missing")
		return true, false, true
	}

	return true, false, false
}

// HandleReconcileErrors processes reconciliation errors and updates service status.
// Returns true if errors were found and handled.
func HandleReconcileErrors(
	status *aimv1alpha1.AIMServiceStatus,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
	errs controllerutils.ReconcileErrors,
) bool {
	if !errs.HasError() {
		return false
	}

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
	return true
}

// HandleRuntimeConfigMissing checks for missing runtime config and updates status.
// Returns true if the runtime config is missing.
func HandleRuntimeConfigMissing(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.RuntimeConfigErr == nil {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	message := obs.RuntimeConfigErr.Error()
	reason := aimv1alpha1.AIMServiceReasonRuntimeConfigMissing
	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, "Cannot configure runtime without AIMRuntimeConfig")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Runtime configuration is missing")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
	return true
}

// HandleRouteTemplateError checks for route template errors and updates status.
// Returns true if there is a route template error.
func HandleRouteTemplateError(
	status *aimv1alpha1.AIMServiceStatus,
	service *aimv1alpha1.AIMService,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if service.Spec.Routing == nil || !service.Spec.Routing.Enabled {
		return false
	}
	if obs == nil || obs.RouteTemplateErr == nil {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	message := obs.RouteTemplateErr.Error()
	reason := aimv1alpha1.AIMServiceReasonRouteTemplateInvalid
	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, "Cannot configure HTTP routing")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Routing template is invalid")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
	return true
}

// HandleTemplateDegraded checks if the template is degraded and updates status.
// Returns true if the template is degraded.
func HandleTemplateDegraded(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.TemplateStatus == nil || obs.TemplateStatus.Status != aimv1alpha1.AIMTemplateStatusDegraded {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	templateReason := "TemplateDegraded"
	templateMessage := fmt.Sprintf("Template %q is degraded", obs.TemplateName)

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
	return true
}

// HandleTemplateNotAvailable checks if the template is not available and updates status.
// Returns true if the template is not yet available.
func HandleTemplateNotAvailable(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.TemplateAvailable {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusStarting
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
		fmt.Sprintf("Template %q is not yet Available", obs.TemplateName))
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCreatingRuntime,
		"Waiting for template discovery to complete")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
		"Template is not available")
	return true
}

// HandleMissingModelSource checks if the template is available but has no model sources.
// Returns true if model sources are missing (discovery succeeded but produced no usable sources).
func HandleMissingModelSource(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if !obs.TemplateAvailable || obs.TemplateStatus == nil {
		return false
	}

	// Check if template is Available but has no model sources
	hasModelSources := len(obs.TemplateStatus.ModelSources) > 0
	if hasModelSources {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	reason := "NoModelSources"
	message := fmt.Sprintf("Template %q is Available but discovery produced no usable model sources", obs.TemplateName)

	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason,
		"Cannot create InferenceService without model sources")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason,
		"Service is degraded due to missing model sources")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason,
		"Service cannot be ready without model sources")
	return true
}

// EvaluateInferenceServiceStatus checks InferenceService and routing readiness.
// Updates status conditions based on the InferenceService and routing state.
func EvaluateInferenceServiceStatus(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	inferenceService *servingv1beta1.InferenceService,
	httpRoute *gatewayapiv1.HTTPRoute,
	routingEnabled bool,
	routingReady bool,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) {
	if inferenceService == nil {
		if status.Status != aimv1alpha1.AIMServiceStatusFailed {
			status.Status = aimv1alpha1.AIMServiceStatusStarting
		}
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"Waiting for InferenceService creation")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"Reconciling InferenceService resources")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime,
			"InferenceService not yet created")
		return
	}

	if inferenceService.Status.IsReady() && routingReady {
		status.Status = aimv1alpha1.AIMServiceStatusRunning
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRuntimeReady,
			"InferenceService is ready")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonRuntimeReady,
			"Service is running")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonRuntimeReady,
			"AIMService is ready to serve traffic")
		return
	}

	if status.Status != aimv1alpha1.AIMServiceStatusFailed && status.Status != aimv1alpha1.AIMServiceStatusDegraded {
		status.Status = aimv1alpha1.AIMServiceStatusStarting
	}
	reason := aimv1alpha1.AIMServiceReasonCreatingRuntime
	message := "Waiting for InferenceService to become ready"
	if inferenceService.Status.ModelStatus.LastFailureInfo != nil {
		reason = aimv1alpha1.AIMServiceReasonRuntimeFailed
		message = inferenceService.Status.ModelStatus.LastFailureInfo.Message
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
}

func convertTemplateScope(scope TemplateScope) aimv1alpha1.AIMResolutionScope {
	switch scope {
	case TemplateScopeNamespace:
		return aimv1alpha1.AIMResolutionScopeNamespace
	case TemplateScopeCluster:
		return aimv1alpha1.AIMResolutionScopeCluster
	default:
		return aimv1alpha1.AIMResolutionScopeUnknown
	}
}

// ProjectServiceStatus computes and updates the service status based on observations and errors.
// This is a high-level orchestrator that calls the individual status handler functions.
func ProjectServiceStatus(
	service *aimv1alpha1.AIMService,
	obs *ServiceObservation,
	inferenceService *servingv1beta1.InferenceService,
	httpRoute *gatewayapiv1.HTTPRoute,
	errs controllerutils.ReconcileErrors,
) {
	status := &service.Status
	status.ResolvedRuntimeConfig = nil
	status.ResolvedImage = nil
	status.Routing = nil

	if obs != nil && obs.ResolvedRuntimeConfig != nil {
		status.ResolvedRuntimeConfig = obs.ResolvedRuntimeConfig
	}
	if obs != nil && obs.ResolvedImage != nil {
		status.ResolvedImage = obs.ResolvedImage
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

	status.ResolvedTemplate = nil
	if obs != nil && obs.TemplateName != "" {
		status.ResolvedTemplate = &aimv1alpha1.AIMServiceResolvedTemplate{
			AIMResolvedReference: aimv1alpha1.AIMResolvedReference{
				Name:      obs.TemplateName,
				Namespace: obs.TemplateNamespace,
				Scope:     convertTemplateScope(obs.Scope),
				Kind:      "AIMServiceTemplate",
			},
		}
	} else if name := strings.TrimSpace(TemplateNameFromSpec(service)); name != "" {
		status.ResolvedTemplate = &aimv1alpha1.AIMServiceResolvedTemplate{
			AIMResolvedReference: aimv1alpha1.AIMResolvedReference{
				Name:  name,
				Scope: aimv1alpha1.AIMResolutionScopeUnknown,
				Kind:  "AIMServiceTemplate",
			},
		}
	}

	routingEnabled, routingReady, routingHasFatalError := EvaluateRoutingStatus(service, obs, status, setCondition)

	// Check routing readiness if enabled (but skip if we already have a fatal routing error)
	if routingEnabled && !routingHasFatalError {
		if httpRoute == nil {
			setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonConfiguringRoute,
				"Waiting for HTTPRoute to be created")
			routingReady = false
		} else {
			ready, reason, message := EvaluateHTTPRouteStatus(httpRoute)
			conditionStatus := metav1.ConditionFalse
			if ready {
				conditionStatus = metav1.ConditionTrue
				routingReady = true
			}
			setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, conditionStatus, reason, message)
			if !ready && reason == aimv1alpha1.AIMServiceReasonRouteFailed {
				status.Status = aimv1alpha1.AIMServiceStatusDegraded
				setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
			}
		}
	}

	if HandleReconcileErrors(status, setCondition, errs) {
		return
	}

	// Clear failure condition when reconciliation succeeds and there are no routing errors.
	if !routingEnabled || (routingReady && !routingHasFatalError) {
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonResolved, "No active failures")
	}

	if obs == nil || !obs.TemplateFound() {
		status.Status = aimv1alpha1.AIMServiceStatusPending
		setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound,
			fmt.Sprintf("AIMServiceTemplate %q not found; will create default template", TemplateNameFromSpec(service)))
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime, "Waiting for template creation")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Waiting for template to be created")
		setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Template missing")
		return
	}

	setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonResolved,
		fmt.Sprintf("Resolved template %q", obs.TemplateName))

	if HandleRuntimeConfigMissing(status, obs, setCondition) {
		return
	}

	if HandleRouteTemplateError(status, service, obs, setCondition) {
		return
	}

	if HandleTemplateDegraded(status, obs, setCondition) {
		return
	}

	if HandleTemplateNotAvailable(status, obs, setCondition) {
		return
	}

	if HandleMissingModelSource(status, obs, setCondition) {
		return
	}

	if service.Spec.CacheModel {
		setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCacheWarm, "Template caching enabled")
	}

	EvaluateInferenceServiceStatus(status, obs, inferenceService, httpRoute, routingEnabled, routingReady, setCondition)
}
