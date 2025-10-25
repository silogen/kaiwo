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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// This file defines types for the discovery process that inspects AIM container images
// to determine runtime requirements, model sources, and deployment profiles.
//
// Discovery Process Overview:
// When a template or image with discovery enabled is created, the operator launches a
// discovery job that runs the container in dry-run mode. This job:
// 1. Inspects the container's runtime configuration
// 2. Determines required model artifacts and their download sources
// 3. Extracts deployment profiles optimized for different GPU types and metrics
// 4. Returns structured metadata that is cached in the template's status
//
// The types in this file capture both the intermediate discovery output (AIMDiscoveryProfile)
// and the final cached profile (AIMProfile in aimservicetemplate_types.go).

// ProfileDiscoveryStatusEnum tracks the lifecycle of discovery for a specific deployment profile.
// A single image may have multiple profiles (e.g., latency-optimized for MI300X, throughput-optimized for MI325X).
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;NotAvailable
type ProfileDiscoveryStatusEnum string

const (
	// ProfileDiscoveryStatusPending indicates discovery has not started for this profile.
	ProfileDiscoveryStatusPending ProfileDiscoveryStatusEnum = "Pending"

	// ProfileDiscoveryStatusRunning indicates discovery is currently in progress for this profile.
	ProfileDiscoveryStatusRunning ProfileDiscoveryStatusEnum = "Running"

	// ProfileDiscoveryStatusCompleted indicates discovery completed successfully and profile data is available.
	ProfileDiscoveryStatusCompleted ProfileDiscoveryStatusEnum = "Completed"

	// ProfileDiscoveryStatusFailed indicates discovery failed for this profile (e.g., container error, invalid metadata).
	ProfileDiscoveryStatusFailed ProfileDiscoveryStatusEnum = "Failed"

	// ProfileDiscoveryStatusNotAvailable indicates the required GPU type for this profile doesn't exist in the cluster.
	// The profile is valid but cannot be used until appropriate hardware is available.
	ProfileDiscoveryStatusNotAvailable ProfileDiscoveryStatusEnum = "NotAvailable"
)

// AIMDiscoveryProfile represents a deployment configuration discovered during image inspection.
// This is the raw output from a discovery job before it's processed and cached in a template.
//
// Each profile represents a specific combination of optimization target (metric), precision,
// GPU type, and GPU count. A single image may produce multiple profiles for different hardware
// configurations.
//
// Relationship to AIMProfile:
// - AIMDiscoveryProfile: Raw discovery job output, used during initial inspection
// - AIMProfile (in aimservicetemplate_types.go): Processed and cached in template status
//
// Both types have identical structure but serve different lifecycle stages.
type AIMDiscoveryProfile struct {
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
	Metadata AIMDiscoveryProfileMetadata `json:"metadata,omitempty"`
}

// AIMDiscoveryProfileMetadata describes the characteristics of a discovered deployment profile.
type AIMDiscoveryProfileMetadata struct {
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

// AIMModelSource describes a model artifact that must be downloaded for inference.
// Discovery extracts these from the container's configuration to enable caching and validation.
type AIMModelSource struct {
	// Name is a human-readable identifier for this model artifact.
	// May be empty if the source represents the primary model.
	// +optional
	Name string `json:"name,omitempty"`

	// SourceURI is the location from which the model should be downloaded.
	// Supported schemes:
	// - hf://org/model - Hugging Face Hub model
	// - s3://bucket/key - S3-compatible storage
	// +kubebuilder:validation:Pattern=`^(hf|s3)://[^ \t\r\n]+$`
	SourceURI string `json:"sourceUri"`

	// Size is the expected storage space required for this model artifact.
	// Used for PVC sizing and capacity planning during cache creation.
	Size resource.Quantity `json:"size"`
}
