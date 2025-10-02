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
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

// AIMConfigSpec defines the desired configuration for an AIM-enabled namespace.
type AIMConfigSpec struct {
	// Routing controls automatic HTTPRoute creation for AIM services in this namespace.
	// When enabled (default), the operator creates one HTTPRoute per service using
	// path-based routing with the pattern `/<namespace>/<workload_id>/` and attaches
	// it to the referenced Gateway listener.
	Routing AIMRoutingConfig `json:"routing,omitempty"`

	// CacheStorageClassName is the name of the storage class to use for cached models
	CacheStorageClassName string `json:"cacheStorageClassName,omitempty"`

	// ImagePullSecrets are a list of secrets that will be merged with any existing ones when referencing the AIM containers
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// AIMRoutingConfig controls automatic HTTPRoute provisioning in the namespace.
type AIMRoutingConfig struct {
	// Gateway references the Gateway listener to use for exposure and routing
	// (mirrors HTTPRoute.parentRefs[*]).
	Gateway gatewayapi.ParentReference `json:"gateway,omitempty"`

	// AutoCreateRoute enables automatic HTTPRoute creation for AIM services.
	// +kubebuilder:default=true
	AutoCreateRoute bool `json:"autoCreateRoute,omitempty"`
}

type AIMCachingConfig struct {
	// StorageClassName is the name of the storage class to use for cached models
	StorageClassName string `json:"storageClassName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=aimccfg,categories=aim;all
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMClusterConfig configures credentials and routing at a cluster level.
type AIMClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AIMConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// AIMConfigList contains a list of AIMClusterConfig.
type AIMConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterConfig{}, &AIMConfigList{})
}
