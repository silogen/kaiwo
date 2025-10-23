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

const (
	// AIMImageConditionRuntimeResolved captures whether runtime config resolution succeeded.
	AIMImageConditionRuntimeResolved = "RuntimeResolved"

	// AIMImageReasonRuntimeResolved indicates resolution succeeded.
	AIMImageReasonRuntimeResolved = "RuntimeResolved"

	// AIMImageReasonRuntimeConfigMissing is set when the referenced runtime config cannot be found.
	AIMImageReasonRuntimeConfigMissing = "RuntimeConfigMissing"

	// AIMImageReasonDefaultRuntimeConfigMissing indicates the implicit default runtime config was not found.
	AIMImageReasonDefaultRuntimeConfigMissing = "DefaultRuntimeConfigMissing"

	// AIMImageConditionMetadataExtracted captures whether image metadata extraction succeeded.
	AIMImageConditionMetadataExtracted = "MetadataExtracted"

	// AIMImageReasonMetadataExtracted indicates metadata extraction succeeded.
	AIMImageReasonMetadataExtracted = "MetadataExtracted"

	// AIMImageReasonMetadataExtractionFailed indicates metadata extraction failed (non-blocking, prevents retries).
	AIMImageReasonMetadataExtractionFailed = "MetadataExtractionFailed"
)

// AIMImageStatusEnum represents the overall status of an AIMImage.
// +kubebuilder:validation:Enum=Pending;Progressing;Ready;Degraded;Failed
type AIMImageStatusEnum string

const (
	// AIMImageStatusPending indicates the image has been created but template generation has not started.
	AIMImageStatusPending AIMImageStatusEnum = "Pending"

	// AIMImageStatusProgressing indicates one or more templates are still being discovered.
	AIMImageStatusProgressing AIMImageStatusEnum = "Progressing"

	// AIMImageStatusReady indicates all templates are available and ready.
	AIMImageStatusReady AIMImageStatusEnum = "Ready"

	// AIMImageStatusDegraded indicates one or more templates are degraded or failed.
	AIMImageStatusDegraded AIMImageStatusEnum = "Degraded"

	// AIMImageStatusFailed indicates all templates are degraded or failed.
	AIMImageStatusFailed AIMImageStatusEnum = "Failed"
)

// AIMImageDiscoverySpec configures metadata discovery and template generation for an image.
type AIMImageDiscoverySpec struct {
	// Enabled toggles metadata discovery for this image. Disabled by default.
	Enabled bool `json:"enabled,omitempty"`

	// AutoCreateTemplates controls whether recommended deployments from discovery
	// automatically create ServiceTemplates. Enabled by default when discovery runs.
	// +optional
	AutoCreateTemplates *bool `json:"autoCreateTemplates,omitempty"`
}

// AIMImageSpec defines the desired state of AIMImage.
type AIMImageSpec struct {
	// Image is the container image URI for this AIM model.
	// This image is inspected by the operator to select runtime profiles used by templates.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// DefaultServiceTemplate is the default template to use for this image, if the user does not provide any
	DefaultServiceTemplate string `json:"defaultServiceTemplate,omitempty"`

	// Discovery controls metadata extraction and automatic template creation for this image.
	// +optional
	Discovery AIMImageDiscoverySpec `json:"discovery,omitempty"`
	// RuntimeConfigName references the AIM runtime configuration (by name) to use for this image.
	// +kubebuilder:default=default
	RuntimeConfigName string `json:"runtimeConfigName,omitempty"`

	// Resources defines the default resource requirements for services using this image.
	// Template- or service-level values override these defaults.
	// +kubebuilder:validation:Required
	// Must have both cpu and memory in requests
	// +kubebuilder:validation:XValidation:rule="has(self.requests) && 'cpu' in self.requests && 'memory' in self.requests",message="resources.requests must include cpu and memory"
	// Must have memory in limits
	// +kubebuilder:validation:XValidation:rule="has(self.limits) && 'memory' in self.limits",message="resources.limits must include memory"
	Resources corev1.ResourceRequirements `json:"resources"`
}

// AIMImageStatus defines the observed state of AIMImage.
type AIMImageStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Status represents the overall status of the image based on its templates
	// +kubebuilder:default=Pending
	Status AIMImageStatusEnum `json:"status,omitempty"`

	// Conditions represent the latest available observations of the model's state
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResolvedRuntimeConfig captures metadata about the runtime config that was resolved.
	// +optional
	ResolvedRuntimeConfig *AIMResolvedRuntimeConfig `json:"resolvedRuntimeConfig,omitempty"`

	// ImageMetadata is the metadata extracted from an AIM image
	// +optional
	ImageMetadata *ImageMetadata `json:"imageMetadata,omitempty"`
}

// AIMClusterImage is the Schema for cluster-scoped AIM image catalog entries.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimclimg,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AIMClusterImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMImageSpec   `json:"spec,omitempty"`
	Status AIMImageStatus `json:"status,omitempty"`
}

// AIMClusterImageList contains a list of AIMClusterImage.
// +kubebuilder:object:root=true
type AIMClusterImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterImage `json:"items"`
}

// AIMImage is the Schema for namespace-scoped AIM image catalog entries.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimimg,categories=aim;all
// +kubebuilder:printcolumn:name="Model ID",type=string,JSONPath=`.spec.modelId`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AIMImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMImageSpec   `json:"spec,omitempty"`
	Status AIMImageStatus `json:"status,omitempty"`
}

// AIMImageList contains a list of AIMImage.
// +kubebuilder:object:root=true
type AIMImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMImage `json:"items"`
}

// GetStatus returns a pointer to the AIMImage status.
func (img *AIMImage) GetStatus() *AIMImageStatus {
	return &img.Status
}

// GetStatus returns a pointer to the AIMClusterImage status.
func (img *AIMClusterImage) GetStatus() *AIMImageStatus {
	return &img.Status
}

func init() {
	SchemeBuilder.Register(&AIMClusterImage{}, &AIMClusterImageList{}, &AIMImage{}, &AIMImageList{})
}

// IsEnabled returns true when discovery is enabled.
func (d *AIMImageDiscoverySpec) IsEnabled() bool {
	return d != nil && d.Enabled
}

// AutoCreateTemplatesEnabled returns true when auto template creation should run.
// Defaults to true when unset.
func (d *AIMImageDiscoverySpec) AutoCreateTemplatesEnabled() bool {
	if d == nil || d.AutoCreateTemplates == nil {
		return true
	}
	return *d.AutoCreateTemplates
}

// DiscoveryEnabled reports whether discovery is enabled on the image spec.
func (spec *AIMImageSpec) DiscoveryEnabled() bool {
	if spec == nil {
		return false
	}
	return spec.Discovery.IsEnabled()
}

// AutoCreateTemplatesEnabled reports whether auto template creation is enabled on the image spec.
func (spec *AIMImageSpec) AutoCreateTemplatesEnabled() bool {
	if spec == nil {
		return true
	}
	return spec.Discovery.AutoCreateTemplatesEnabled()
}
