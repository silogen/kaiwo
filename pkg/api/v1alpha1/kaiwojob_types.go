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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaiwoJobSpec defines the desired state of KaiwoJob.
type KaiwoJobSpec struct {
	CommonMetaSpec `json:",inline"`

	// Unified workload parameters
	EntryPoint string `json:"entryPoint,omitempty"`

	// Optional workload-specific configs
	RayClusterSpec *RayClusterSpec `json:"rayClusterSpec,omitempty"`
	JobSpec        *JobSpec        `json:"jobSpec,omitempty"`
}

// KaiwoJobStatus defines the observed state of KaiwoJob.
type KaiwoJobStatus struct {
	StartTime       *metav1.Time       `json:"startTime,omitempty"`
	CompletionTime  *metav1.Time       `json:"completionTime,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
	ReplicaStatuses map[string]int32   `json:"replicaStatuses,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type KaiwoJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoJobSpec   `json:"spec,omitempty"`
	Status KaiwoJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type KaiwoJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoJob{}, &KaiwoJobList{})
}
