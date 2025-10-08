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
	"k8s.io/apimachinery/pkg/types"
)

// AIMBaseImageCacheSpec defines desired caching behaviour for a base image digest.
type AIMBaseImageCacheSpec struct {
	// Image is the base container image (ideally pinned by digest) that should be cached cluster-wide.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ServiceAccountName specifies the service account used by the caching DaemonSet.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// ImagePullSecrets contains registry pull secrets attached to the caching pods.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// NodeSelector restricts the nodes where caching pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// ParallelismLimit controls the maximum number of concurrent node pulls.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	ParallelismLimit *int32 `json:"parallelismLimit,omitempty"`
}

// AIMBaseImageCacheReference captures a consumer referencing the cache.
type AIMBaseImageCacheReference struct {
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace,omitempty"`
	UID       types.UID `json:"uid,omitempty"`
}

// AIMBaseImageCacheStatus reflects the observed state of the cache.
type AIMBaseImageCacheStatus struct {
	// ObservedGeneration is the last processed generation.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions provides high-level cache readiness and error states.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// NodesTotal tracks how many nodes are targeted by the cache.
	NodesTotal int32 `json:"nodesTotal,omitempty"`

	// NodesCached tracks how many nodes currently have the base image cached.
	NodesCached int32 `json:"nodesCached,omitempty"`

	// NodesFailed tracks nodes where caching failed.
	NodesFailed int32 `json:"nodesFailed,omitempty"`

	// RefCount summarises the number of active references.
	RefCount int32 `json:"refCount,omitempty"`

	// Refs enumerates resources currently referencing this cache.
	Refs []AIMBaseImageCacheReference `json:"refs,omitempty"`
}

// Condition types for AIMBaseImageCache.
const (
	// AIMBaseImageCacheConditionReady indicates the cache is fully present on targeted nodes.
	AIMBaseImageCacheConditionReady = "Ready"
	// AIMBaseImageCacheConditionProgressing indicates caching is in progress.
	AIMBaseImageCacheConditionProgressing = "Progressing"
	// AIMBaseImageCacheConditionFailure indicates a terminal failure.
	AIMBaseImageCacheConditionFailure = "Failure"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aimbic,categories=aim;all
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Cached",type=integer,JSONPath=`.status.nodesCached`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.nodesTotal`
// +kubebuilder:printcolumn:name="Refs",type=integer,JSONPath=`.status.refCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMBaseImageCache defines a DaemonSet-backed cache for a base container image.
type AIMBaseImageCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMBaseImageCacheSpec   `json:"spec,omitempty"`
	Status AIMBaseImageCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMBaseImageCacheList contains a list of AIMBaseImageCache.
type AIMBaseImageCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMBaseImageCache `json:"items"`
}

// GetStatus returns a pointer to the AIMBaseImageCache status.
func (c *AIMBaseImageCache) GetStatus() *AIMBaseImageCacheStatus {
	return &c.Status
}

func init() {
	SchemeBuilder.Register(&AIMBaseImageCache{}, &AIMBaseImageCacheList{})
}
