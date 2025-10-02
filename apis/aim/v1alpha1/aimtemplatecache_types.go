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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIMTemplateCacheSpec defines the desired state of AIMTemplateCache
type AIMTemplateCacheSpec struct {
	// TemplateRef is the name of the AIMServiceTemplate or AIMClusterServiceTemplate to cache.
	// The controller will first look for a namespace-scoped AIMServiceTemplate in the same namespace.
	// If not found, it will look for a cluster-scoped AIMClusterServiceTemplate with the same name.
	// Namespace-scoped templates take priority over cluster-scoped templates.
	// +kubebuilder:validation:MinLength=1
	TemplateRef string `json:"templateRef"`

	// Env specifies environment variables to use for authentication when downloading models.
	// These variables are used for authentication with model registries (e.g., HuggingFace tokens).
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// ImagePullSecrets references secrets for pulling AIM container images.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// StorageClassName is the name for the storage class to use for this cache
	StorageClassName string `json:"storageClassName,omitempty"`
}

// AIMTemplateCacheStatusEnum defines the status of the template cache.
// +kubebuilder:validation:Enum=Pending;Progressing;Available;Failed
type AIMTemplateCacheStatusEnum string

const (
	// AIMTemplateCacheStatusPending denotes that the template cache has been created but not yet processed.
	AIMTemplateCacheStatusPending AIMTemplateCacheStatusEnum = "Pending"
	// AIMTemplateCacheStatusProgressing denotes that the template cache is being warmed.
	AIMTemplateCacheStatusProgressing AIMTemplateCacheStatusEnum = "Progressing"
	// AIMTemplateCacheStatusAvailable denotes that the template cache is ready and models are cached.
	AIMTemplateCacheStatusAvailable AIMTemplateCacheStatusEnum = "Available"
	// AIMTemplateCacheStatusFailed denotes that the template cache operation has failed.
	AIMTemplateCacheStatusFailed AIMTemplateCacheStatusEnum = "Failed"
)

// AIMTemplateCacheStatus defines the observed state of AIMTemplateCache
type AIMTemplateCacheStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of the template cache state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status represents the current high-level status of the template cache.
	// +kubebuilder:default=Pending
	Status AIMTemplateCacheStatusEnum `json:"status,omitempty"`

	// ResolvedTemplateKind indicates whether the template resolved to a namespace-scoped
	// AIMServiceTemplate or cluster-scoped AIMClusterServiceTemplate.
	// Values: "AIMServiceTemplate", "AIMClusterServiceTemplate"
	ResolvedTemplateKind string `json:"resolvedTemplateKind,omitempty"`
}

// Condition types for AIMTemplateCache
const (
	// AIMTemplateCacheConditionResolved is True when the template reference has been resolved.
	AIMTemplateCacheConditionResolved = "Resolved"
	// AIMTemplateCacheConditionCacheWarm is True when the template's models are cached.
	AIMTemplateCacheConditionCacheWarm = "CacheWarm"
	// AIMTemplateCacheConditionReady is True when the template cache is ready.
	AIMTemplateCacheConditionReady = "Ready"
	// AIMTemplateCacheConditionProgressing is True when cache warming is in progress.
	AIMTemplateCacheConditionProgressing = "Progressing"
	// AIMTemplateCacheConditionFailure is True when a failure has occurred.
	AIMTemplateCacheConditionFailure = "Failure"
)

// Condition reasons for AIMTemplateCache
const (
	// Resolution related
	AIMTemplateCacheReasonTemplateNotFound = "TemplateNotFound"
	AIMTemplateCacheReasonResolved         = "Resolved"

	// Cache related
	AIMTemplateCacheReasonWarming = "Warming"
	AIMTemplateCacheReasonWarm    = "Warm"
	AIMTemplateCacheReasonFailed  = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimtc,categories=aim;all
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.status.resolvedTemplateKind`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMTemplateCache pre-warms model caches for a specified template.
type AIMTemplateCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMTemplateCacheSpec   `json:"spec,omitempty"`
	Status AIMTemplateCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMTemplateCacheList contains a list of AIMTemplateCache.
type AIMTemplateCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMTemplateCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMTemplateCache{}, &AIMTemplateCacheList{})
}
