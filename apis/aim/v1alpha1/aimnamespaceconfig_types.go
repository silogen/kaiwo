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

//// AIMGCSCredential configures a GCS service account credential reference.
//// The referenced secret key should contain the JSON content of the service account.
//type AIMGCSCredential struct {
//	ApplicationCredentials v1.SecretReference `json:"applicationCredentials,omitempty"`
//}
//
//// AIMAzureBlobCredential configures an Azure Blob Storage connection string reference.
//type AIMAzureBlobCredential struct {
//	ConnectionString v1.SecretReference `json:"connectionString,omitempty"`
//}

// AIMNamespaceCredentials gathers optional credentials for model discovery and caching.
// One or more providers can be configured.
type AIMNamespaceCredentials struct {
	// HuggingFaceToken is the token used to access Hugging Face repositories.
	HuggingFaceToken *v1.SecretKeySelector `json:"huggingFaceToken,omitempty"`
	// S3 contains one or more S3 credential configurations.
	S3 *AIMS3Credential `json:"s3,omitempty"`
	//// GCS contains one or more GCS credential configurations.
	//GCS []AIMGCSCredential `json:"gcs,omitempty"`
	//// AzureBlob contains one or more Azure Blob credential configurations.
	//AzureBlob []AIMAzureBlobCredential `json:"azureBlob,omitempty"`
}

// AIMNamespaceImageConfig defines image pull configuration for AIM workloads in the namespace.
//
// Configure one or more image pull secrets to access private registries
// hosting AIM images referenced by AIMClusterModel entries.
//
// Example:
//
// ```yaml
// imagePullSecrets:
//   - my-regcred
//   - another-secret
//
// ```
type AIMNamespaceImageConfig struct {
	// ImagePullSecrets is the list of Secret names to use for pulling AIM images.
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`
}

// AIMNamespaceConfigSpec defines the desired configuration for an AIM-enabled namespace.
type AIMNamespaceConfigSpec struct {
	// Credentials provide secret references used during model access and caching.
	Credentials AIMNamespaceCredentials `json:"credentials,omitempty"`

	// Images configures image pull behavior for AIM images within this namespace.
	Images AIMNamespaceImageConfig `json:"images,omitempty"`

	// Routing controls automatic HTTPRoute creation for AIM services in this namespace.
	// When enabled (default), the operator creates one HTTPRoute per service using
	// path-based routing with the pattern `/<namespace>/<workload_id>/` and attaches
	// it to the referenced Gateway listener.
	Routing AIMNamespaceRoutingConfig `json:"routing,omitempty"`

	// CacheStorageClassName is the name of the storage class to use for cached models
	CacheStorageClassName string `json:"cacheStorageClassName,omitempty"`
}

// AIMNamespaceRoutingConfig controls automatic HTTPRoute provisioning in the namespace.
type AIMNamespaceRoutingConfig struct {
	// Gateway references the Gateway listener to use for exposure and routing
	// (mirrors HTTPRoute.parentRefs[*]).
	Gateway gatewayapi.ParentReference `json:"gateway,omitempty"`

	// AutoCreateRoute enables automatic HTTPRoute creation for AIM services.
	// +kubebuilder:default=true
	AutoCreateRoute bool `json:"autoCreateRoute,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=aimns,categories=aim;all
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.routing.gateway.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// AIMNamespaceConfig configures credentials and routing for a namespace.
// It is namespaced and typically created by a tenant administrator.
type AIMNamespaceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AIMNamespaceConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// AIMNamespaceConfigList contains a list of AIMNamespaceConfig.
type AIMNamespaceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMNamespaceConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMNamespaceConfig{}, &AIMNamespaceConfigList{})
}
