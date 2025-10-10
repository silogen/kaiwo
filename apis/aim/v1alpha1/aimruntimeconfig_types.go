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

// AIMRuntimeConfigCommon captures configuration fields shared across cluster and namespace scopes.
type AIMRuntimeConfigCommon struct {
	// DefaultStorageClassName is the storage class used for model caches when one is not
	// specified directly on the consumer resource.
	DefaultStorageClassName string `json:"defaultStorageClassName,omitempty"`
}

// AIMRuntimeConfigCredentials captures namespace-scoped authentication knobs.
type AIMRuntimeConfigCredentials struct {
	// ServiceAccountName is the service account used for discovery jobs, cache warmers,
	// and any other workloads spawned by the operator on behalf of this runtime config.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// ImagePullSecrets are merged with controller defaults when creating pods that need
	// to pull model or runtime images.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// AIMClusterRuntimeConfigSpec defines cluster-wide defaults for AIM resources.
type AIMClusterRuntimeConfigSpec struct {
	AIMRuntimeConfigCommon `json:",inline"`
}

// AIMRuntimeConfigSpec defines namespace-scoped overrides for AIM resources.
type AIMRuntimeConfigSpec struct {
	AIMRuntimeConfigCommon      `json:",inline"`
	AIMRuntimeConfigCredentials `json:",inline"`
}

// AIMRuntimeConfigStatus records the resolved config reference surfaced to consumers.
type AIMRuntimeConfigStatus struct {
	// ObservedGeneration is the last reconciled generation.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions communicate reconciliation progress.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AIMRuntimeConfigReference records the source runtime config used during resolution.
type AIMRuntimeConfigReference struct {
	// Name is the metadata.name of the runtime config.
	Name string `json:"name"`

	// Namespace is only set for namespace-scoped runtime configs.
	Namespace string `json:"namespace,omitempty"`

	// UID is included to detect stale references.
	UID types.UID `json:"uid,omitempty"`

	// Kind is either "AIMRuntimeConfig" or "AIMClusterRuntimeConfig".
	Kind string `json:"kind"`
}

// AIMEffectiveRuntimeConfig surfaces the resolved configuration applied to a consumer.
type AIMEffectiveRuntimeConfig struct {
	// NamespaceRef points at the namespace-scoped runtime config, if present.
	NamespaceRef *AIMRuntimeConfigReference `json:"namespaceRef,omitempty"`

	// ClusterRef points at the cluster-scoped runtime config, if present.
	ClusterRef *AIMRuntimeConfigReference `json:"clusterRef,omitempty"`

	// Hash is a stable hash of the merged configuration used for change detection.
	Hash string `json:"hash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=aimcrcfg,categories=aim;all
// +kubebuilder:printcolumn:name="CacheBaseImages",type=boolean,JSONPath=`.spec.cacheBaseImages`
// +kubebuilder:printcolumn:name="DefaultStorageClass",type=string,JSONPath=`.spec.defaultStorageClassName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMClusterRuntimeConfig defines cluster-scoped runtime defaults for AIM resources.
type AIMClusterRuntimeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMClusterRuntimeConfigSpec `json:"spec,omitempty"`
	Status AIMRuntimeConfigStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMClusterRuntimeConfigList contains a list of AIMClusterRuntimeConfig.
type AIMClusterRuntimeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterRuntimeConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=aimrcfg,categories=aim;all
// +kubebuilder:printcolumn:name="ServiceAccount",type=string,JSONPath=`.spec.serviceAccountName`
// +kubebuilder:printcolumn:name="CacheBaseImages",type=boolean,JSONPath=`.spec.cacheBaseImages`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMRuntimeConfig defines namespace-scoped runtime overrides for AIM resources.
type AIMRuntimeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMRuntimeConfigSpec   `json:"spec,omitempty"`
	Status AIMRuntimeConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AIMRuntimeConfigList contains a list of AIMRuntimeConfig.
type AIMRuntimeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMRuntimeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterRuntimeConfig{}, &AIMClusterRuntimeConfigList{}, &AIMRuntimeConfig{}, &AIMRuntimeConfigList{})
}
