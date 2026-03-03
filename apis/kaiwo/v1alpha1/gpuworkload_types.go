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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PreemptionPolicy determines when an idle GpuWorkload is eligible for preemption.
// +kubebuilder:validation:Enum=OnPressure;Always
type PreemptionPolicy string

const (
	PreemptionPolicyOnPressure PreemptionPolicy = "OnPressure"
	PreemptionPolicyAlways     PreemptionPolicy = "Always"
)

// AggregationPolicy determines how utilization is aggregated across multiple pods.
// +kubebuilder:validation:Enum=Min;Max;Avg
type AggregationPolicy string

const (
	AggregationPolicyMin AggregationPolicy = "Min"
	AggregationPolicyMax AggregationPolicy = "Max"
	AggregationPolicyAvg AggregationPolicy = "Avg"
)

// GpuWorkloadPhase represents the lifecycle phase of a tracked GPU workload.
// +kubebuilder:validation:Enum=PendingGpu;PendingOther;Active;Idle;Preempting;Preempted;Deleted;""
type GpuWorkloadPhase string

const (
	// GpuWorkloadPhasePendingGpu indicates pods are unschedulable specifically
	// due to insufficient GPU resources (validated via the scheduler condition
	// message). Acts as the demand signal for OnPressure preemption.
	GpuWorkloadPhasePendingGpu GpuWorkloadPhase = "PendingGpu"

	// GpuWorkloadPhasePendingOther indicates pods exist but are not yet
	// Running for non-GPU reasons: image pulls, init containers, PVC
	// binding, node affinity, taints, etc. No idle time is counted and
	// the phase does not trigger preemption evaluation.
	GpuWorkloadPhasePendingOther GpuWorkloadPhase = "PendingOther"

	// GpuWorkloadPhaseActive indicates pods are running and aggregated GPU
	// utilization is at or above the configured threshold.
	GpuWorkloadPhaseActive GpuWorkloadPhase = "Active"

	// GpuWorkloadPhaseIdle indicates pods are running but aggregated GPU
	// utilization is below the configured threshold.
	GpuWorkloadPhaseIdle GpuWorkloadPhase = "Idle"

	// GpuWorkloadPhasePreempting indicates the workload has been claimed for
	// preemption by the evaluator; deletion is in progress.
	GpuWorkloadPhasePreempting GpuWorkloadPhase = "Preempting"

	// GpuWorkloadPhasePreempted indicates the underlying workload was deleted
	// by the preemption system.
	GpuWorkloadPhasePreempted GpuWorkloadPhase = "Preempted"

	// GpuWorkloadPhaseDeleted indicates the underlying workload disappeared
	// on its own (not by preemption).
	GpuWorkloadPhaseDeleted GpuWorkloadPhase = "Deleted"
)

// WorkloadReference identifies the root owner resource that this GpuWorkload tracks.
type WorkloadReference struct {
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	UID types.UID `json:"uid"`
}

// PodGpuUtilization holds a utilization sample for a single GPU on a single pod.
type PodGpuUtilization struct {
	PodName string `json:"podName"`
	GpuID   string `json:"gpuId,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Utilization float64     `json:"utilization"`
	LastUpdate  metav1.Time `json:"lastUpdate"`
}

// GpuWorkloadSpec defines the desired state of a tracked GPU workload.
type GpuWorkloadSpec struct {
	// WorkloadRef identifies the root owner resource being tracked.
	WorkloadRef WorkloadReference `json:"workloadRef"`

	// GpuResources maps GPU resource names to the total count requested across
	// all pods belonging to this workload.
	// Example: {"amd.com/gpu": 4} or {"amd.com/gpu": 2, "amd.com/gpu-0": 4}.
	// Used for resource-name-aware pressure matching and capacity-aware fit
	// checks during preemption evaluation.
	GpuResources map[string]int `json:"gpuResources"`

	// UtilizationThreshold overrides the cluster-wide default (GPU_PREEMPTION_DEFAULT_THRESHOLD).
	// Utilization below this percentage is considered idle.
	// +optional
	UtilizationThreshold *float64 `json:"utilizationThreshold,omitempty"`

	// IfIdleAfter overrides the cluster-wide default (GPU_PREEMPTION_DEFAULT_IF_IDLE_AFTER).
	// The workload must be idle for at least this duration before becoming preemptible.
	// +optional
	IfIdleAfter *metav1.Duration `json:"ifIdleAfter,omitempty"`

	// PreemptionPolicy overrides the cluster-wide default (GPU_PREEMPTION_DEFAULT_POLICY).
	// +optional
	PreemptionPolicy *PreemptionPolicy `json:"preemptionPolicy,omitempty"`

	// AggregationPolicy overrides the cluster-wide default (GPU_PREEMPTION_DEFAULT_AGGREGATION).
	// Determines how utilization across multiple pods is aggregated.
	// +optional
	AggregationPolicy *AggregationPolicy `json:"aggregationPolicy,omitempty"`

	// TTLAfterFinished controls how long this CR is retained after the workload
	// reaches a terminal phase (Preempted or Deleted).
	// Falls back to GPU_PREEMPTION_DEFAULT_TTL. Use "0" to retain forever.
	// +optional
	TTLAfterFinished *metav1.Duration `json:"ttlAfterFinished,omitempty"`
}

// GpuWorkloadStatus defines the observed state of a tracked GPU workload.
type GpuWorkloadStatus struct {
	// Phase is the current lifecycle phase of the tracked workload.
	Phase GpuWorkloadPhase `json:"phase,omitempty"`

	// PodUtilizations holds per-pod, per-GPU utilization entries updated by the
	// metrics scraper.
	PodUtilizations []PodGpuUtilization `json:"podUtilizations,omitempty"`

	// AggregatedUtilization is computed by the reconciler from PodUtilizations
	// using the configured AggregationPolicy.
	AggregatedUtilization *float64 `json:"aggregatedUtilization,omitempty"`

	// LastMetricsUpdate records the last time the scraper wrote utilization data.
	LastMetricsUpdate *metav1.Time `json:"lastMetricsUpdate,omitempty"`

	// IdleSince records when the workload first entered the Idle phase.
	IdleSince *metav1.Time `json:"idleSince,omitempty"`

	// FinishedAt records when a terminal phase (Preempted or Deleted) was entered.
	// Used together with TTLAfterFinished for automatic CR cleanup.
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// OwnerChain describes the owner reference path from pod to root owner.
	// Example: "Pod/worker-0 -> Job/training -> KaiwoJob/my-job"
	OwnerChain string `json:"ownerChain,omitempty"`

	// PreemptedAt records when the preemption was initiated.
	PreemptedAt *metav1.Time `json:"preemptedAt,omitempty"`

	// PreemptedFor holds the name of the pending GpuWorkload this workload was
	// preempted to make room for.
	PreemptedFor string `json:"preemptedFor,omitempty"`

	// PreemptionReason provides a human-readable explanation of why the workload
	// was preempted.
	PreemptionReason string `json:"preemptionReason,omitempty"`

	// Conditions follows standard Kubernetes condition conventions.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GpuWorkload tracks a GPU workload for utilization monitoring and preemption.
// One GpuWorkload CR is created per tracked root-owner resource. The CR persists
// after the workload is deleted, providing audit history and preemption records.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Utilization",type="number",JSONPath=".status.aggregatedUtilization",format="float"
// +kubebuilder:printcolumn:name="Idle Since",type="date",JSONPath=".status.idleSince"
// +kubebuilder:printcolumn:name="Owner",type="string",JSONPath=".spec.workloadRef.kind"
// +kubebuilder:printcolumn:name="OwnerName",type="string",JSONPath=".spec.workloadRef.name"
type GpuWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GpuWorkloadSpec   `json:"spec,omitempty"`
	Status GpuWorkloadStatus `json:"status,omitempty"`
}

// GpuWorkloadList contains a list of GpuWorkload resources.
// +kubebuilder:object:root=true
type GpuWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GpuWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GpuWorkload{}, &GpuWorkloadList{})
}
