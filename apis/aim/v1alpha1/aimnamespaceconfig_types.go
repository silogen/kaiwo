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
)

// SecretValueReference references a key within a Kubernetes Secret.
//
// Example:
//
// ```yaml
// secretName: hf-creds
// secretKey: token
// ```

// GatewayRef identifies a KGateway instance the namespace should use for exposure and routing.
// The operator can create or link to this instance according to policy.
type GatewayRef struct {
	// Name is the name of the gateway object.
	Name string `json:"name,omitempty"`
	// Namespace is the namespace where the gateway object resides.
	Namespace string `json:"namespace,omitempty"`
}

// AIMS3Credential configures S3 credential references for a specific endpoint.
// Provide an endpoint and secret references for authentication.
type AIMS3Credential struct {
	EndpointUrl     string             `json:"endpointUrl,omitempty"`
	AccessKeyId     v1.SecretReference `json:"accessKeyId,omitempty"`
	SecretAccessKey v1.SecretReference `json:"secretAccessKey,omitempty"`
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
	HuggingFaceToken *v1.SecretReference `json:"huggingFaceToken,omitempty"`
	// S3 contains one or more S3 credential configurations.
	S3 []AIMS3Credential `json:"s3,omitempty"`
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
//
// - `gateway`: KGateway instance to use for exposure and routing
// - `credentials`: secret references for model sources and caches (HF/S3/GCS/Azure)
//
// Example:
//
// ```yaml
// gateway:
//
//	name: kgw-default
//	namespace: gateway-system
//
// credentials:
//
//	huggingFaceToken:
//	  secretName: hf-creds
//	  secretKey: token
//	s3:
//	  - endpointUrl: https://s3.us-east-1.amazonaws.com
//	    accessKeyId: { secretName: s3-creds, secretKey: accessKeyId }
//	    secretAccessKey: { secretName: s3-creds, secretKey: secretAccessKey }
//
// ```
type AIMNamespaceConfigSpec struct {
	// Gateway references the KGateway instance to use for exposure and routing.
	Gateway GatewayRef `json:"gateway,omitempty"`
	// Credentials provide secret references used during model access and caching.
	Credentials AIMNamespaceCredentials `json:"credentials,omitempty"`

	// Images configures image pull behavior for AIM images within this namespace.
	Images AIMNamespaceImageConfig `json:"images,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=aimns,categories=kaiwo;all
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.gateway.name`
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
