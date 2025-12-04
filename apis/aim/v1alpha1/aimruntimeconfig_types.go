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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// AIMModelConfig controls model creation and discovery behavior.
type AIMModelConfig struct {
	// AutoDiscovery controls whether models run discovery by default.
	// When true, models run discovery jobs to extract metadata and auto-create templates.
	// When false, discovery is skipped. Discovery failures are non-fatal and reported via conditions.
	// +kubebuilder:default=true
	// +optional
	AutoDiscovery *bool `json:"autoDiscovery,omitempty"`
}

// AIMRuntimeConfigCommon captures configuration fields shared across cluster and namespace scopes.
// These settings apply to both AIMRuntimeConfig (namespace-scoped) and AIMClusterRuntimeConfig (cluster-scoped).
type AIMRuntimeConfigCommon struct {
	// DefaultStorageClassName specifies the storage class to use for model caches and PVCs
	// when the consuming resource (AIMModelCache, AIMTemplateCache, AIMServiceTemplate) does not
	// specify a storage class. If this field is empty, the cluster's default storage class is used.
	// +optional
	DefaultStorageClassName string `json:"defaultStorageClassName,omitempty"`

	// Model controls model creation and discovery defaults.
	// +optional
	Model *AIMModelConfig `json:"model,omitempty"`

	// Routing controls HTTP routing defaults applied to AIM resources.
	// When set, these defaults are used for AIMService resources that enable routing
	// but do not specify their own routing configuration.
	// +optional
	Routing *AIMRuntimeRoutingConfig `json:"routing,omitempty"`

	// PVCHeadroomPercent specifies the percentage of extra space to add to PVCs
	// for model storage. This accounts for filesystem overhead and temporary files
	// during model loading. The value represents a percentage (e.g., 10 means 10% extra space).
	// If not specified, defaults to 10%.
	// +kubebuilder:validation:Minimum=0
	// +optional
	PVCHeadroomPercent *int32 `json:"pvcHeadroomPercent,omitempty"`
}

// AIMClusterRuntimeConfigSpec defines cluster-wide defaults for AIM resources.
type AIMClusterRuntimeConfigSpec struct {
	AIMRuntimeConfigCommon `json:",inline"`
}

// AIMRuntimeConfigSpec defines namespace-scoped overrides for AIM resources.
type AIMRuntimeConfigSpec struct {
	AIMRuntimeConfigCommon `json:",inline"`
}

// AIMRuntimeRoutingConfig configures HTTP routing defaults for inference services.
// These settings control how Gateway API HTTPRoutes are created and configured.
type AIMRuntimeRoutingConfig struct {
	// Enabled controls whether HTTP routing is managed for inference services using this config.
	// When true, the operator creates HTTPRoute resources for services that reference this config.
	// When false or unset, routing must be explicitly enabled on each service.
	// This provides a namespace or cluster-wide default that individual services can override.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// GatewayRef specifies the Gateway API Gateway resource that should receive HTTPRoutes.
	// This identifies the parent gateway for routing traffic to inference services.
	// The gateway can be in any namespace (cross-namespace references are supported).
	// If routing is enabled but GatewayRef is not specified, service reconciliation will fail
	// with a validation error.
	// +optional
	GatewayRef *gatewayapiv1.ParentReference `json:"gatewayRef,omitempty"`

	// PathTemplate defines the HTTP path template for routes, evaluated using JSONPath expressions.
	// The template is rendered against the AIMService object to generate unique paths.
	//
	// Example templates:
	// - `/{.metadata.namespace}/{.metadata.name}` - namespace and service name
	// - `/{.metadata.namespace}/{.metadata.labels['team']}/inference` - with label
	// - `/models/{.spec.aimModelName}` - based on model name
	//
	// The template must:
	// - Use valid JSONPath expressions wrapped in {...}
	// - Reference fields that exist on the service
	// - Produce a path ≤ 200 characters after rendering
	// - Result in valid URL path segments (lowercase, RFC 1123 compliant)
	//
	// If evaluation fails, the service enters Degraded state with PathTemplateInvalid reason.
	// Individual services can override this template via spec.routing.pathTemplate.
	// +optional
	PathTemplate string `json:"pathTemplate,omitempty"`

	// Annotations defines additional annotations to add to the HTTPRoute resource.
	// These annotations can be used for various purposes such as configuring ingress
	// behavior, adding metadata, or triggering external integrations.
	// Individual services can override these via spec.routing.annotations.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// RequestTimeout defines the HTTP request timeout for routes.
	// This sets the maximum duration for a request to complete before timing out.
	// The timeout applies to the entire request/response cycle.
	// If not specified, no timeout is set on the route.
	// Individual services can override this value via spec.routing.requestTimeout.
	// +optional
	RequestTimeout *metav1.Duration `json:"requestTimeout,omitempty"`
}

// AIMRuntimeConfigStatus records the resolved config reference surfaced to consumers.
type AIMRuntimeConfigStatus struct {
	// ObservedGeneration is the last reconciled generation.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions communicate reconciliation progress.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AIMResolvedRuntimeConfig captures metadata about the runtime config that was resolved.
// This follows the same pattern as AIMServiceResolvedTemplate for consistency.
type AIMResolvedRuntimeConfig struct {
	AIMResolvedReference `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=aimcrcfg,categories=aim;all
// +kubebuilder:printcolumn:name="CacheBaseImages",type=boolean,JSONPath=`.spec.cacheBaseImages`
// +kubebuilder:printcolumn:name="DefaultStorageClass",type=string,JSONPath=`.spec.defaultStorageClassName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMClusterRuntimeConfig defines cluster-scoped runtime defaults for AIM resources.
type AIMClusterRuntimeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMClusterRuntimeConfigSpec `json:"spec,omitempty"`
	Status AIMRuntimeConfigStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMClusterRuntimeConfigList contains a list of AIMClusterRuntimeConfig.
type AIMClusterRuntimeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterRuntimeConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=aimrcfg,categories=aim;all
// +kubebuilder:printcolumn:name="ServiceAccount",type=string,JSONPath=`.spec.serviceAccountName`
// +kubebuilder:printcolumn:name="CacheBaseImages",type=boolean,JSONPath=`.spec.cacheBaseImages`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMRuntimeConfig defines namespace-scoped runtime overrides for AIM resources.
type AIMRuntimeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMRuntimeConfigSpec   `json:"spec,omitempty"`
	Status AIMRuntimeConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMRuntimeConfigList contains a list of AIMRuntimeConfig.
type AIMRuntimeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMRuntimeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterRuntimeConfig{}, &AIMClusterRuntimeConfigList{}, &AIMRuntimeConfig{}, &AIMRuntimeConfigList{})
}
