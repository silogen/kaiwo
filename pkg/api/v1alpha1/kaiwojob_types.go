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
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadcommon "github.com/silogen/kaiwo/pkg/workloads/common"
)

// KaiwoJobSpec defines the desired state of KaiwoJob.
type KaiwoJobSpec struct {
	CommonMetaSpec `json:",inline"`

	// ClusterQueue is the Kueue ClusterQueue name.
	ClusterQueue string `json:"clusterQueue,omitempty"`

	// PriorityClass specifies the Kubernetes PriorityClass for scheduling.
	PriorityClass string `json:"priorityClass,omitempty"`

	// EntryPoint specifies the command or script executed in a Job or RayJob.
	// Can also be defined inside Job struct as Command in the form of string array or
	// inside RayJob struct as Entrypoint in the form of string
	EntryPoint string `json:"entrypoint,omitempty"`

	// RayJob defines the RayJob configuration.
	// +kubebuilder:pruning:PreserveUnknownFields
	RayJob *rayv1.RayJob `json:"rayJob,omitempty"`

	// Job defines the Kubernetes Job configuration.
	// +kubebuilder:pruning:PreserveUnknownFields
	Job *batchv1.Job `json:"job,omitempty"`
}

func (spec *KaiwoJobSpec) IsBatchJob() bool {
	return !spec.IsRayJob()
}

func (spec *KaiwoJobSpec) IsRayJob() bool {
	return spec.RayJob != nil || spec.Ray
}

// KaiwoJobStatus defines the observed state of KaiwoJob.
type KaiwoJobStatus struct {
	StartTime          *metav1.Time       `json:"startTime,omitempty"`
	CompletionTime     *metav1.Time       `json:"completionTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	Status             Status             `json:"status,omitempty"`
	Duration           int64              `json:"duration,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// KaiwoJob
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="StartTime",type="string",JSONPath=".status.startTime"
// +kubebuilder:printcolumn:name="CompletionTime",type="string",JSONPath=".status.completionTime"
// +kubebuilder:printcolumn:name="Duration(s)",type="integer",JSONPath=".status.duration"
type KaiwoJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoJobSpec   `json:"spec,omitempty"`
	Status KaiwoJobStatus `json:"status,omitempty"`
}

func (job *KaiwoJob) GetUser() string {
	return job.Spec.CommonMetaSpec.User
}

func (job *KaiwoJob) GetObjectMeta() *metav1.ObjectMeta {
	return &job.ObjectMeta
}

func (job *KaiwoJob) GetStatus() string {
	return string(job.Status.Status)
}

func (job *KaiwoJob) GetType() string {
	return "job"
}

func (job *KaiwoJob) GetPods(ctx context.Context, k8sClient client.Client) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace(job.Namespace), client.MatchingLabels{
		workloadcommon.KaiwoRunIdLabel: string(job.UID),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

func (job *KaiwoJob) GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

// KaiwoJobList
// +kubebuilder:object:root=true
type KaiwoJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KaiwoJob{}, &KaiwoJobList{})
}
