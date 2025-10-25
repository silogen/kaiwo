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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIMServiceTemplateSpecCommon contains the shared fields for both cluster-scoped
// and namespace-scoped service templates.
type AIMServiceTemplateSpecCommon struct {
	// AIMImageName is the AIM image name. Matches `metadata.name` of an AIMImage. Immutable.
	//
	// Example: `meta/llama-3-8b:1.1+20240915`
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="image name is immutable"
	AIMImageName string `json:"aimImageName"`

	AIMRuntimeParameters `json:",inline"`

	// RuntimeConfigName references the AIM runtime configuration (by name) to use for this template.
	// +kubebuilder:default=default
	RuntimeConfigName string `json:"runtimeConfigName,omitempty"`

	// Resources defines the default container resource requirements applied to services derived from this template.
	// Service-specific values override the template defaults.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AIMTemplateCachingConfig configures model caching behavior for namespace-scoped templates.
type AIMTemplateCachingConfig struct {
	// Enabled controls whether caching is enabled for this template.
	// Defaults to `false`.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Env specifies environment variables to use when downloading the model.
	// These variables are available to the model download process and can be used
	// to configure download behavior, authentication, proxies, etc.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// AIMServiceTemplateSpec defines the desired state of AIMServiceTemplate (namespace-scoped).
//
// A namespaced and versioned template that selects a runtime profile
// for a given AIM model (by canonical name). Templates are intentionally
// narrow: they describe runtime selection knobs for the AIM container and do
// not redefine the full Kubernetes deployment shape.
type AIMServiceTemplateSpec struct {
	AIMServiceTemplateSpecCommon `json:",inline"`

	// Caching configures model caching behavior for this namespace-scoped template.
	// When enabled, models will be cached using the specified environment variables
	// during download.
	// +optional
	Caching *AIMTemplateCachingConfig `json:"caching,omitempty"`

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

// AIMClusterServiceTemplateSpec defines the desired state of AIMClusterServiceTemplate (cluster-scoped).
//
// A cluster-scoped template that selects a runtime profile for a given AIM model.
type AIMClusterServiceTemplateSpec struct {
	AIMServiceTemplateSpecCommon `json:",inline"`
}

// AIMServiceTemplateStatus defines the observed state of AIMServiceTemplate.
type AIMServiceTemplateStatus struct {
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

	// Status represents the current high‑level status of the template lifecycle.
	// Values: `Pending`, `Progressing`, `Ready`, `Failed`, `NotAvailable`.
	// +kubebuilder:default=Pending
	Status AIMTemplateStatusEnum `json:"status,omitempty"`

	// ModelSources list the models that this template requires to run. These are the models that will be
	// cached, if this template is cached.
	ModelSources []AIMModelSource `json:"modelSources,omitempty"`

	// Profile contains the full discovery result profile as a free-form JSON object.
	// This includes metadata, engine args, environment variables, and model details.
	Profile AIMProfile `json:"profile,omitempty"`
}

// AIMProfile contains the cached discovery results for a template.
// This is the processed and validated version of AIMDiscoveryProfile that is stored
// in the template's status after successful discovery.
//
// The profile serves as a cache of runtime configuration, eliminating the need to
// re-run discovery for each service that uses this template. Services and caching
// mechanisms reference this cached profile for deployment parameters and model sources.
//
// See discovery.go for AIMDiscoveryProfile (the raw discovery output) and the
// relationship between these types.
type AIMProfile struct {
	// EngineArgs contains runtime-specific engine configuration as a free-form JSON object.
	// The structure depends on the inference engine being used (e.g., vLLM, TGI).
	// These arguments are passed to the runtime container to configure model loading and inference.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EngineArgs *apiextensionsv1.JSON `json:"engine_args,omitempty"`

	// EnvVars contains environment variables required by the runtime for this profile.
	// These may include engine-specific settings, optimization flags, or hardware configuration.
	// +optional
	EnvVars map[string]string `json:"env_vars,omitempty"`

	// Metadata provides structured information about this deployment profile's characteristics.
	Metadata AIMProfileMetadata `json:"metadata,omitempty"`
}

// AIMProfileMetadata describes the characteristics of a cached deployment profile.
// This is identical to AIMDiscoveryProfileMetadata but exists in the template status namespace.
type AIMProfileMetadata struct {
	// Engine identifies the inference engine used for this profile (e.g., "vllm", "tgi").
	// +optional
	Engine string `json:"engine,omitempty"`

	// GPU specifies the GPU model this profile is optimized for (e.g., "MI300X", "MI325X").
	// +optional
	GPU string `json:"gpu,omitempty"`

	// GPUCount indicates how many GPUs are required per replica for this profile.
	// +optional
	GPUCount int32 `json:"gpu_count,omitempty"`

	// Metric indicates the optimization goal for this profile ("latency" or "throughput").
	// +optional
	Metric AIMMetric `json:"metric,omitempty"`

	// Precision specifies the numeric precision used in this profile (e.g., "fp16", "fp8").
	// +optional
	Precision AIMPrecision `json:"precision,omitempty"`
}

// AIMTemplateStatusEnum defines coarse-grained states for a template.
// +kubebuilder:validation:Enum=Pending;Progressing;NotAvailable;Ready;Degraded;Failed
type AIMTemplateStatusEnum string

const (
	// AIMTemplateStatusPending denotes that the template has been created and discovery has not yet started.
	AIMTemplateStatusPending AIMTemplateStatusEnum = "Pending"
	// AIMTemplateStatusProgressing denotes that discovery and/or cache warm is in progress.
	AIMTemplateStatusProgressing AIMTemplateStatusEnum = "Progressing"
	// AIMTemplateStatusNotAvailable denotes that the template cannot run because the required GPU resources are not present in the cluster.
	AIMTemplateStatusNotAvailable AIMTemplateStatusEnum = "NotAvailable"
	// AIMTemplateStatusReady denotes that discovery succeeded and, if requested, caches are warmed.
	AIMTemplateStatusReady AIMTemplateStatusEnum = "Ready"
	// AIMTemplateStatusDegraded denotes that the template is non-functional for some reason, for example that the cluster doesn't have the resources specified.
	AIMTemplateStatusDegraded AIMTemplateStatusEnum = "Degraded"
	// AIMTemplateStatusFailed denotes a terminal failure for discovery or warm operations.
	AIMTemplateStatusFailed AIMTemplateStatusEnum = "Failed"
)

// Condition types for AIMServiceTemplate
const (
	// AIMTemplateConditionDiscovered is True when runtime profiles have been discovered and sources resolved for the referenced model.
	AIMTemplateConditionDiscovered = "Discovered"
	// AIMTemplateConditionCacheWarm is True when all requested caches have been warmed.
	AIMTemplateConditionCacheWarm = "CacheWarm"
	// AIMTemplateConditionReady is True when the template is ready for use (discovered and, if requested, cache warmed).
	AIMTemplateConditionReady = "Ready"
	// AIMTemplateConditionProgressing is True when the controller is actively processing discovery or cache warm operations.
	AIMTemplateConditionProgressing = "Progressing"
	// AIMTemplateConditionFailure is True when the last attempt has resulted in a terminal failure.
	AIMTemplateConditionFailure = "Failure"
)

// Condition reasons for AIMServiceTemplate
const (
	// Discovery related
	AIMTemplateReasonAwaitingDiscovery  = "AwaitingDiscovery"
	AIMTemplateReasonProfilesDiscovered = "ProfilesDiscovered"
	AIMTemplateReasonDiscoveryFailed    = "DiscoveryFailed"

	// Cache warm related
	AIMTemplateReasonWarmRequested = "WarmRequested"
	AIMTemplateReasonWarming       = "Warming"
	AIMTemplateReasonWarm          = "Warm"
	AIMTemplateReasonWarmFailed    = "WarmFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimst,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.modelId`
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.status.profile.metadata.engine`
// +kubebuilder:printcolumn:name="Metric",type=string,JSONPath=`.status.profile.metadata.metric`
// +kubebuilder:printcolumn:name="Precision",type=string,JSONPath=`.status.profile.metadata.precision`
// +kubebuilder:printcolumn:name="GPUs/replica",type=integer,JSONPath=`.status.profile.metadata.gpu_count`
// +kubebuilder:printcolumn:name="GPU",type=string,JSONPath=`.status.profile.metadata.gpu`

type AIMServiceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMServiceTemplateSpec   `json:"spec,omitempty"`
	Status AIMServiceTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type AIMServiceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMServiceTemplate `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimclst,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.status.profile.metadata.engine`
// +kubebuilder:printcolumn:name="Metric",type=string,JSONPath=`.status.profile.metadata.metric`
// +kubebuilder:printcolumn:name="Precision",type=string,JSONPath=`.status.profile.metadata.precision`
// +kubebuilder:printcolumn:name="GPUs/replica",type=integer,JSONPath=`.status.profile.metadata.gpu_count`
// +kubebuilder:printcolumn:name="GPU",type=string,JSONPath=`.status.profile.metadata.gpu`
type AIMClusterServiceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMClusterServiceTemplateSpec `json:"spec,omitempty"`
	Status AIMServiceTemplateStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type AIMClusterServiceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterServiceTemplate `json:"items"`
}

// GetModelID returns the model ID from the template spec
func (t *AIMServiceTemplate) GetModelName() string {
	return t.Spec.AIMImageName
}

// GetStatus returns a pointer to the template status
func (t *AIMServiceTemplate) GetStatus() *AIMServiceTemplateStatus {
	return &t.Status
}

// GetModelID returns the model ID from the cluster template spec
func (t *AIMClusterServiceTemplate) GetModelName() string {
	return t.Spec.AIMImageName
}

// GetStatus returns a pointer to the cluster template status
func (t *AIMClusterServiceTemplate) GetStatus() *AIMServiceTemplateStatus {
	return &t.Status
}

func init() {
	SchemeBuilder.Register(&AIMServiceTemplate{}, &AIMServiceTemplateList{}, &AIMClusterServiceTemplate{}, &AIMClusterServiceTemplateList{})
}
