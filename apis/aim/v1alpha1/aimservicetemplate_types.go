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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIMMetric enumerates the targeted service characteristic
// +kubebuilder:validation:Enum=latency;throughput
type AIMMetric string

const (
	AIMMetricLatency    AIMMetric = "latency"
	AIMMetricThroughput AIMMetric = "throughput"
)

// AIMPrecision enumerates supported numeric precisions
// +kubebuilder:validation:Enum=bf16;fp16;fp8;int8
type AIMPrecision string

const (
	AIMPrecisionAuto AIMPrecision = "auto"
	AIMPrecisionFP4  AIMPrecision = "fp4"
	AIMPrecisionFP8  AIMPrecision = "fp8"
	AIMPrecisionFP16 AIMPrecision = "fp16"
	AIMPrecisionFP32 AIMPrecision = "fp32"
	AIMPrecisionBF16 AIMPrecision = "bf16"
	AIMPrecisionInt4 AIMPrecision = "int4"
	AIMPrecisionInt8 AIMPrecision = "int8"
)

// AIMRuntimeParameters contains the runtime configuration parameters shared
// across templates and services. Fields use pointers to allow optional usage
// in different contexts (required in templates, optional in service overrides).
type AIMRuntimeParameters struct {
	// Metric selects the optimization goal.
	//
	// - `latency`: prioritize low end‑to‑end latency
	// - `throughput`: prioritize sustained requests/second
	//
	// +optional
	// +kubebuilder:validation:Enum=latency;throughput
	Metric *AIMMetric `json:"metric,omitempty"`

	// Precision selects the numeric precision used by the runtime.
	// +optional
	// +kubebuilder:validation:Enum=auto;fp4;fp8;fp16;fp32;bf16;int4;int8
	Precision *AIMPrecision `json:"precision,omitempty"`

	// AimGpuSelector contains the strategy to choose the resources to give each replica
	// +optional
	GpuSelector *AimGpuSelector `json:"gpuSelector,omitempty"`
}

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

type AimGpuSelector struct {
	// Count is the number of the GPU resources requested per replica
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`

	// Model is the model name of the GPU that is supported by this template
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// TODO re-enable partitioning once it is supported

	//// ComputePartitioning mode.
	//// +kubebuilder:default="spx"
	//// +kubebuilder:validation:Enum=spx;cpx
	//ComputePartitioning string `json:"computePartitioning,omitempty"`
	//
	//// ComputePartitioning mode
	//// +kubebuilder:default:"nps1"
	//// +kubebuilder:validation:Enum=nps1;nps4
	//MemoryPartitioning string `json:"memoryPartitioning,omitempty"`
}

// AIMServiceTemplateStatus defines the observed state of AIMServiceTemplate.
type AIMServiceTemplateStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of template state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// EffectiveRuntimeConfig surfaces the merged runtime configuration applied to this template.
	EffectiveRuntimeConfig *AIMEffectiveRuntimeConfig `json:"effectiveRuntimeConfig,omitempty"`

	// Status represents the current high‑level status of the template lifecycle.
	// Values: `Pending`, `Progressing`, `Available`, `Failed`.
	// +kubebuilder:default=Pending
	Status AIMTemplateStatusEnum `json:"status,omitempty"`

	// ModelSources list the models that this template requires to run. These are the models that will be
	// cached, if this template is cached.
	ModelSources []AIMModelSource `json:"modelSources,omitempty"`

	// Profile contains the full discovery result profile as a free-form JSON object.
	// This includes metadata, engine args, environment variables, and model details.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Profile *apiextensionsv1.JSON `json:"profile,omitempty"`
}

type AIMModelSource struct {
	// Name is the name of the model
	// +optional
	Name string `json:"name,omitempty"`

	// SourceURI is the source where the model should be downloaded from
	SourceURI string `json:"sourceUri"`

	// Size is the amount of storage that the source expects
	Size resource.Quantity `json:"size"`
}

// AIMTemplateStatusEnum defines coarse-grained states for a template.
// +kubebuilder:validation:Enum=Pending;Progressing;Available;Failed
type AIMTemplateStatusEnum string

const (
	// AIMTemplateStatusPending denotes that the template has been created and discovery has not yet started.
	AIMTemplateStatusPending AIMTemplateStatusEnum = "Pending"
	// AIMTemplateStatusProgressing denotes that discovery and/or cache warm is in progress.
	AIMTemplateStatusProgressing AIMTemplateStatusEnum = "Progressing"
	// AIMTemplateStatusAvailable denotes that discovery succeeded and, if requested, caches are warmed.
	AIMTemplateStatusAvailable AIMTemplateStatusEnum = "Available"
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
// +kubebuilder:printcolumn:name="Metric",type=string,JSONPath=`.spec.metric`
// +kubebuilder:printcolumn:name="Precision",type=string,JSONPath=`.spec.precision`
// +kubebuilder:printcolumn:name="GPUs/replica",type=integer,JSONPath=`.spec.gpuSelector.count`
// +kubebuilder:printcolumn:name="GPU",type=string,JSONPath=`.spec.gpuSelector.model`
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
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.modelId`
// +kubebuilder:printcolumn:name="Metric",type=string,JSONPath=`.spec.metric`
// +kubebuilder:printcolumn:name="Precision",type=string,JSONPath=`.spec.precision`
// +kubebuilder:printcolumn:name="GPUs/replica",type=integer,JSONPath=`.spec.gpuSelector.count`
// +kubebuilder:printcolumn:name="GPU",type=string,JSONPath=`.spec.gpuSelector.model`
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

// GetStatus returns a pointer to the cluster template status
func (t *AIMClusterServiceTemplate) GetStatus() *AIMServiceTemplateStatus {
	return &t.Status
}

func init() {
	SchemeBuilder.Register(&AIMServiceTemplate{}, &AIMServiceTemplateList{}, &AIMClusterServiceTemplate{}, &AIMClusterServiceTemplateList{})
}
