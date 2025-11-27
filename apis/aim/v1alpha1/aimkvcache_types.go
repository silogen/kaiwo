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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIMKVCacheSpec defines the desired state of AIMKVCache
type AIMKVCacheSpec struct {
	// KVCacheType specifies the type of key-value cache to create
	// +kubebuilder:validation:Enum=redis
	// +kubebuilder:default=redis
	KVCacheType string `json:"kvCacheType"` // redis

	// Image specifies the container image to use for the KV cache service.
	// If not specified, defaults to appropriate images based on KVCacheType:
	// - redis: redis:7.2.4
	// +optional
	Image *string `json:"image,omitempty"`

	// Env specifies environment variables to set in the KV cache container.
	// If not specified (nil), no additional environment variables are set.
	// If explicitly set to an empty array, no environment variables are added.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Storage defines the persistent storage configuration for the KV cache
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// Resources defines the resource requirements for the KV cache container.
	// If not specified, defaults to 1 CPU and 1Gi memory for both requests and limits.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// StorageSpec defines the persistent storage configuration
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.size) || !has(self.size) || self.size == oldSelf.size",message="storage size is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.storageClassName) || !has(self.storageClassName) || self.storageClassName == oldSelf.storageClassName",message="storage class name is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.accessModes) || self.accessModes == oldSelf.accessModes",message="access modes are immutable once set"
type StorageSpec struct {
	// Size specifies the storage size for the persistent volume.
	// Minimum recommended size is 1Gi for Redis to function properly.
	// If not specified, defaults to 1Gi.
	// WARNING: This field is immutable after creation due to StatefulSet VolumeClaimTemplate limitations.
	// +kubebuilder:default="1Gi"
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`

	// StorageClassName specifies the storage class to use for the persistent volume.
	// If not specified, the cluster's default storage class will be used.
	// Ensure your cluster has a default storage class configured or specify one explicitly.
	// WARNING: This field is immutable after creation due to StatefulSet VolumeClaimTemplate limitations.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// AccessModes specifies the access modes for the persistent volume.
	// Defaults to ReadWriteOnce if not specified.
	// WARNING: This field is immutable after creation due to StatefulSet VolumeClaimTemplate limitations.
	// +kubebuilder:default={ReadWriteOnce}
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
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

	// StatefulSetName represents the name of the created statefulset
	StatefulSetName string `json:"statefulSetName,omitempty"`

	// ServiceName represents the name of the created service
	ServiceName string `json:"serviceName,omitempty"`

	// Endpoint provides the connection information for accessing the KV cache.
	// Format depends on the backend type (e.g., "redis://service-name:6379" for Redis).
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Replicas is the total number of replicas configured for the StatefulSet.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of pods that are ready and serving traffic.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// StorageSize represents the total storage capacity allocated for the KV cache.
	// This reflects the size specified in the PersistentVolumeClaim.
	// +optional
	StorageSize string `json:"storageSize,omitempty"`

	// LastError contains details about the most recent error encountered.
	// This field is cleared when the error is resolved.
	// +optional
	LastError string `json:"lastError,omitempty"`
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
	AIMKVCacheReasonStatefulSetCreated = "StatefulSetCreated"
	AIMKVCacheReasonWaitingForPods     = "WaitingForPods"

	// Ready-related reasons
	AIMKVCacheReasonStatefulSetReady   = "StatefulSetReady"
	AIMKVCacheReasonStatefulSetPending = "StatefulSetPending"

	// Failure-related reasons
	AIMKVCacheReasonNoFailure         = "NoFailure"
	AIMKVCacheReasonStatefulSetFailed = "StatefulSetFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aimkvc,categories=aim;all
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.kvCacheType`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`,priority=1
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
