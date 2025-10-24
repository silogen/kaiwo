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
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="metric is immutable"
	Metric *AIMMetric `json:"metric,omitempty"`

	// Precision selects the numeric precision used by the runtime.
	// +optional
	// +kubebuilder:validation:Enum=auto;fp4;fp8;fp16;fp32;bf16;int4;int8
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="precision is immutable"
	Precision *AIMPrecision `json:"precision,omitempty"`

	// AimGpuSelector contains the strategy to choose the resources to give each replica
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="gpuSelector is immutable"
	GpuSelector *AimGpuSelector `json:"gpuSelector,omitempty"`
}

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

type AimGpuSelector struct {
	// Count is the number of the GPU resources requested per replica
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`

	// Model is the model name of the GPU that is supported by this template
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// ResourceName is the Kubernetes resource name for GPU resources
	// +optional
	// +kubebuilder:default="amd.com/gpu"
	ResourceName string `json:"resourceName,omitempty"`

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
