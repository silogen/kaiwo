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
	// AIMModelConditionRuntimeResolved captures whether runtime config resolution succeeded.
	AIMModelConditionRuntimeResolved = "RuntimeResolved"

	// AIMModelReasonRuntimeResolved indicates resolution succeeded.
	AIMModelReasonRuntimeResolved = "RuntimeResolved"

	// AIMModelReasonRuntimeConfigMissing is set when the referenced runtime config cannot be found.
	AIMModelReasonRuntimeConfigMissing = "RuntimeConfigMissing"

	// AIMModelReasonDefaultRuntimeConfigMissing indicates the implicit default runtime config was not found.
	AIMModelReasonDefaultRuntimeConfigMissing = "DefaultRuntimeConfigMissing"

	// AIMModelConditionMetadataExtracted captures whether image metadata extraction succeeded.
	AIMModelConditionMetadataExtracted = "MetadataExtracted"

	// AIMModelReasonMetadataExtracted indicates metadata extraction succeeded.
	AIMModelReasonMetadataExtracted = "MetadataExtracted"

	// AIMModelReasonMetadataExtractionFailed indicates metadata extraction failed (non-blocking, prevents retries).
	AIMModelReasonMetadataExtractionFailed = "MetadataExtractionFailed"
)

// AIMModelDiscoveryConfig controls discovery behavior for a model.
type AIMModelDiscoveryConfig struct {
	// Enabled controls whether discovery runs for this model.
	// When unset (nil), uses the runtime config's model.autoDiscovery setting.
	// When true, discovery always runs regardless of runtime config.
	// When false, discovery never runs regardless of runtime config.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// AutoCreateTemplates controls whether templates are auto-created from discovery results.
	// When unset, templates are created if discovery succeeds and returns recommended deployments.
	// When false, discovery runs but templates are not created (metadata extraction only).
	// When true, templates are always created from discovery results.
	// +optional
	AutoCreateTemplates *bool `json:"autoCreateTemplates,omitempty"`
}

// AIMModelStatusEnum represents the overall status of an AIMModel.
// +kubebuilder:validation:Enum=Pending;Progressing;Ready;Degraded;Failed
type AIMModelStatusEnum string

const (
	// AIMModelStatusPending indicates the image has been created but template generation has not started.
	AIMModelStatusPending AIMModelStatusEnum = "Pending"

	// AIMModelStatusProgressing indicates one or more templates are still being discovered.
	AIMModelStatusProgressing AIMModelStatusEnum = "Progressing"

	// AIMModelStatusReady indicates all templates are available and ready.
	AIMModelStatusReady AIMModelStatusEnum = "Ready"

	// AIMModelStatusDegraded indicates one or more templates are degraded or failed.
	AIMModelStatusDegraded AIMModelStatusEnum = "Degraded"

	// AIMModelStatusFailed indicates all templates are degraded or failed.
	AIMModelStatusFailed AIMModelStatusEnum = "Failed"
)

// AIMModelSpec defines the desired state of AIMModel.
type AIMModelSpec struct {
	// Image is the container image URI for this AIM model.
	// This image is inspected by the operator to select runtime profiles used by templates.
	// Discovery behavior is controlled by the discovery field and runtime config's AutoDiscovery setting.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Discovery controls discovery behavior for this model.
	// When unset, uses runtime config defaults.
	// +optional
	Discovery *AIMModelDiscoveryConfig `json:"discovery,omitempty"`

	// DefaultServiceTemplate is the default template to use for this image, if the user does not provide any
	DefaultServiceTemplate string `json:"defaultServiceTemplate,omitempty"`

	// RuntimeConfigName references the AIM runtime configuration (by name) to use for this image.
	// The runtime config controls discovery behavior and model creation scope.
	// +kubebuilder:default=default
	RuntimeConfigName string `json:"runtimeConfigName,omitempty"`

	// ImagePullSecrets lists secrets containing credentials for pulling the model container image.
	// These secrets are used for:
	// - OCI registry metadata extraction during discovery
	// - Pulling the image for inference services
	// The secrets are merged with any runtime config defaults.
	// For namespace-scoped models, secrets must exist in the same namespace.
	// For cluster-scoped models, secrets must exist in the operator namespace.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// ServiceAccountName specifies the Kubernetes service account to use for workloads related to this model.
	// This includes metadata extraction jobs and any other model-related operations.
	// If empty, the default service account for the namespace is used.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Resources defines the default resource requirements for services using this image.
	// Template- or service-level values override these defaults.
	// +Optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AIMModelStatus defines the observed state of AIMModel.
type AIMModelStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Status represents the overall status of the image based on its templates
	// +kubebuilder:default=Pending
	Status AIMModelStatusEnum `json:"status,omitempty"`

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

// AIMClusterModel is the Schema for cluster-scoped AIM model catalog entries.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimclmdl,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AIMClusterModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMModelSpec   `json:"spec,omitempty"`
	Status AIMModelStatus `json:"status,omitempty"`
}

// AIMClusterModelList contains a list of AIMClusterModel.
// +kubebuilder:object:root=true
type AIMClusterModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterModel `json:"items"`
}

// AIMModel is the Schema for namespace-scoped AIM model catalog entries.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimmdl,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AIMModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMModelSpec   `json:"spec,omitempty"`
	Status AIMModelStatus `json:"status,omitempty"`
}

// AIMModelList contains a list of AIMModel.
// +kubebuilder:object:root=true
type AIMModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMModel `json:"items"`
}

// GetStatus returns a pointer to the AIMModel status.
func (img *AIMModel) GetStatus() *AIMModelStatus {
	return &img.Status
}

// GetStatus returns a pointer to the AIMClusterModel status.
func (img *AIMClusterModel) GetStatus() *AIMModelStatus {
	return &img.Status
}

func init() {
	SchemeBuilder.Register(&AIMClusterModel{}, &AIMClusterModelList{}, &AIMModel{}, &AIMModelList{})
}
