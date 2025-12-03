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
)

// RecommendedDeployment describes a recommended deployment configuration for a model.
type RecommendedDeployment struct {
	// GPUModel is the GPU model name (e.g., MI300X, MI325X)
	// +optional
	GPUModel string `json:"gpuModel,omitempty"`

	// GPUCount is the number of GPUs required
	// +optional
	GPUCount int32 `json:"gpuCount,omitempty"`

	// Precision is the recommended precision (e.g., fp8, fp16, bf16)
	// +optional
	Precision string `json:"precision,omitempty"`

	// Metric is the optimization target (e.g., latency, throughput)
	// +optional
	Metric string `json:"metric,omitempty"`

	// Description provides additional context about this deployment configuration
	// +optional
	Description string `json:"description,omitempty"`
}

// ImageMetadata contains metadata extracted from or provided for a container image.
type ImageMetadata struct {
	// Model contains AMD Silogen model-specific metadata.
	// +optional
	Model *ModelMetadata `json:"model,omitempty"`

	// OCI contains standard OCI image metadata.
	// +optional
	OCI *OCIMetadata `json:"oci,omitempty"`

	// OriginalLabels contains the originally parsed metadata from the image registry.
	// This is stored as JSON to preserve the raw label data.
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	OriginalLabels *apiextensionsv1.JSON `json:"originalLabels,omitempty"`
}

// ModelMetadata contains AMD Silogen model-specific metadata extracted from image labels.
type ModelMetadata struct {
	// CanonicalName is the canonical model identifier (e.g., mistralai/Mixtral-8x22B-Instruct-v0.1).
	// Extracted from: org.amd.silogen.model.canonicalName
	// +optional
	CanonicalName string `json:"canonicalName,omitempty"`

	// Source is the URL where the model can be found.
	// Extracted from: org.amd.silogen.model.source
	// +optional
	Source string `json:"source,omitempty"`

	// Tags are descriptive tags (e.g., ["text-generation", "chat", "instruction"]).
	// Extracted from: org.amd.silogen.model.tags (comma-separated)
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Versions lists available versions.
	// Extracted from: org.amd.silogen.model.versions (comma-separated)
	// +optional
	Versions []string `json:"versions,omitempty"`

	// Variants lists model variants.
	// Extracted from: org.amd.silogen.model.variants (comma-separated)
	// +optional
	Variants []string `json:"variants,omitempty"`

	// HFTokenRequired indicates if a HuggingFace token is required.
	// Extracted from: org.amd.silogen.hfToken.required
	// +optional
	HFTokenRequired bool `json:"hfTokenRequired,omitempty"`

	// Title is the Silogen-specific title for the model.
	// Extracted from: org.amd.silogen.title
	// +optional
	Title string `json:"title,omitempty"`

	// DescriptionFull is the full description.
	// Extracted from: org.amd.silogen.description.full
	// +optional
	DescriptionFull string `json:"descriptionFull,omitempty"`

	// ReleaseNotes contains release notes for this version.
	// Extracted from: org.amd.silogen.release.notes
	// +optional
	ReleaseNotes string `json:"releaseNotes,omitempty"`

	// RecommendedDeployments contains recommended deployment configurations.
	// Extracted from: org.amd.silogen.model.recommendedDeployments (parsed from JSON array)
	// +optional
	RecommendedDeployments []RecommendedDeployment `json:"recommendedDeployments,omitempty"`
}

// OCIMetadata contains standard OCI image metadata extracted from image labels.
type OCIMetadata struct {
	// Title is the human-readable title.
	// Extracted from: org.opencontainers.image.title
	// +optional
	Title string `json:"title,omitempty"`

	// Description is a brief description.
	// Extracted from: org.opencontainers.image.description
	// +optional
	Description string `json:"description,omitempty"`

	// Licenses is the SPDX license identifier(s).
	// Extracted from: org.opencontainers.image.licenses
	// +optional
	Licenses string `json:"licenses,omitempty"`

	// Vendor is the organization that produced the image.
	// Extracted from: org.opencontainers.image.vendor
	// +optional
	Vendor string `json:"vendor,omitempty"`

	// Authors is contact details of the authors.
	// Extracted from: org.opencontainers.image.authors
	// +optional
	Authors string `json:"authors,omitempty"`

	// Source is the URL to the source code repository.
	// Extracted from: org.opencontainers.image.source
	// +optional
	Source string `json:"source,omitempty"`

	// Documentation is the URL to documentation.
	// Extracted from: org.opencontainers.image.documentation
	// +optional
	Documentation string `json:"documentation,omitempty"`

	// Created is the creation timestamp.
	// Extracted from: org.opencontainers.image.created
	// +optional
	Created string `json:"created,omitempty"`

	// Revision is the source control revision.
	// Extracted from: org.opencontainers.image.revision
	// +optional
	Revision string `json:"revision,omitempty"`

	// Version is the image version.
	// Extracted from: org.opencontainers.image.version
	// +optional
	Version string `json:"version,omitempty"`
}
