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
	"k8s.io/apimachinery/pkg/runtime"
)

// AIMUseCase enumerates the targeted service characteristic
// +kubebuilder:validation:Enum=latency;throughput
type AIMUseCase string

const (
	AIMUseCaseLatency    AIMUseCase = "latency"
	AIMUseCaseThroughput AIMUseCase = "throughput"
)

// AIMPrecision enumerates supported numeric precisions
// +kubebuilder:validation:Enum=bf16;fp16;fp8;int8
type AIMPrecision string

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

	// UseCase selects the optimization goal. Immutable.
	//
	// - `latency`: prioritize low end‑to‑end latency
	// - `throughput`: prioritize sustained requests/second
	//
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="useCase is immutable"
	UseCase AIMUseCase `json:"useCase"`

	// Precision selects the numeric precision used by the runtime. Immutable.
	//
	// - `bf16`
	// - `fp16`
	// - `fp8`
	// - `int8`
	//
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="precision is immutable"
	Precision AIMPrecision `json:"precision"`

	// GpusPerReplica is the total number of GPUs for a single model replica. Immutable.
	//
	// Example: `1`, `2`
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpusPerReplica is immutable"
	GpusPerReplica int32 `json:"gpusPerReplica"`

	// GpuModel is the physical GPU card model targeted by this template (single value). Immutable.
	//
	// Example: `MI300X`, `MI325X`
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpuModel is immutable"
	GpuModel string `json:"gpuModel"`

	// TensorParallelism is the tensor parallel degree expected by the runtime. Immutable.
	//
	// Example: `1` (no TP), `2`
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="tensorParallelism is immutable"
	TensorParallelism int32 `json:"tensorParallelism"`

	// WarmCache requests immediate model cache warming in this namespace after profile discovery.
	// Defaults to `false`. Immutable.
	//
	// When left `false`, services can still request caching via `AIMService.spec.cacheModel: true`.
	//
	// +kubebuilder:default=false
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="warmCache is immutable"
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
// +kubebuilder:printcolumn:name="UseCase",type=string,JSONPath=`.spec.useCase`
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

// DeepCopy implementations (manual to avoid requiring code generation in this stub phase)

func (in *AIMServiceTemplate) DeepCopyInto(out *AIMServiceTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
	out.Status.ObservedGeneration = in.Status.ObservedGeneration
}

func (in *AIMServiceTemplate) DeepCopy() *AIMServiceTemplate {
	if in == nil {
		return nil
	}
	out := new(AIMServiceTemplate)
	in.DeepCopyInto(out)
	return out
}

func (in *AIMServiceTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AIMServiceTemplateList) DeepCopyInto(out *AIMServiceTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AIMServiceTemplate, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AIMServiceTemplateList) DeepCopy() *AIMServiceTemplateList {
	if in == nil {
		return nil
	}
	out := new(AIMServiceTemplateList)
	in.DeepCopyInto(out)
	return out
}

func (in *AIMServiceTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&AIMServiceTemplate{}, &AIMServiceTemplateList{})
}
