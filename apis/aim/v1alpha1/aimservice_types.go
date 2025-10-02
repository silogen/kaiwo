// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	// AIMModelID is the canonical model name (including version/revision) to deploy.
	// Expected to match the `spec.name` of an AIMImage. Example:
	// `meta/llama-3-8b:1.1+20240915`.
	// +kubebuilder:validation:MinLength=1
	AIMModelID string `json:"aimModelId"`

	// TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to use.
	// The template selects the runtime profile and GPU parameters.
	TemplateRef string `json:"templateRef"`

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

	// ConfigRef selects the cluster-scoped AIMClusterConfig (by name) to use for this service.
	// +kubebuilder:default=default
	ConfigRef string `json:"configRef,omitempty"`

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
}

// AIMServiceStatus defines the observed state of AIMService.
type AIMServiceStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of template state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status represents the current high‑level status of the service lifecycle.
	// Values: `Pending`, `Starting`, `Running`, `Failed`, `Degraded`.
	// +kubebuilder:default=Pending
	Status AIMServiceStatusEnum `json:"status,omitempty"`
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
	AIMServiceReasonTemplateNotFound = "TemplateNotFound"
	AIMServiceReasonModelNotFound    = "ModelNotFound"
	AIMServiceReasonResolved         = "Resolved"
	AIMServiceReasonValidationFailed = "ValidationFailed"

	// Cache
	AIMServiceReasonWaitingForCache = "WaitingForCache"
	AIMServiceReasonCacheWarming    = "CacheWarming"
	AIMServiceReasonCacheWarm       = "CacheWarm"
	AIMServiceReasonCacheFailed     = "CacheFailed"

	// Runtime
	AIMServiceReasonCreatingRuntime = "CreatingRuntime"
	AIMServiceReasonRuntimeReady    = "RuntimeReady"
	AIMServiceReasonRuntimeFailed   = "RuntimeFailed"

	// Routing
	AIMServiceReasonConfiguringRoute = "ConfiguringRoute"
	AIMServiceReasonRouteReady       = "RouteReady"
	AIMServiceReasonRouteFailed      = "RouteFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimsvc,categories=aim;all
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMService manages a KServe-based AIM inference service for the selected model and template.
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

func init() {
	SchemeBuilder.Register(&AIMService{}, &AIMServiceList{})
}
