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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIMKVCacheSpec defines the desired state of AIMKVCache
type AIMKVCacheSpec struct {
	// KVCacheType specifies the type of key-value cache to create
	// +kubebuilder:validation:Enum=redis;mooncake
	// +kubebuilder:default=redis
	KVCacheType string `json:"kvCacheType"` // redis or mooncake
}

// +kubebuilder:validation:Enum=Pending;Progressing;Ready;Failed
type AIMKVCacheStatusEnum string

const (
	// AIMKVCacheStatusPending denotes that the KV cache is being created
	AIMKVCacheStatusPending AIMKVCacheStatusEnum = "Pending"

	// AIMKVCacheStatusProgressing denotes that the KV cache is being deployed
	AIMKVCacheStatusProgressing AIMKVCacheStatusEnum = "Progressing"

	// AIMKVCacheStatusReady denotes that the KV cache is ready to be used
	AIMKVCacheStatusReady AIMKVCacheStatusEnum = "Ready"

	// AIMKVCacheStatusFailed denotes that the KV cache deployment has failed
	AIMKVCacheStatusFailed AIMKVCacheStatusEnum = "Failed"
)

// AIMKVCacheStatus defines the observed state of AIMKVCache
type AIMKVCacheStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the KV cache's state
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status represents the current status of the KV cache
	// +kubebuilder:default=Pending
	Status AIMKVCacheStatusEnum `json:"status,omitempty"`

	// DeploymentName represents the name of the created deployment
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName represents the name of the created service
	ServiceName string `json:"serviceName,omitempty"`
}

func (m *AIMKVCache) GetStatus() *AIMKVCacheStatus {
	return &m.Status
}

// Condition types for AIMKVCache
const (
	// AIMKVCacheConditionProgressing is True when the cache is actively being deployed
	AIMKVCacheConditionProgressing = "Progressing"

	// AIMKVCacheConditionReady is True when the cache is ready to be used
	AIMKVCacheConditionReady = "Ready"

	// AIMKVCacheConditionFailure is True when deployment has failed
	AIMKVCacheConditionFailure = "Failure"
)

// Condition reasons for AIMKVCache
const (
	// Progressing-related reasons
	AIMKVCacheReasonDeploymentCreated = "DeploymentCreated"
	AIMKVCacheReasonWaitingForPods    = "WaitingForPods"

	// Ready-related reasons
	AIMKVCacheReasonDeploymentReady   = "DeploymentReady"
	AIMKVCacheReasonDeploymentPending = "DeploymentPending"

	// Failure-related reasons
	AIMKVCacheReasonNoFailure        = "NoFailure"
	AIMKVCacheReasonDeploymentFailed = "DeploymentFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimkvc,categories=aim;all
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.kvCacheType`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Deployment",type=string,JSONPath=`.status.deploymentName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIMKVCache is the Schema for the KV caches API
type AIMKVCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIMKVCacheSpec   `json:"spec,omitempty"`
	Status AIMKVCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIMKVCacheList contains a list of AIMKVCache
type AIMKVCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIMKVCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIMKVCache{}, &AIMKVCacheList{})
}
