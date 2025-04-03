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

	// ClusterQueue specifies the name of the Kueue `ClusterQueue` that the service should be submitted to for scheduling and resource management.
	//
	// This value is set as the `kueue.x-k8s.io/queue-name` label on the underlying Kubernetes Deployment or RayService.
	//
	// If omitted, it defaults to the value specified by the `DEFAULT_CLUSTER_QUEUE_NAME` environment variable in the Kaiwo controller (typically "kaiwo").
	//
	// The `kaiwo submit` CLI command can override this using the `--queue` flag or the `clusterQueue` field in the `kaiwoconfig.yaml` file.
	ClusterQueue string `json:"clusterQueue,omitempty"`

	// PriorityClass specifies the name of a Kubernetes `PriorityClass` to be assigned to the service's pods. This influences the scheduling priority relative to other pods in the cluster.
	PriorityClass string `json:"priorityClass,omitempty"`

	// EntryPoint specifies the command or script executed in a Deployment.
	// Can also be defined inside Deployment struct as regular command in the form of string array.
	//
	// It is *not* used when `ray: true` (use `serveConfigV2` or the `rayService` spec instead for Ray entrypoints).
	EntryPoint string `json:"entrypoint,omitempty"`

	// Defines the applications and deployments to deploy, should be a YAML multi-line scalar string.
	// Can also be defined inside RayService struct
	ServeConfigV2 string `json:"serveConfigV2,omitempty"`

	// RayService allows providing a full `rayv1.RayService` spec.
	//
	// If present (or `spec.ray` is `true`), Kaiwo creates a `RayService` (wrapped in an AppWrapper for Kueue integration) instead of a `Deployment`.
	//
	// Common fields are merged into the `RayClusterSpec` within this spec.
	//
	// Allows fine-grained control over the Ray cluster and Ray Serve configurations.
	// +kubebuilder:pruning:PreserveUnknownFields
	RayService *rayv1.RayService `json:"rayService,omitempty"`

	// Deployment allows providing a full `appsv1.Deployment` spec.
	//
	// If present and `spec.ray` is `false`, this is used as the base for the created `Deployment`.
	//
	// Common fields are merged into this spec.
	//
	// Allows fine-grained control over Kubernetes Deployment parameters (strategy, selectors, pod template, etc.).
	// +kubebuilder:pruning:PreserveUnknownFields
	Deployment *appsv1.Deployment `json:"deployment,omitempty"`
}

// KaiwoServiceStatus defines the observed state of KaiwoService.
type KaiwoServiceStatus struct {
	// StartTime records the timestamp when the first pod associated with the KaiwoService started running.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Conditions lists the observed conditions of the KaiwoService resource, following standard Kubernetes conventions. May include conditions reflecting the underlying Deployment or RayService state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status reflects the current high-level phase of the KaiwoService lifecycle (e.g., PENDING, STARTING, READY, FAILED).
	Status Status `json:"status,omitempty"`

	// Duration indicates how long the service has been running since StartTime, in seconds. Calculated periodically while running.
	Duration int64 `json:"duration,omitempty"`

	// ObservedGeneration records the `.metadata.generation` of the KaiwoService resource that was last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// KaiwoService represents a long-running service workload managed by Kaiwo. It encapsulates either a standard Kubernetes Deployment  or a RayService (via an AppWrapper), along with common metadata, storage configurations, and scheduling preferences. The Kaiwo controller reconciles this resource to create and manage the underlying workload objects.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="StartTime",type="string",JSONPath=".status.startTime"
// +kubebuilder:printcolumn:name="Duration(s)",type="integer",JSONPath=".status.duration"
type KaiwoService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KaiwoService, including workload type (Deployment/RayService), configuration, resources, and common metadata.
	Spec KaiwoServiceSpec `json:"spec,omitempty"`

	// Status reflects the most recently observed state of the KaiwoService, including its phase, start time, duration, and conditions.
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
