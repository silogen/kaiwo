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

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

// KaiwoServiceSpec defines the desired state of KaiwoService.
type KaiwoServiceSpec struct {
	CommonMetaSpec `json:",inline"`

	// ClusterQueue is the Kueue ClusterQueue name.
	ClusterQueue string `json:"clusterQueue,omitempty"`

	// PriorityClass specifies the Kubernetes PriorityClass for scheduling.
	PriorityClass string `json:"priorityClass,omitempty"`

	// EntryPoint specifies the command or script executed in a Deployment.
	// Can also be defined inside Deployment struct as regular command in the form of string array
	EntryPoint string `json:"entrypoint,omitempty"`

	// Defines the applications and deployments to deploy, should be a YAML multi-line scalar string.
	// Can also be defined inside RayService struct
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`

	// Optional workload-specific configs (Pointers to avoid bloating CRD)
	// +kubebuilder:pruning:PreserveUnknownFields
	RayService *rayv1.RayService `json:"rayService,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	Deployment *appsv1.Deployment `json:"deployment,omitempty"`
}

// KaiwoServiceStatus defines the observed state of KaiwoService.
type KaiwoServiceStatus struct {
	StartTime          *metav1.Time       `json:"startTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	Status             Status             `json:"status,omitempty"`
	Duration           int64              `json:"duration,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// KaiwoService
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="StartTime",type="string",JSONPath=".status.startTime"
// +kubebuilder:printcolumn:name="Duration(s)",type="integer",JSONPath=".status.duration"
type KaiwoService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaiwoServiceSpec   `json:"spec,omitempty"`
	Status KaiwoServiceStatus `json:"status,omitempty"`
}

// KaiwoServiceList
// +kubebuilder:object:root=true
type KaiwoServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoService `json:"items"`
}

func (svc *KaiwoService) GetUser() string {
	return svc.Spec.CommonMetaSpec.User
}

func (svc *KaiwoService) GetObjectMeta() *metav1.ObjectMeta {
	return &svc.ObjectMeta
}

func (spec *KaiwoServiceSpec) IsRayService() bool {
	return spec.RayService != nil || spec.Ray
}

func (svc *KaiwoService) GetStatus() string {
	return string(svc.Status.Status)
}

func (svc *KaiwoService) GetType() string {
	return "service"
}

func init() {
	SchemeBuilder.Register(&KaiwoService{}, &KaiwoServiceList{})
}

func (svc *KaiwoService) GetPods(ctx context.Context, k8sClient client.Client) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace(svc.Namespace), client.MatchingLabels{
		common.KaiwoRunIdLabel: string(svc.UID),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

func (svc *KaiwoService) GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}
