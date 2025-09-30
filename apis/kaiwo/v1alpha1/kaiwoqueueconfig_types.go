/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// This code includes portions from Kueue project, licensed under the Apache License 2.0.
// See https://github.com/kubernetes-sigs/kueue for details.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

type QueueConfigStatusDescription string

// StatusReady indicates a KaiwoService is fully deployed and ready to serve requests (Deployment ready or RayService healthy). Not applicable to KaiwoJob.
const (
	QueueConfigStatusReady  QueueConfigStatusDescription = "READY"
	QueueConfigStatusFailed QueueConfigStatusDescription = "FAILED"
)

// KaiwoQueueConfigSpec defines the desired configuration for Kaiwo's management of Kueue resources.
// There should typically be only one KaiwoQueueConfig resource in the cluster, named 'kaiwo'.
type KaiwoQueueConfigSpec struct {
	// ClusterQueues defines a list of Kueue ClusterQueues that Kaiwo should manage. Kaiwo ensures these ClusterQueues exist and match the provided specs.
	// +kubebuilder:validation:MaxItems=1000
	ClusterQueues []ClusterQueue `json:"clusterQueues,omitempty"`

	// ResourceFlavors defines a list of Kueue ResourceFlavors that Kaiwo should manage. Kaiwo ensures these ResourceFlavors exist and match the provided specs. If omitted or empty, Kaiwo attempts to automatically discover node pools and create default flavors based on node labels.
	// +kubebuilder:validation:MaxItems=20
	ResourceFlavors []ResourceFlavorSpec `json:"resourceFlavors,omitempty"`

	// WorkloadPriorityClasses defines a list of Kueue WorkloadPriorityClasses that Kaiwo should manage. Kaiwo ensures these priority classes exist with the specified values. See Kueue documentation for `WorkloadPriorityClass`.
	// +kubebuilder:validation:MaxItems=20
	WorkloadPriorityClasses []kueuev1beta1.WorkloadPriorityClass `json:"workloadPriorityClasses,omitempty"`

	// Topologies defines a list of Kueue Topologies that Kaiwo should manage. Kaiwo ensures these Topologies exist with the specified values. See Kueue documentation for `Topology`.
	// +kubebuilder:validation:MaxItems=10
	Topologies []Topology `json:"topologies,omitempty"`
}

// ClusterQueue defines the configuration for a Kueue ClusterQueue managed by Kaiwo.
type ClusterQueue struct {
	// Name specifies the name of the Kueue ClusterQueue resource.
	Name string `json:"name"`

	// Spec contains the desired Kueue `ClusterQueueSpec`. Kaiwo ensures the corresponding ClusterQueue resource matches this spec. See Kueue documentation for `ClusterQueueSpec` fields like `resourceGroups`, `cohort`, `preemption`, etc.
	Spec ClusterQueueSpec `json:"spec,omitempty"`

	// Namespaces optionally lists Kubernetes namespaces where Kaiwo should automatically create a Kueue `LocalQueue` resource pointing to this ClusterQueue.
	// If one or more namespaces are provided, the KaiwoQueueConfig controller takes over managing the LocalQueues for this ClusterQueue.
	// Leave this empty if you want to be able to create your own LocalQueues for this ClusterQueue.
	Namespaces []string `json:"namespaces,omitempty"`
}

type ClusterQueueSpec struct {
	// resourceGroups describes groups of resources.
	// Each resource group defines the list of resources and a list of flavors
	// that provide quotas for these resources.
	// Each resource and each flavor can only form part of one resource group.
	// resourceGroups can be up to 16.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=16
	ResourceGroups []kueuev1beta1.ResourceGroup `json:"resourceGroups,omitempty"`

	// cohort that this ClusterQueue belongs to. CQs that belong to the
	// same cohort can borrow unused resources from each other.
	//
	// A CQ can be a member of a single borrowing cohort. A workload submitted
	// to a queue referencing this CQ can borrow quota from any CQ in the cohort.
	// Only quota for the [resource, flavor] pairs listed in the CQ can be
	// borrowed.
	// If empty, this ClusterQueue cannot borrow from any other ClusterQueue and
	// vice versa.
	//
	// A cohort is a name that links CQs together, but it doesn't reference any
	// object.
	Cohort kueuev1beta1.CohortReference `json:"cohort,omitempty"`

	// QueueingStrategy indicates the queueing strategy of the workloads
	// across the queues in this ClusterQueue.
	// Current Supported Strategies:
	//
	// - StrictFIFO: workloads are ordered strictly by creation time.
	// Older workloads that can't be admitted will block admitting newer
	// workloads even if they fit available quota.
	// - BestEffortFIFO: workloads are ordered by creation time,
	// however older workloads that can't be admitted will not block
	// admitting newer workloads that fit existing quota.
	//
	// +kubebuilder:default=BestEffortFIFO
	// +kubebuilder:validation:Enum=StrictFIFO;BestEffortFIFO
	QueueingStrategy kueuev1beta1.QueueingStrategy `json:"queueingStrategy,omitempty"`

	// namespaceSelector defines which namespaces are allowed to submit workloads to
	// this clusterQueue. Beyond this basic support for policy, a policy agent like
	// Gatekeeper should be used to enforce more advanced policies.
	// Defaults to null which is a nothing selector (no namespaces eligible).
	// If set to an empty selector `{}`, then all namespaces are eligible.
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// flavorFungibility defines whether a workload should try the next flavor
	// before borrowing or preempting in the flavor being evaluated.
	// +kubebuilder:default={}
	FlavorFungibility *kueuev1beta1.FlavorFungibility `json:"flavorFungibility,omitempty"`

	// +kubebuilder:default={}
	Preemption *kueuev1beta1.ClusterQueuePreemption `json:"preemption,omitempty"`

	// admissionChecks lists the AdmissionChecks required by this ClusterQueue.
	// Cannot be used along with AdmissionCheckStrategy.
	// +optional
	AdmissionChecks []kueuev1beta1.AdmissionCheckReference `json:"admissionChecks,omitempty"`

	// admissionCheckStrategy defines a list of strategies to determine which ResourceFlavors require AdmissionChecks.
	// This property cannot be used in conjunction with the 'admissionChecks' property.
	// +optional
	AdmissionChecksStrategy *kueuev1beta1.AdmissionChecksStrategy `json:"admissionChecksStrategy,omitempty"`

	// stopPolicy - if set to a value different from None, the ClusterQueue is considered Inactive, no new reservation being
	// made.
	//
	// Depending on its value, its associated workloads will:
	//
	// - None - Workloads are admitted
	// - HoldAndDrain - Admitted workloads are evicted and Reserving workloads will cancel the reservation.
	// - Hold - Admitted workloads will run to completion and Reserving workloads will cancel the reservation.
	//
	// +optional
	// +kubebuilder:validation:Enum=None;Hold;HoldAndDrain
	// +kubebuilder:default="None"
	StopPolicy *kueuev1beta1.StopPolicy `json:"stopPolicy,omitempty"`

	// fairSharing defines the properties of the ClusterQueue when
	// participating in FairSharing.  The values are only relevant
	// if FairSharing is enabled in the Kueue configuration.
	// +optional
	FairSharing *kueuev1beta1.FairSharing `json:"fairSharing,omitempty"`
}

// ResourceFlavorSpec defines the configuration for a Kueue ResourceFlavor managed by Kaiwo.
type ResourceFlavorSpec struct {
	// Name specifies the name of the Kueue ResourceFlavor resource (e.g., "amd-mi300-8gpu").
	Name string `json:"name"`

	// NodeLabels specifies the labels that pods requesting this flavor must match on nodes. This is used by Kueue for scheduling decisions. Keys and values should correspond to actual node labels. Example: `{"kaiwo/nodepool": "amd-gpu-nodes"}`
	// +kubebuilder:validation:MaxProperties=10
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// Taints specifies a list of taints associated with this flavor.
	// +kubebuilder:validation:MaxItems=5
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Tolerations specifies a list of tolerations associated with this flavor. This is less common than using Taints; Kueue primarily uses Taints to derive Tolerations.
	// +kubebuilder:validation:MaxItems=5
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// TopologyName specifies the name of the Kueue Topology that this flavor belongs to. If specified, it must match one of the Topologies defined in the KaiwoQueueConfig.
	// This is used to group flavors by topology for scheduling purposes.
	TopologyName string `json:"topologyName,omitempty"`
}

// Topology is the Schema for the topology API
type Topology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec TopologySpec `json:"spec"`
}

type TopologySpec struct {
	// levels define the levels of topology.
	// +required
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:validation:XValidation:rule="size(self.filter(i, size(self.filter(j, j == i)) > 1)) == 0",message="must be unique"
	// +kubebuilder:validation:XValidation:rule="size(self.filter(i, i.nodeLabel == 'kubernetes.io/hostname')) == 0 || self[size(self) - 1].nodeLabel == 'kubernetes.io/hostname'",message="the kubernetes.io/hostname label can only be used at the lowest level of topology"
	Levels []kueuev1alpha1.TopologyLevel `json:"levels"`
}

// KaiwoQueueConfigStatus represents the observed state of KaiwoQueueConfig.
type KaiwoQueueConfigStatus struct {
	// Conditions lists the observed conditions of the KaiwoQueueConfig resource, such as whether the managed Kueue resources are synchronized and ready.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status reflects the overall status of the Kueue resource synchronization managed by this config (e.g., READY, FAILED).
	Status QueueConfigStatusDescription `json:"status,omitempty"`
}

// KaiwoQueueConfig manages Kueue resources like ClusterQueues, ResourceFlavors, and WorkloadPriorityClasses based on its spec. It acts as a central configuration point for Kaiwo's integration with Kueue. Typically, only one cluster-scoped resource named 'kaiwo' should exist. The controller ensures that the specified Kueue resources are created, updated, or deleted to match the desired state defined here.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="WorkloadStatus",type="string",JSONPath=".status.status"
// KaiwoQueueConfig manages Kueue resources.
type KaiwoQueueConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state for Kueue resources managed by Kaiwo.
	Spec KaiwoQueueConfigSpec `json:"spec,omitempty"`

	// Status reflects the most recently observed state of the Kueue resource synchronization.
	Status KaiwoQueueConfigStatus `json:"status,omitempty"`
}

// KaiwoQueueConfigList contains a list of KaiwoQueueConfig resources.
// +kubebuilder:object:root=true
type KaiwoQueueConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaiwoQueueConfig `json:"items"`
}

// Register Kaiwo CRDs
func init() {
	SchemeBuilder.Register(&KaiwoQueueConfig{}, &KaiwoQueueConfigList{})
}
