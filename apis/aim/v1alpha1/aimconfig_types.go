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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

// AIMS3Credential configures S3 credential references for a specific endpoint.
// Provide an endpoint and secret references for authentication.
type AIMS3Credential struct {
	// EndpointURL (S3-compatible). If both Region and EndpointURL are set, EndpointURL wins.
	EndpointURL string `json:"endpointUrl,omitempty"`

	// Region (AWS S3). If both Region and EndpointURL are set, EndpointURL wins.
	Region string `json:"region,omitempty"`

	AccessKeyID     v1.SecretKeySelector `json:"accessKeyId,omitempty"`
	SecretAccessKey v1.SecretKeySelector `json:"secretAccessKey,omitempty"`
}

// AIMConfigSpec defines the desired configuration for an AIM-enabled namespace.
type AIMConfigSpec struct {
	// Routing controls automatic HTTPRoute creation for AIM services in this namespace.
	// When enabled (default), the operator creates one HTTPRoute per service using
	// path-based routing with the pattern `/<namespace>/<workload_id>/` and attaches
	// it to the referenced Gateway listener.
	Routing AIMRoutingConfig `json:"routing,omitempty"`

	// CacheStorageClassName is the name of the storage class to use for cached models
	CacheStorageClassName string `json:"cacheStorageClassName,omitempty"`
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
// +kubebuilder:resource:scope=Cluster,shortName=aimns,categories=aim;all
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMClusterConfig configures credentials and routing at a cluster level.
type AIMClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AIMConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// AIMClusterConfigList contains a list of AIMClusterConfig.
type AIMClusterConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMClusterConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMClusterConfig{}, &AIMClusterConfigList{})
}
