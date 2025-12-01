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

// AIMServiceKVCache specifies KV cache configuration for the service.
// The controller will use an existing AIMKVCache if found, otherwise it will create one.
type AIMServiceKVCache struct {
	// Name specifies the name of the AIMKVCache resource to use.
	// If an AIMKVCache with this name exists, it will be used.
	// If it doesn't exist, a new AIMKVCache will be created with this name.
	// If not specified, defaults to "kvcache-{service-name}".
	// +optional
	Name string `json:"name,omitempty"`

	// Type specifies the type of KV cache backend.
	// Only used when creating a new AIMKVCache (ignored if referencing existing).
	// +kubebuilder:validation:Enum=redis
	// +kubebuilder:default=redis
	Type string `json:"type,omitempty"`

	// Image specifies the container image to use for the KV cache service.
	// Only used when creating a new AIMKVCache (ignored if referencing existing).
	// If not specified, defaults to appropriate images based on Type.
	// +optional
	Image *string `json:"image,omitempty"`

	// Env specifies environment variables to set in the KV cache container.
	// Only used when creating a new AIMKVCache (ignored if referencing existing).
	// If not specified (nil), no additional environment variables are set.
	// If explicitly set to an empty array, no environment variables are added.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Storage defines the persistent storage configuration for the KV cache.
	// Only used when creating a new AIMKVCache (ignored if referencing existing).
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// Resources defines the resource requirements for the KV cache container.
	// Only used when creating a new AIMKVCache (ignored if referencing existing).
	// If not specified, defaults to 1 CPU and 1Gi memory for both requests and limits.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// LMCacheConfig specifies the custom LMCache configuration YAML content.
	// When specified, this exact configuration is used for the lmcache_config.yaml file.
	// When empty, a default configuration is generated with standard LMCache settings.
	// Note: The remote_url field in custom configs will have the {SERVICE_URL} placeholder
	// replaced with the actual KV cache service URL.
	// +optional
	LMCacheConfig string `json:"lmCacheConfig,omitempty"`
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

	// MinReplicas specifies the minimum number of replicas for autoscaling.
	// Defaults to 1 but can be set to 0 to enable scale-to-zero.
	// When specified with MaxReplicas, enables autoscaling for the service.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas specifies the maximum number of replicas for autoscaling.
	// Required when MinReplicas is set or when AutoScaling configuration is provided.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// AutoScaling configures advanced autoscaling behavior using HPA or KEDA.
	// Supports custom metrics from various backends (Prometheus, OpenTelemetry, etc.)
	// When specified, MinReplicas and MaxReplicas should also be set.
	// +optional
	AutoScaling *AIMServiceAutoScaling `json:"autoScaling,omitempty"`

	// RuntimeConfigName references the AIM runtime configuration (by name) to use for this service.
	// +kubebuilder:default=default
	RuntimeConfigName string `json:"runtimeConfigName,omitempty"`

	// Resources overrides the container resource requirements for this service.
	// When specified, these values take precedence over the template and image defaults.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// KVCache specifies KV cache configuration for the service.
	// When specified, enables LMCache with the configured KV cache backend.
	// +optional
	KVCache *AIMServiceKVCache `json:"kvCache,omitempty"`

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

	// RuntimeOverrides allows overriding runtime configuration settings for this service.
	// When specified, these values take precedence over both namespace and cluster-level runtime configs.
	// This provides fine-grained control over storage, model behavior, routing, and other runtime settings.
	//
	// The precedence order is:
	// 1. AIMService.Spec.RuntimeOverrides (highest priority)
	// 2. AIMRuntimeConfig (namespace-level)
	// 3. AIMClusterRuntimeConfig (cluster-level)
	// +optional
	RuntimeOverrides *AIMRuntimeConfigCommon `json:"runtimeOverrides,omitempty"`
}

// AIMServiceAutoScaling mirrors KServe's AutoScalingSpec for advanced autoscaling configuration.
// Supports custom metrics from various backends including Prometheus, OpenTelemetry, and KEDA.
type AIMServiceAutoScaling struct {
	// Metrics is a list of metrics spec to be used for autoscaling.
	// Each metric defines a source (Resource, External, or PodMetric) and target values.
	// +optional
	Metrics []AIMServiceMetricsSpec `json:"metrics,omitempty"`
}

// AIMServiceMetricsSpec defines a single metric for autoscaling.
// Specifies the metric source type and configuration.
type AIMServiceMetricsSpec struct {
	// Type is the type of metric source.
	// Valid values: "Resource" (CPU/memory), "External" (external metrics), "PodMetric" (per-pod custom metrics)
	// +kubebuilder:validation:Enum=Resource;External;PodMetric
	Type string `json:"type"`

	// PodMetric refers to a metric describing each pod in the current scale target.
	// Used when Type is "PodMetric". Supports backends like OpenTelemetry for custom metrics.
	// +optional
	PodMetric *AIMServicePodMetricSource `json:"podmetric,omitempty"`
}

// AIMServicePodMetricSource defines pod-level metrics configuration.
// Specifies the metric identification and target values for pod-based autoscaling.
type AIMServicePodMetricSource struct {
	// Metric contains the metric identification and backend configuration.
	// Defines which metrics to collect and how to query them.
	// +optional
	Metric *AIMServicePodMetric `json:"metric,omitempty"`

	// Target specifies the target value for the metric.
	// The autoscaler will scale to maintain this target value.
	// +optional
	Target *AIMServiceMetricTarget `json:"target,omitempty"`
}

// AIMServicePodMetric identifies the pod metric and its backend.
// Supports multiple metrics backends including OpenTelemetry, Prometheus, etc.
type AIMServicePodMetric struct {
	// Backend defines the metrics backend to use.
	// Example: "opentelemetry" for OpenTelemetry-based metrics.
	// +optional
	Backend string `json:"backend,omitempty"`

	// ServerAddress specifies the address of the metrics backend server.
	// Example: "http://otel-collector:9090" for OpenTelemetry Collector.
	// If not specified, the default server address for the backend will be used.
	// +optional
	ServerAddress string `json:"serverAddress,omitempty"`

	// MetricNames is the list of metric names to collect from the backend.
	// Example: ["vllm:num_requests_running"] for vLLM request metrics.
	// +optional
	MetricNames []string `json:"metricNames,omitempty"`

	// Query specifies the query to run to retrieve metrics from the backend.
	// The query syntax depends on the backend being used.
	// Example: "vllm:num_requests_running" for OpenTelemetry.
	// +optional
	Query string `json:"query,omitempty"`

	// OperationOverTime specifies the operation to aggregate metrics over time.
	// Valid values: "last_one", "avg", "max", "min", "rate", "count"
	// Default: "last_one"
	// +optional
	OperationOverTime string `json:"operationOverTime,omitempty"`
}

// AIMServiceMetricTarget defines the target value for a metric.
// Specifies how the metric value should be interpreted and what target to maintain.
type AIMServiceMetricTarget struct {
	// Type specifies how to interpret the metric value.
	// "Value": absolute value target (use Value field)
	// "AverageValue": average value across all pods (use AverageValue field)
	// "Utilization": percentage utilization for resource metrics (use AverageUtilization field)
	// +kubebuilder:validation:Enum=Value;AverageValue;Utilization
	Type string `json:"type"`

	// Value is the target value of the metric (as a quantity).
	// Used when Type is "Value".
	// Example: "1" for 1 request, "100m" for 100 millicores
	// +optional
	Value string `json:"value,omitempty"`

	// AverageValue is the target value of the average of the metric across all relevant pods (as a quantity).
	// Used when Type is "AverageValue".
	// Example: "100m" for 100 millicores per pod
	// +optional
	AverageValue string `json:"averageValue,omitempty"`

	// AverageUtilization is the target value of the average of the resource metric across all relevant pods,
	// represented as a percentage of the requested value of the resource for the pods.
	// Used when Type is "Utilization". Only valid for Resource metric source type.
	// Example: 80 for 80% utilization
	// +optional
	AverageUtilization *int32 `json:"averageUtilization,omitempty"`
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

	// ResolvedTemplateCache captures metadata about the template cache being used, if any.
	// +optional
	ResolvedTemplateCache *AIMResolvedReference `json:"resolvedTemplateCache,omitempty"`

	// ResolvedKVCache captures metadata about the KV cache being used, if any.
	// +optional
	ResolvedKVCache *AIMResolvedReference `json:"resolvedKVCache,omitempty"`
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

	// ConditionKVCacheReady is True when required KVCache is ready.
	AIMServiceConditionKVCacheReady = "KVCacheReady"

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

	// KVCache
	AIMServiceReasonWaitingForKVCache   = "WaitingForKVCache"
	AIMServiceReasonKVCacheProgressing  = "KVCacheProgressing"
	AIMServiceReasonKVCacheReady        = "KVCacheReady"
	AIMServiceReasonKVCacheFailed       = "KVCacheFailed"
	AIMServiceReasonKVCacheNotRequested = "KVCacheNotRequested"

	// Runtime
	AIMServiceReasonCreatingRuntime      = "CreatingRuntime"
	AIMServiceReasonRuntimeReady         = "RuntimeReady"
	AIMServiceReasonRuntimeFailed        = "RuntimeFailed"
	AIMServiceReasonRuntimeConfigMissing = "RuntimeConfigMissing"

	// Image pull related
	AIMServiceReasonImagePullAuthFailure = "ImagePullAuthFailure"
	AIMServiceReasonImageNotFound        = "ImageNotFound"
	AIMServiceReasonImagePullBackOff     = "ImagePullBackOff"

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
