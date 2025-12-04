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
	"errors"
	"fmt"

	"github.com/silogen/kaiwo/internal/controller/aim/routingconfig"

	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
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
		// Check if the HTTPRoute is accepted by this parent gateway
		acceptedCond := meta.FindStatusCondition(parent.Conditions, string(gatewayapiv1.RouteConditionAccepted))
		if acceptedCond == nil {
			return false, aimv1alpha1.AIMServiceReasonConfiguringRoute, "HTTPRoute Accepted condition not found"
		}
		if acceptedCond.Status != metav1.ConditionTrue {
			reason := acceptedCond.Reason
			if reason == "" {
				reason = aimv1alpha1.AIMServiceReasonRouteFailed
			}
			message := acceptedCond.Message
			if message == "" {
				message = "HTTPRoute not accepted by gateway"
			}
			return false, reason, message
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

// HandleModelResolutionFailure checks for model resolution failures and updates status.
// Returns true if model resolution failed.
func HandleModelResolutionFailure(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.ModelResolutionErr == nil {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusFailed
	message := obs.ModelResolutionErr.Error()
	reason := "ModelResolutionFailed"

	// Check if the error is due to multiple models being found
	if errors.Is(obs.ModelResolutionErr, ErrMultipleModelsFound) {
		reason = "MultipleModelsFound"
	}

	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, reason, "Cannot resolve model for service")
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, "Cannot proceed without resolved model")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Model resolution failed")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
	return true
}

// HandleImageMissing checks for missing image and updates status.
// Returns true if the image is missing.
func HandleImageMissing(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.ImageErr == nil {
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	message := obs.ImageErr.Error()
	reason := "ImageNotFound"
	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, "Cannot create InferenceService without image")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Image is missing")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
	return true
}

// HandleImageNotReady checks if the resolved image is not yet ready and updates status.
// Returns true if the service should wait for the image to become ready.
func HandleImageNotReady(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs == nil || obs.ImageReady || obs.ImageReadyReason == "" {
		return false
	}

	message := obs.ImageReadyMessage
	if message == "" {
		message = "Image is not ready"
	}

	// Set status based on the reason
	// ModelFailed is a terminal error (e.g., image not found 404) - cascade to Failed
	// ModelDegraded is a recoverable error (e.g., auth issues) - cascade to Degraded
	// Other reasons (e.g., model pending) - set to Pending
	switch obs.ImageReadyReason {
	case "ModelFailed":
		status.Status = aimv1alpha1.AIMServiceStatusFailed
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, obs.ImageReadyReason, message)
	case "ModelDegraded":
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, obs.ImageReadyReason, message)
	default:
		status.Status = aimv1alpha1.AIMServiceStatusPending
	}

	setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, obs.ImageReadyReason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, obs.ImageReadyReason, "Waiting for image readiness")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, obs.ImageReadyReason, "Awaiting image readiness")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, obs.ImageReadyReason, message)
	return true
}

// HandlePathTemplateError checks for path template errors and updates status.
// Returns true if there is a path template error.
// This can occur when routing is enabled (via service spec or runtime config) but the path template is invalid.
func HandlePathTemplateError(
	status *aimv1alpha1.AIMServiceStatus,
	service *aimv1alpha1.AIMService,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs == nil || obs.PathTemplateErr == nil {
		return false
	}

	// Check if routing is enabled (via service spec or runtime config)
	var runtimeRouting *aimv1alpha1.AIMRuntimeRoutingConfig
	if obs != nil {
		runtimeRouting = obs.RuntimeConfigSpec.Routing
	}
	resolved := routingconfig.Resolve(service, runtimeRouting)
	if !resolved.Enabled {
		// Path template error doesn't matter if routing is disabled
		return false
	}

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	message := obs.PathTemplateErr.Error()
	reason := aimv1alpha1.AIMServiceReasonPathTemplateInvalid
	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, "Cannot configure HTTP routing")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Path template is invalid")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
	return true
}

// HandleTemplateDegraded checks if the template is degraded, not available, or failed and updates status.
// Returns true if the template is degraded, not available, or failed.
func HandleTemplateDegraded(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.TemplateStatus == nil {
		return false
	}

	// Handle Degraded, NotAvailable, and Failed template statuses
	if obs.TemplateStatus.Status != aimv1alpha1.AIMTemplateStatusDegraded &&
		obs.TemplateStatus.Status != aimv1alpha1.AIMTemplateStatusNotAvailable &&
		obs.TemplateStatus.Status != aimv1alpha1.AIMTemplateStatusFailed {
		return false
	}

	// Use Failed for terminal failures, Degraded for recoverable issues (including NotAvailable)
	if obs.TemplateStatus.Status == aimv1alpha1.AIMTemplateStatusFailed {
		status.Status = aimv1alpha1.AIMServiceStatusFailed
	} else {
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
	}

	templateReason := "TemplateDegraded"
	templateMessage := "Template is not available"

	// Extract failure details from template conditions
	for _, cond := range obs.TemplateStatus.Conditions {
		if cond.Type == "Failure" && cond.Status == metav1.ConditionTrue {
			if cond.Message != "" {
				templateMessage = cond.Message
			}
			if cond.Reason != "" {
				templateReason = cond.Reason
			}
			break
		}
	}

	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, templateReason, templateMessage)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, templateReason,
		"Cannot create InferenceService due to template issues")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, templateReason,
		"Service cannot proceed due to template issues")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, templateReason,
		"Service cannot be ready due to template issues")
	return true
}

// HandleTemplateNotAvailable checks if the template is not available and updates status.
// Returns true if the template is not yet available (Pending or Progressing).
// Sets the service to Pending state because it's waiting for a dependency (the template).
func HandleTemplateNotAvailable(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs.TemplateAvailable {
		return false
	}

	// Service is Pending because it's waiting for the template to become available.
	// The template itself may be Progressing (running discovery) or Pending.
	status.Status = aimv1alpha1.AIMServiceStatusPending

	reason := "TemplateNotAvailable"
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason,
		fmt.Sprintf("Template %q is not yet Available", obs.TemplateName))
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, reason,
		"Waiting for template discovery to complete")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason,
		"Template is not available")
	return true
}

// HandleTemplateSelectionFailure reports failures during automatic template selection.
func HandleTemplateSelectionFailure(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs == nil || obs.TemplateSelectionReason == "" {
		return false
	}

	message := obs.TemplateSelectionMessage
	if message == "" {
		message = "Template selection failed"
	}

	status.Status = aimv1alpha1.AIMServiceStatusFailed
	reason := obs.TemplateSelectionReason
	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, reason, message)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, reason, "Cannot proceed without a unique template")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, reason, "Template selection failed")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, reason, message)
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

func HandleModelCacheReadiness(service *aimv1alpha1.AIMService, status *aimv1alpha1.AIMServiceStatus, obs *ServiceObservation, setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string)) bool {
	if obs == nil || obs.ModelCaches == nil {
		return false
	}

	if status.ModelCaches == nil {
		status.ModelCaches = make(map[string]aimv1alpha1.AIMResolvedModelCache)
	}

	// If we use model caching, display the state of the caches - and add mount if it's present
	if obs.TemplateCache != nil {
		// Add status of model caches from the template cache if they are not already mounted
		for mcName, mc := range obs.TemplateCache.Status.ModelCaches {
			if obs.InferenceService != nil {
				if entry, ok := status.ModelCaches[mcName]; ok {
					if entry.MountPoint != "" {
						// Model cache already mounted, don't change
						continue
					}
				}
			}
			// We didn't find a mounted cache, copy info from template cache
			status.ModelCaches[mcName] = mc
		}

		// Go through all mounted models and attach the mount point to the model cache status if present
		if obs.InferenceService != nil && obs.InferenceService.Spec.Predictor.Model != nil {
			for _, model := range obs.InferenceService.Spec.Predictor.Model.VolumeMounts {
				if entry, ok := status.ModelCaches[model.Name]; ok {
					entry.MountPoint = model.MountPath
					status.ModelCaches[model.Name] = entry
				}
			}
		}

	}

	if service.Spec.CacheModel || obs.TemplateCache != nil {
		// We are trying to use a template cache - check if the cache is warm, failed or pending
		if obs.TemplateCache != nil && obs.TemplateCache.Status.Status == aimv1alpha1.AIMTemplateCacheStatusAvailable {
			setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCacheWarm, "Template cache is warm")
			setCondition(aimv1alpha1.AIMServiceConditionCacheFailed, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCacheFailed, "Template cache is warm")
		} else if obs.TemplateCache != nil && obs.TemplateCache.Status.Status == aimv1alpha1.AIMTemplateCacheStatusFailed {
			setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCacheWarm, "Template cache failed")
			setCondition(aimv1alpha1.AIMServiceConditionCacheFailed, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCacheFailed, "Template cache failed")
			status.Status = aimv1alpha1.AIMServiceStatusFailed
			return true
		} else {
			setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCacheWarm, "Template caching is enabled")
			setCondition(aimv1alpha1.AIMServiceConditionCacheFailed, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCacheFailed, "Template caching is enabled")
		}
	}

	return false
}

// HandleInferenceServicePodImageError checks for image pull errors in InferenceService pods.
// Returns true if an image pull error was detected.
func HandleInferenceServicePodImageError(
	status *aimv1alpha1.AIMServiceStatus,
	obs *ServiceObservation,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs == nil || obs.InferenceServicePodImageError == nil {
		return false
	}

	pullErr := obs.InferenceServicePodImageError

	// Determine the condition reason based on error type
	var conditionReason string
	switch pullErr.Type {
	case ImagePullErrorAuth:
		conditionReason = aimv1alpha1.AIMServiceReasonImagePullAuthFailure
	case ImagePullErrorNotFound:
		conditionReason = aimv1alpha1.AIMServiceReasonImageNotFound
	default:
		conditionReason = aimv1alpha1.AIMServiceReasonImagePullBackOff
	}

	// Format detailed message
	containerType := "Container"
	if pullErr.IsInitContainer {
		containerType = "Init container"
	}
	detailedMessage := fmt.Sprintf("InferenceService pod %s %q is stuck in %s: %s",
		containerType, pullErr.Container, pullErr.Reason, pullErr.Message)

	status.Status = aimv1alpha1.AIMServiceStatusDegraded
	setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, conditionReason, detailedMessage)
	setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, conditionReason,
		"InferenceService cannot run due to image pull failure")
	setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, conditionReason,
		"Service is degraded due to image pull failure")
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, conditionReason,
		"Service cannot be ready due to image pull failure")
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

// initializeStatusReferences resets and populates resolved references in status.
func initializeStatusReferences(status *aimv1alpha1.AIMServiceStatus, obs *ServiceObservation) {
	status.ResolvedRuntimeConfig = nil
	status.ResolvedImage = nil
	status.Routing = nil
	status.ResolvedTemplateCache = nil

	if obs != nil && obs.ResolvedRuntimeConfig != nil {
		status.ResolvedRuntimeConfig = obs.ResolvedRuntimeConfig
	}
	if obs != nil && obs.ResolvedImage != nil {
		status.ResolvedImage = obs.ResolvedImage
	}
	if obs != nil && obs.TemplateCache != nil {
		status.ResolvedTemplateCache = &aimv1alpha1.AIMResolvedReference{
			Name:      obs.TemplateCache.Name,
			Namespace: obs.TemplateCache.Namespace,
			Kind:      "AIMTemplateCache",
			Scope: func() aimv1alpha1.AIMResolutionScope {
				if obs.TemplateCache.Namespace != "" {
					return aimv1alpha1.AIMResolutionScopeNamespace
				}
				return aimv1alpha1.AIMResolutionScopeCluster
			}(),
			UID: obs.TemplateCache.UID,
		}
	}
}

// setupCacheCondition sets the cache condition based on whether caching is requested.
func setupCacheCondition(
	service *aimv1alpha1.AIMService,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) {
	if !service.Spec.CacheModel {
		setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonCacheWarm, "Caching not requested")
	} else {
		setCondition(aimv1alpha1.AIMServiceConditionCacheReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonWaitingForCache, "Waiting for cache warm-up")
	}
}

// setupResolvedTemplate populates the resolved template reference in status.
func setupResolvedTemplate(obs *ServiceObservation, status *aimv1alpha1.AIMServiceStatus) {
	status.ResolvedTemplate = nil
	if obs != nil && obs.TemplateName != "" {
		status.ResolvedTemplate = &aimv1alpha1.AIMServiceResolvedTemplate{
			AIMResolvedReference: aimv1alpha1.AIMResolvedReference{
				Name:      obs.TemplateName,
				Namespace: obs.TemplateNamespace,
				Scope:     convertTemplateScope(obs.Scope),
				Kind:      "AIMServiceTemplate",
			},
			Profile: aimv1alpha1.AIMServiceResolvedTemplateProfile{
				Metadata: obs.TemplateStatus.Profile.Metadata,
			},
		}
	}
	// Don't set resolvedTemplate if no template was actually resolved
}

// evaluateHTTPRouteReadiness checks HTTP route status and updates routing conditions.
// Returns the updated routingReady flag.
func evaluateHTTPRouteReadiness(
	httpRoute *gatewayapiv1.HTTPRoute,
	status *aimv1alpha1.AIMServiceStatus,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if httpRoute == nil {
		setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonConfiguringRoute,
			"Waiting for HTTPRoute to be created")
		return false
	}

	ready, reason, message := EvaluateHTTPRouteStatus(httpRoute)
	conditionStatus := metav1.ConditionFalse
	if ready {
		conditionStatus = metav1.ConditionTrue
	}
	setCondition(aimv1alpha1.AIMServiceConditionRoutingReady, conditionStatus, reason, message)
	if !ready && reason == aimv1alpha1.AIMServiceReasonRouteFailed {
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, reason, message)
	}
	return ready
}

// handleTemplateNotFound handles the case when no template is found.
// Returns true if this handler applies.
func handleTemplateNotFound(
	obs *ServiceObservation,
	status *aimv1alpha1.AIMServiceStatus,
	setCondition func(conditionType string, conditionStatus metav1.ConditionStatus, reason, message string),
) bool {
	if obs != nil && obs.TemplateFound() {
		return false
	}

	var message string
	if obs != nil && obs.ShouldCreateTemplate {
		status.Status = aimv1alpha1.AIMServiceStatusPending
		message = "Template not found; creating derived template"
		setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, message)
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonCreatingRuntime, "Waiting for template creation")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Waiting for template to be created")
	} else if obs != nil && obs.TemplatesExistButNotReady {
		// Templates exist but aren't Available yet - service should wait
		status.Status = aimv1alpha1.AIMServiceStatusPending
		message = "Waiting for templates to become Available"
		setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, "TemplateNotAvailable", message)
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, "TemplateNotAvailable", "Waiting for template discovery to complete")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionTrue, "TemplateNotAvailable", "Templates exist but are not yet Available")
	} else {
		// No template could be resolved and no derived template will be created.
		// This is a degraded state - the service cannot proceed.
		status.Status = aimv1alpha1.AIMServiceStatusDegraded
		if obs != nil {
			switch {
			case obs.TemplateSelectionMessage != "":
				message = obs.TemplateSelectionMessage
			case obs.BaseTemplateName == "":
				message = "No template reference specified and no templates are available for the selected image. Provide spec.templateRef or create templates for the image."
			default:
				message = fmt.Sprintf("Template %q not found. Create the template or verify the template name.", obs.BaseTemplateName)
			}
		} else {
			message = "Template not found"
		}
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonTemplateNotFound, message)
		setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, message)
		setCondition(aimv1alpha1.AIMServiceConditionRuntimeReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Referenced template does not exist")
		setCondition(aimv1alpha1.AIMServiceConditionProgressing, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Cannot proceed without template")
	}
	setCondition(aimv1alpha1.AIMServiceConditionReady, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonTemplateNotFound, "Template missing")
	return true
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
	initializeStatusReferences(status, obs)

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

	setupCacheCondition(service, setCondition)
	setupResolvedTemplate(obs, status)

	routingEnabled, routingReady, routingHasFatalError := EvaluateRoutingStatus(service, obs, status, setCondition)

	// Check routing readiness if enabled (but skip if we already have a fatal routing error)
	if routingEnabled && !routingHasFatalError {
		routingReady = evaluateHTTPRouteReadiness(httpRoute, status, setCondition)
	}

	if HandleReconcileErrors(status, setCondition, errs) {
		return
	}

	// Clear failure condition when reconciliation succeeds and there are no routing errors.
	if !routingEnabled || (routingReady && !routingHasFatalError) {
		setCondition(aimv1alpha1.AIMServiceConditionFailure, metav1.ConditionFalse, aimv1alpha1.AIMServiceReasonResolved, "No active failures")
	}

	if HandleModelResolutionFailure(status, obs, setCondition) {
		return
	}

	if HandleImageMissing(status, obs, setCondition) {
		return
	}

	if HandleImageNotReady(status, obs, setCondition) {
		return
	}

	if HandleTemplateSelectionFailure(status, obs, setCondition) {
		return
	}

	if handleTemplateNotFound(obs, status, setCondition) {
		return
	}

	setCondition(aimv1alpha1.AIMServiceConditionResolved, metav1.ConditionTrue, aimv1alpha1.AIMServiceReasonResolved,
		fmt.Sprintf("Resolved template %q", obs.TemplateName))

	if HandleRuntimeConfigMissing(status, obs, setCondition) {
		return
	}

	if HandlePathTemplateError(status, service, obs, setCondition) {
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

	// Check for image pull errors in InferenceService pods
	if HandleInferenceServicePodImageError(status, obs, setCondition) {
		return
	}

	// Check for model cache readiness
	if HandleModelCacheReadiness(service, status, obs, setCondition) {
		return
	}

	EvaluateInferenceServiceStatus(status, obs, inferenceService, httpRoute, routingEnabled, routingReady, setCondition)
}
