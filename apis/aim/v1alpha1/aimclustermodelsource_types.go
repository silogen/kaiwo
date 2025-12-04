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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AIMStatus string

const (
	// DefaultSyncInterval is the default interval between registry syncs (1 hour).
	DefaultSyncInterval             = 1 * time.Hour
	AIMStatusDegraded     AIMStatus = "Degraded"
	AIMStatusPending      AIMStatus = "Pending"
	AIMStatusStarting     AIMStatus = "Starting"
	AIMStatusProgressing  AIMStatus = "Progressing"
	AIMStatusReady        AIMStatus = "Ready"
	AIMStatusRunning      AIMStatus = "Running"
	AIMStatusNotAvailable AIMStatus = "NotAvailable"
	AIMStatusFailed       AIMStatus = "Failed"
)

// AIMClusterModelSource automatically discovers and syncs AI model images from container registries.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimclsrc,categories=aim;all
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Models",type=integer,JSONPath=`.status.discoveredModels`
// +kubebuilder:printcolumn:name="LastSync",type=date,JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AIMClusterModelSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMClusterModelSourceSpec   `json:"spec,omitempty"`
	Status AIMClusterModelSourceStatus `json:"status,omitempty"`
}

// AIMClusterModelSourceSpec defines the desired state of AIMClusterModelSource.
// +kubebuilder:validation:XValidation:rule="(self.registry == \"\" || self.registry == 'docker.io' || self.registry == 'hub.docker.com' || self.registry == 'index.docker.io') || !self.filters.exists(f, f.image.contains('*'))",message="Wildcard patterns in filters are only supported for docker.io registry. Other registries (ghcr.io, gcr.io, etc.) do not support repository discovery. Use exact repository names or switch to docker.io."
type AIMClusterModelSourceSpec struct {
	// Registry to sync from (e.g., docker.io, ghcr.io, gcr.io).
	// Defaults to docker.io if not specified.
	// +kubebuilder:default=docker.io
	// +optional
	Registry string `json:"registry,omitempty"`

	// ImagePullSecrets contains references to secrets for authenticating to private registries.
	// Secrets must exist in the operator namespace (typically kaiwo-system).
	// Used for both registry catalog listing and image metadata extraction.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Filters define which images to discover and sync.
	// Each filter specifies an image pattern with optional version constraints and exclusions.
	// Multiple filters are combined with OR logic (any match includes the image).
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Filters []ModelSourceFilter `json:"filters"`

	// SyncInterval defines how often to sync with the registry.
	// Defaults to 1h. Minimum recommended interval is 15m to avoid rate limiting.
	// Format: duration string (e.g., "30m", "1h", "2h30m").
	// +kubebuilder:default="1h"
	// +optional
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`

	// Versions specifies global semantic version constraints applied to all filters.
	// Individual filters can override this with their own version constraints.
	// Constraints use semver syntax: >=1.0.0, <2.0.0, ~1.2.0, ^1.0.0, etc.
	// Non-semver tags (e.g., "latest", "dev") are silently skipped.
	//
	// Version ranges work on all registries (including ghcr.io, gcr.io) when combined with
	// exact repository names (no wildcards). The controller uses the Tags List API to fetch
	// all tags for the repository and filters them by the semver constraint.
	//
	// Example: registry=ghcr.io, filters=[{image: "silogen/aim-llama"}], versions=[">=1.0.0"]
	// will fetch all tags from ghcr.io/silogen/aim-llama and include only those >=1.0.0.
	// +optional
	Versions []string `json:"versions,omitempty"`
}

// ModelSourceFilter defines a pattern for discovering images.
// Supports multiple formats:
// - Repository patterns: "org/repo*" - matches repositories with wildcards
// - Repository with tag: "org/repo:1.0.0" - exact tag match
// - Full URI: "ghcr.io/org/repo:1.0.0" - overrides registry and tag
// - Full URI with wildcard: "ghcr.io/org/repo*" - overrides registry, matches pattern
type ModelSourceFilter struct {
	// Image pattern with wildcard and full URI support.
	//
	// Supported formats:
	// - Repository pattern: "amdenterpriseai/aim-*"
	// - Repository with tag: "silogen/aim-llama:1.0.0" (overrides versions field)
	// - Full URI: "ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1" (overrides spec.registry and versions)
	// - Full URI with wildcard: "ghcr.io/silogen/aim-*" (overrides spec.registry)
	//
	// When a full URI is specified (including registry like ghcr.io), only images from that
	// registry will match. When a tag is included, it takes precedence over the versions field.
	//
	// Wildcard: * matches any sequence of characters.
	// +kubebuilder:validation:MaxLength=512
	Image string `json:"image"`

	// Exclude lists specific repository names to skip (exact match on repository name only, not registry).
	// Useful for excluding base images or experimental versions.
	//
	// Examples:
	// - ["amdenterpriseai/aim-base", "amdenterpriseai/aim-experimental"]
	// - ["silogen/aim-base"] - works with "ghcr.io/silogen/aim-*" (registry is not checked in exclusion)
	//
	// Note: Exclusions match against repository names (e.g., "silogen/aim-base"), not full URIs.
	// +optional
	Exclude []string `json:"exclude,omitempty"`

	// Versions specifies semantic version constraints for this filter.
	// If specified, overrides the global Versions field.
	// Only tags that parse as valid semver are considered (including prereleases like 0.8.1-rc1).
	// Ignored if the Image field includes an explicit tag (e.g., "repo:1.0.0").
	//
	// Examples: ">=1.0.0", "<2.0.0", "~1.2.0" (patch updates), "^1.0.0" (minor updates)
	//
	// Prerelease versions (e.g., 0.8.1-rc1) are supported and follow semver rules:
	// - 0.8.1-rc1 matches ">=0.8.0" (prerelease is part of version 0.8.1)
	// - Use ">=0.8.1-rc1" to match only that prerelease or higher
	// - Leave empty to match all tags (including prereleases and non-semver tags)
	// +optional
	Versions []string `json:"versions,omitempty"`
}

// AIMClusterModelSourceStatus defines the observed state of AIMClusterModelSource.
type AIMClusterModelSourceStatus struct {
	// Status represents the overall state of the model source.
	// +kubebuilder:validation:Enum=Pending;Starting;Progressing;Ready;Running;Degraded;NotAvailable;Failed
	// +optional
	Status string `json:"status,omitempty"`

	// LastSyncTime is the timestamp of the last successful registry sync.
	// Updated after each successful sync operation.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// DiscoveredModels is the count of AIMClusterModel resources managed by this source.
	// Includes both existing and newly created models.
	// +optional
	DiscoveredModels int `json:"discoveredModels,omitempty"`

	// DiscoveredImages provides a summary of recently discovered images.
	// Limited to avoid excessive status size. Typically shows the most recent 50 images.
	// +optional
	DiscoveredImages []DiscoveredImageInfo `json:"discoveredImages,omitempty"`

	// Conditions represent the latest available observations of the source's state.
	// Standard conditions: Ready, Syncing, RegistryReachable.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// DiscoveredImageInfo provides information about a discovered image.
type DiscoveredImageInfo struct {
	// Image is the full image reference (repository:tag).
	Image string `json:"image"`

	// Tag is the image tag.
	Tag string `json:"tag"`

	// ModelName is the name of the generated AIMClusterModel resource.
	ModelName string `json:"modelName"`

	// CreatedAt is when this image was first discovered.
	CreatedAt metav1.Time `json:"createdAt"`
}

// GetStatus returns a pointer to the status for use with the controller pipeline.
func (s *AIMClusterModelSource) GetStatus() *AIMClusterModelSourceStatus {
	return &s.Status
}

// GetConditions returns the status conditions.
func (s *AIMClusterModelSourceStatus) GetConditions() []metav1.Condition {
	return s.Conditions
}

// SetConditions sets the status conditions.
func (s *AIMClusterModelSourceStatus) SetConditions(conditions []metav1.Condition) {
	s.Conditions = conditions
}

// SetStatus sets the overall status string.
func (s *AIMClusterModelSourceStatus) SetStatus(status string) {
	s.Status = status
}

// AIMClusterModelSourceList contains a list of AIMClusterModelSource.
// +kubebuilder:object:root=true
type AIMClusterModelSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterModelSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterModelSource{}, &AIMClusterModelSourceList{})
}
