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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelCacheSpec defines the desired state of ModelCache
type ModelCacheSpec struct {
	// SourceURI is the source of the model to be downloaded. This is the only
	// identifier
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="sourceUri is immutable"
	// +kubebuilder:validation:Pattern=`^(hf|s3)://[^ \t\r\n]+$`
	SourceURI string `json:"sourceUri"`

	// StorageClassName specifies the storage class for the cache volume
	StorageClassName string `json:"storageClassName,omitempty"`

	// Size specifies the size of the cache volume
	// +kubebuilder:validation:XValidation:rule="self > quantity('0')",message="size must be greater than 0"
	Size resource.Quantity `json:"size"`

	// Env lists the environment variables required to download the model into the cache
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Progressing;Available;Failed
type ModelCacheStatusEnum string

const (
	// ModelCacheStatusPending denotes that the model cache has not been created yet
	ModelCacheStatusPending ModelCacheStatusEnum = "Pending"

	// ModelCacheStatusProgressing denotes that the model cache is currently being filled
	ModelCacheStatusProgressing ModelCacheStatusEnum = "Progressing"

	// ModelCacheStatusAvailable denotes that a model cache is filled and ready to be used
	ModelCacheStatusAvailable ModelCacheStatusEnum = "Available"

	// ModelCacheStatusFailed denotes that the model cache has failed. A more detailed reason will be available in the conditions.
	ModelCacheStatusFailed ModelCacheStatusEnum = "Failed"
)

// ModelCacheStatus defines the observed state of ModelCache
type ModelCacheStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the model cache's state
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status represents the current status of the model cache
	// +kubebuilder:default=Pending
	Status ModelCacheStatusEnum `json:"status,omitempty"`

	// LastUsed represents the last time a model was deployed that used this cache
	LastUsed *metav1.Time `json:"lastUsed,omitempty"`

	// PersistentVolumeClaim represents the name of the created PVC
	PersistentVolumeClaim string `json:"persistentVolumeClaim,omitempty"`
}

// Condition types
const (
	// ConditionProgressing is True when the cache is actively being prepared (PVC being bound, job running, etc.)
	ConditionProgressing = "Progressing"

	// ConditionReady is True when the cache is present and usable (PVC Bound & content populated)
	ConditionReady = "Ready"

	// ConditionStorageReady is True when storage backing the cache is provisioned and mounted (PVC Bound)
	ConditionStorageReady = "StorageReady"

	// ConditionFailure is True when the last warm/fill attempt has reached a terminal failure
	ConditionFailure = "Failure"
)

// Condition reasons
const (
	// StorageReady

	ReasonPVCProvisioning      = "PVCProvisioning"
	ReasonPVCBound             = "PVCBound"
	ReasonPVCPending           = "PVCPending"
	ReasonPVCLost              = "PVCLost"
	ReasonStorageClassMissing  = "StorageClassMissing"
	ReasonInsufficientCapacity = "InsufficientCapacity"

	// Progressing

	ReasonWaitingForPVC = "WaitingForPVC"
	ReasonDownloading   = "Downloading"
	ReasonRetryBackoff  = "RetryBackoff"

	// Ready

	ReasonWarm = "Warm"

	// Failure

	ReasonDownloadFailed = "DownloadFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mc,categories=aim;all
// +kubebuilder:printcolumn:name="Cache Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="PVC",type=string,JSONPath=`.status.persistentVolumeClaim`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ModelCache is the Schema for the modelcaches API
type ModelCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelCacheSpec   `json:"spec,omitempty"`
	Status ModelCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelCacheList contains a list of ModelCache
type ModelCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelCache{}, &ModelCacheList{})
}
