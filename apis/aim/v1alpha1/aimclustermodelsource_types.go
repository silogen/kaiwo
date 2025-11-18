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

// AIMClusterModelSourceSpec defines the desired state of AIMClusterModelSource
type AIMClusterModelSourceSpec struct {
	// Registry is the container registry to synchronize from.
	// Defaults to docker.io if not specified.
	// Examples: "ghcr.io", "docker.io", "quay.io"
	// +optional
	// +kubebuilder:default="docker.io"
	Registry string `json:"registry,omitempty"`

	// Versions are global semantic version constraints applied to filters without specific versions.
	// Uses Masterminds/semver constraint syntax.
	// Examples: ">=1.0.0", "~1.2.0", "^2.0.0", ">=1.0.0 <2.0.0"
	// +optional
	Versions []string `json:"versions,omitempty"`

	// Filters define which container images to synchronize from the registry.
	// Each filter can specify image patterns and version constraints.
	// +optional
	Filters []ModelSourceFilter `json:"filters,omitempty"`

	// ImagePullSecrets are references to secrets for authenticating to the registry.
	// For cluster-scoped resources, secrets must exist in the operator namespace (kaiwo-system).
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// SyncInterval defines how often to check the registry for updates.
	// Defaults to 1 hour.
	// +optional
	// +kubebuilder:default="1h"
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
}

// ModelSourceFilter defines image selection criteria
type ModelSourceFilter struct {
	// Image is a wildcard pattern for matching image repository paths.
	// Supports glob-style wildcards with '*' matching any characters.
	// Pattern is matched against the repository path (excluding registry and tag).
	// Examples:
	//   - "aim-models/*" matches all images in the aim-models repository
	//   - "llama-*" matches all images starting with "llama-"
	//   - "*/mixtral-8x22b" matches "mixtral-8x22b" in any repository
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Versions are semantic version constraints for this image pattern.
	// If omitted (nil), uses spec.versions (global constraints).
	// If empty array ([]), includes all valid semver tags (no version filtering).
	// Uses Masterminds/semver constraint syntax.
	// Examples: ">=1.0.0", "~1.2.0", "^2.0.0"
	// +optional
	Versions []string `json:"versions,omitempty"`

	// Exclude are wildcard patterns to exclude from matching.
	// Processed after the Image pattern matches.
	// Examples: "*-dev", "*-experimental"
	// +optional
	Exclude []string `json:"exclude,omitempty"`
}

// AIMClusterModelSourceStatus defines the observed state of AIMClusterModelSource
type AIMClusterModelSourceStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the timestamp of the last successful synchronization with the registry.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// SyncedModelCount is the number of AIMClusterModel resources currently managed by this source.
	// +optional
	SyncedModelCount int32 `json:"syncedModelCount,omitempty"`

	// Conditions represent the latest available observations of the source's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimcms,categories=aim;all
// +kubebuilder:printcolumn:name="Registry",type=string,JSONPath=`.spec.registry`
// +kubebuilder:printcolumn:name="Models",type=integer,JSONPath=`.status.syncedModelCount`
// +kubebuilder:printcolumn:name="LastSync",type=date,JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIMClusterModelSource is the Schema for the aimclustermodelsources API.
// It automatically synchronizes container images from a registry and creates
// corresponding AIMClusterModel resources for discovered models.
type AIMClusterModelSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMClusterModelSourceSpec   `json:"spec,omitempty"`
	Status AIMClusterModelSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIMClusterModelSourceList contains a list of AIMClusterModelSource
type AIMClusterModelSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterModelSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterModelSource{}, &AIMClusterModelSourceList{})
}

// GetStatus returns the status subresource
func (s *AIMClusterModelSource) GetStatus() *AIMClusterModelSourceStatus {
	return &s.Status
}
