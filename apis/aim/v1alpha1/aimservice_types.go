// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// AIMServiceModel specifies which model to deploy. Exactly one field must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.ref) && !has(self.image)) || (!has(self.ref) && has(self.image))",message="exactly one of ref or image must be specified"
type AIMServiceModel struct {
	// Ref references an existing AIMModel or AIMClusterModel by metadata.name.
	// The controller looks for a namespace-scoped AIMModel first, then falls back to cluster-scoped AIMClusterModel.
	// Example: `meta-llama-3-8b`.
	// +optional
	Ref *string `json:"ref,omitempty"`

	// Image specifies a container image URI directly.
	// The controller searches for an existing model with this image, or creates one if none exists.
	// The scope of the created model is controlled by the runtime config's ModelCreationScope field.
	// Example: `ghcr.io/silogen/llama-3-8b:v1.2.0`.
	// +optional
	Image *string `json:"image,omitempty"`
}

// AIMServiceOverrides allows overriding template parameters at the service level.
// All fields are optional. When specified, they override the corresponding values
// from the referenced AIMServiceTemplate.
type AIMServiceOverrides struct {
	AIMRuntimeParameters `json:",inline"`
}

// AIMServiceSpec defines the desired state of AIMService.
//
// Binds a canonical model to an AIMServiceTemplate and configures replicas,
// caching behavior, and optional overrides. The template governs the base
// runtime selection knobs, while the overrides field allows service-specific
// customization.
type AIMServiceSpec struct {
	// Model specifies which model to deploy using one of the available reference methods.
	// Use `ref` to reference an existing AIMModel/AIMClusterModel by name, or use `image`
	// to specify a container image URI directly (which will auto-create a model if needed).
	Model AIMServiceModel `json:"model"`

	// TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to use.
	// The template selects the runtime profile and GPU parameters.
	TemplateRef string `json:"templateRef,omitempty"`

	// CacheModel requests that model sources be cached when starting the service
	// if the template itself does not warm the cache.
	// When `warmCache: false` on the template, this setting ensures caching is
	// performed before the service becomes ready.
	// +kubebuilder:default=false
	CacheModel bool `json:"cacheModel,omitempty"`

	// Replicas overrides the number of replicas for this service.
	// Other runtime settings remain governed by the template unless overridden.
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// RuntimeConfigName references the AIM runtime configuration (by name) to use for this service.
	// +kubebuilder:default=default
	RuntimeConfigName string `json:"runtimeConfigName,omitempty"`

	// Resources overrides the container resource requirements for this service.
	// When specified, these values take precedence over the template and image defaults.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Overrides allows overriding specific template parameters for this service.
	// When specified, these values take precedence over the template values.
	// +optional
	Overrides *AIMServiceOverrides `json:"overrides,omitempty"`

	// Env specifies environment variables to use for authentication when downloading models.
	// These variables are used for authentication with model registries (e.g., HuggingFace tokens).
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// ImagePullSecrets references secrets for pulling AIM container images.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// ServiceAccountName specifies the Kubernetes service account to use for the inference workload.
	// This service account is used by the deployed inference pods.
	// If empty, the default service account for the namespace is used.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Routing enables HTTP routing through Gateway API for this service.
	// +optional
	Routing *AIMServiceRouting `json:"routing,omitempty"`
}

// AIMServiceStatus defines the observed state of AIMService.
type AIMServiceStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of template state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResolvedRuntimeConfig captures metadata about the runtime config that was resolved.
	// +optional
	ResolvedRuntimeConfig *AIMResolvedRuntimeConfig `json:"resolvedRuntimeConfig,omitempty"`

	// ResolvedImage captures metadata about the image that was resolved.
	// +optional
	ResolvedImage *AIMResolvedReference `json:"resolvedImage,omitempty"`

	// Status represents the current high‑level status of the service lifecycle.
	// Values: `Pending`, `Starting`, `Running`, `Failed`, `Degraded`.
	// +kubebuilder:default=Pending
	Status AIMServiceStatusEnum `json:"status,omitempty"`

	// Routing surfaces information about the configured HTTP routing, when enabled.
	// +optional
	Routing *AIMServiceRoutingStatus `json:"routing,omitempty"`

	// ResolvedTemplate captures metadata about the template that satisfied the reference.
	ResolvedTemplate *AIMServiceResolvedTemplate `json:"resolvedTemplate,omitempty"`
}

// AIMServiceStatusEnum defines coarse-grained states for a service.
// +kubebuilder:validation:Enum=Pending;Starting;Running;Failed;Degraded
type AIMServiceStatusEnum string

const (
	// AIMServiceStatusPending denotes that the template has been created and discovery has not yet started.
	AIMServiceStatusPending AIMServiceStatusEnum = "Pending"

	// AIMServiceStatusStarting denotes that discovery and/or cache warm is in progress.
	AIMServiceStatusStarting AIMServiceStatusEnum = "Starting"

	// AIMServiceStatusRunning denotes that discovery succeeded and, if requested, caches are warmed.
	AIMServiceStatusRunning AIMServiceStatusEnum = "Running"

	// AIMServiceStatusFailed denotes a terminal failure for discovery or warm operations.
	AIMServiceStatusFailed AIMServiceStatusEnum = "Failed"

	// AIMServiceStatusDegraded denotes a recoverable failure state.
	AIMServiceStatusDegraded AIMServiceStatusEnum = "Degraded"
)

// Condition types for AIMService
const (
	// ConditionResolved is True when the model and template have been validated and a runtime profile has been selected.
	AIMServiceConditionResolved = "Resolved"

	// ConditionCacheReady is True when required caches are present or warmed as requested.
	AIMServiceConditionCacheReady = "CacheReady"

	// ConditionRuntimeReady is True when the underlying KServe runtime and InferenceService are ready.
	AIMServiceConditionRuntimeReady = "RuntimeReady"

	// ConditionRoutingReady is True when exposure and routing through the configured gateway are ready.
	AIMServiceConditionRoutingReady = "RoutingReady"

	// ConditionReady is True when the service is fully ready to serve traffic.
	AIMServiceConditionReady = "Ready"

	// ConditionProgressing is True when the controller is actively reconciling towards readiness.
	AIMServiceConditionProgressing = "Progressing"

	// ConditionFailure is True when a terminal failure has occurred.
	AIMServiceConditionFailure = "Failure"
)

// Condition reasons for AIMService
const (
	// Resolution
	AIMServiceReasonTemplateNotFound           = "TemplateNotFound"
	AIMServiceReasonModelNotFound              = "ModelNotFound"
	AIMServiceReasonModelNotReady              = "ModelNotReady"
	AIMServiceReasonMultipleModelsFound        = "MultipleModelsFound"
	AIMServiceReasonResolved                   = "Resolved"
	AIMServiceReasonValidationFailed           = "ValidationFailed"
	AIMServiceReasonTemplateSelectionAmbiguous = "TemplateSelectionAmbiguous"

	// Cache
	AIMServiceReasonWaitingForCache = "WaitingForCache"
	AIMServiceReasonCacheWarming    = "CacheWarming"
	AIMServiceReasonCacheWarm       = "CacheWarm"
	AIMServiceReasonCacheFailed     = "CacheFailed"

	// Runtime
	AIMServiceReasonCreatingRuntime      = "CreatingRuntime"
	AIMServiceReasonRuntimeReady         = "RuntimeReady"
	AIMServiceReasonRuntimeFailed        = "RuntimeFailed"
	AIMServiceReasonRuntimeConfigMissing = "RuntimeConfigMissing"

	// Routing
	AIMServiceReasonConfiguringRoute    = "ConfiguringRoute"
	AIMServiceReasonRouteReady          = "RouteReady"
	AIMServiceReasonRouteFailed         = "RouteFailed"
	AIMServiceReasonPathTemplateInvalid = "PathTemplateInvalid"
)

// AIMService manages a KServe-based AIM inference service for the selected model and template.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimsvc,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.status.resolvedImage.name`
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.status.resolvedTemplate.name`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AIMService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMServiceSpec   `json:"spec,omitempty"`
	Status AIMServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMServiceList contains a list of AIMService.
type AIMServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMService `json:"items"`
}

// AIMServiceRouting configures optional HTTP routing for the service.
type AIMServiceRouting struct {
	// Enabled toggles HTTP routing management.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// GatewayRef identifies the Gateway parent that should receive the HTTPRoute.
	// When omitted while routing is enabled, reconciliation will report a failure.
	// +optional
	GatewayRef *gatewayapiv1.ParentReference `json:"gatewayRef,omitempty"`

	// Annotations to add to the HTTPRoute resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// PathTemplate overrides the HTTP path template used for routing.
	// The value is rendered against the AIMService object using JSONPath expressions.
	// +optional
	PathTemplate string `json:"pathTemplate,omitempty"`
}

// AIMServiceRoutingStatus captures observed routing details.
type AIMServiceRoutingStatus struct {
	// Path is the HTTP path prefix used when routing is enabled.
	// Example: `/tenant/svc-uuid`.
	// +optional
	Path string `json:"path,omitempty"`
}

// GetStatus returns a pointer to the AIMService status.
func (svc *AIMService) GetStatus() *AIMServiceStatus {
	return &svc.Status
}

func init() {
	SchemeBuilder.Register(&AIMService{}, &AIMServiceList{})
}
