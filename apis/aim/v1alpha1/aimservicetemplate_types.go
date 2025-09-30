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

// AIMServiceTemplateSpec defines the desired state of AIMServiceTemplate.
//
// A namespaced and versioned template that selects a runtime profile
// for a given AIM model (by canonical name). Templates are intentionally
// narrow: they describe runtime selection knobs for the AIM container and do
// not redefine the full Kubernetes deployment shape.
type AIMServiceTemplateSpec struct {
	// Model is the canonical model name (exact string match), including version/revision.
	// Matches `spec.name` of an AIMClusterModel. Immutable.
	//
	// Example: `meta/llama-3-8b:1.1+20240915`
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="model is immutable"
	Model string `json:"model"`

	// Metric selects the optimization goal. Immutable.
	//
	// - `latency`: prioritize low end‑to‑end latency
	// - `throughput`: prioritize sustained requests/second
	//
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="metric is immutable"
	// +kubebuilder:validation:Enum=latency;throughput
	Metric AIMMetric `json:"metric"`

	// Precision selects the numeric precision used by the runtime. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="precision is immutable"
	// +kubebuilder:default=auto
	// +kubebuilder:validation:Enum=auto;fp4;fp8;fp16;fp32;bf16;int4;int8
	Precision AIMPrecision `json:"precision"`

	// GpusPerReplica is the total number of GPUs for a single model replica. Immutable.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpusPerReplica is immutable"
	GpusPerReplica int32 `json:"gpusPerReplica"`

	// GpuModel is the physical GPU card model targeted by this template. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpuModel is immutable"
	GpuModel string `json:"gpuModel"`

	// TensorParallelism is the tensor parallel degree expected by the runtime. Immutable.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="tensorParallelism is immutable"
	TensorParallelism int32 `json:"tensorParallelism"`

	// WarmCache requests immediate model cache warming in this namespace after profile discovery.
	// Defaults to `false`.
	//
	// When left `false`, services can still request caching via `AIMService.spec.cacheModel: true`.
	//
	// +kubebuilder:default=false
	WarmCache bool `json:"warmCache,omitempty"`
}

// AIMServiceTemplateStatus defines the observed state of AIMServiceTemplate.
type AIMServiceTemplateStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of template state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status represents the current high‑level status of the template lifecycle.
	// Values: `Pending`, `Progressing`, `Available`, `Failed`.
	// +kubebuilder:default=Pending
	Status AIMTemplateStatusEnum `json:"status,omitempty"`

	// ModelSources list the models that this template requires to run. These are the models that will be
	// cached, if this template is cached.
	ModelSources []string `json:"modelSources,omitempty"`
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
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="Metric",type=string,JSONPath=`.spec.useCase`
// +kubebuilder:printcolumn:name="Precision",type=string,JSONPath=`.spec.precision`
// +kubebuilder:printcolumn:name="GPUs/replica",type=integer,JSONPath=`.spec.gpusPerReplica`
// +kubebuilder:printcolumn:name="GPU",type=string,JSONPath=`.spec.gpuModel`
// +kubebuilder:printcolumn:name="TP",type=integer,JSONPath=`.spec.tensorParallelism`
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

func init() {
	SchemeBuilder.Register(&AIMServiceTemplate{}, &AIMServiceTemplateList{})
}
