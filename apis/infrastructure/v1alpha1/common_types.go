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
)

// ProfileReference references a PartitioningProfile.
type ProfileReference struct {
	// Name of the PartitioningProfile.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ExpectedResource defines an expected resource after partitioning.
type ExpectedResource struct {
	// Name is the resource name (e.g., "amd.com/gpu").
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Count is the expected number of resources.
	// +kubebuilder:validation:Minimum=0
	Count int32 `json:"count"`
}

// NodePartitioningPhase represents the current phase of node partitioning.
// +kubebuilder:validation:Enum=Pending;Draining;Applying;WaitingOperator;Verifying;Succeeded;Failed;Skipped
type NodePartitioningPhase string

const (
	// NodePartitioningPhasePending indicates the operation has not started yet.
	NodePartitioningPhasePending NodePartitioningPhase = "Pending"

	// NodePartitioningPhaseDraining indicates the node is being drained.
	NodePartitioningPhaseDraining NodePartitioningPhase = "Draining"

	// NodePartitioningPhaseApplying indicates the DCM profile is being applied.
	NodePartitioningPhaseApplying NodePartitioningPhase = "Applying"

	// NodePartitioningPhaseWaitingOperator indicates waiting for the GPU operator to reconcile.
	NodePartitioningPhaseWaitingOperator NodePartitioningPhase = "WaitingOperator"

	// NodePartitioningPhaseVerifying indicates verification is in progress.
	NodePartitioningPhaseVerifying NodePartitioningPhase = "Verifying"

	// NodePartitioningPhaseSucceeded indicates the operation completed successfully.
	NodePartitioningPhaseSucceeded NodePartitioningPhase = "Succeeded"

	// NodePartitioningPhaseFailed indicates the operation failed.
	NodePartitioningPhaseFailed NodePartitioningPhase = "Failed"

	// NodePartitioningPhaseSkipped indicates the operation was skipped (dry-run or paused).
	NodePartitioningPhaseSkipped NodePartitioningPhase = "Skipped"
)

// PartitioningPlanPhase represents the overall phase of a partitioning plan.
// +kubebuilder:validation:Enum=Pending;Progressing;Paused;Completed;Degraded
type PartitioningPlanPhase string

const (
	// PartitioningPlanPhasePending indicates the plan has not started yet.
	PartitioningPlanPhasePending PartitioningPlanPhase = "Pending"

	// PartitioningPlanPhaseProgressing indicates the plan is actively reconciling.
	PartitioningPlanPhaseProgressing PartitioningPlanPhase = "Progressing"

	// PartitioningPlanPhasePaused indicates the plan is paused.
	PartitioningPlanPhasePaused PartitioningPlanPhase = "Paused"

	// PartitioningPlanPhaseCompleted indicates all nodes have succeeded.
	PartitioningPlanPhaseCompleted PartitioningPlanPhase = "Completed"

	// PartitioningPlanPhaseDegraded indicates some nodes have failed.
	PartitioningPlanPhaseDegraded PartitioningPlanPhase = "Degraded"
)

// ErrorClass represents a normalized error classification for alerting.
// +kubebuilder:validation:Enum="";NodeNotFound;DrainTimeout;OperatorUnavailable;ApplyFailed;VerifyFailed;UnsupportedHardware
type ErrorClass string

const (
	// ErrorClassNone indicates no error.
	ErrorClassNone ErrorClass = ""

	// ErrorClassNodeNotFound indicates the target node does not exist.
	ErrorClassNodeNotFound ErrorClass = "NodeNotFound"

	// ErrorClassDrainTimeout indicates the drain operation timed out.
	ErrorClassDrainTimeout ErrorClass = "DrainTimeout"

	// ErrorClassOperatorUnavailable indicates the GPU operator is not available.
	ErrorClassOperatorUnavailable ErrorClass = "OperatorUnavailable"

	// ErrorClassApplyFailed indicates the profile application failed.
	ErrorClassApplyFailed ErrorClass = "ApplyFailed"

	// ErrorClassVerifyFailed indicates post-partition verification failed.
	ErrorClassVerifyFailed ErrorClass = "VerifyFailed"

	// ErrorClassUnsupportedHardware indicates the hardware does not support the requested profile.
	ErrorClassUnsupportedHardware ErrorClass = "UnsupportedHardware"
)

// Condition types for NodePartitioning
const (
	// NodePartitioningConditionNodeCordoned indicates whether the node has been cordoned.
	NodePartitioningConditionNodeCordoned = "NodeCordoned"

	// NodePartitioningConditionNodeTainted indicates whether the taint has been applied.
	NodePartitioningConditionNodeTainted = "NodeTainted"

	// NodePartitioningConditionDrainCompleted indicates whether the drain has completed.
	NodePartitioningConditionDrainCompleted = "DrainCompleted"

	// NodePartitioningConditionProfileApplied indicates whether the DCM profile has been applied.
	NodePartitioningConditionProfileApplied = "ProfileApplied"

	// NodePartitioningConditionOperatorReady indicates whether the GPU operator is ready.
	NodePartitioningConditionOperatorReady = "OperatorReady"

	// NodePartitioningConditionVerified indicates whether verification passed.
	NodePartitioningConditionVerified = "Verified"
)

// Condition types for PartitioningPlan
const (
	// PartitioningPlanConditionPlanReady indicates whether the plan is valid and ready.
	PartitioningPlanConditionPlanReady = "PlanReady"

	// PartitioningPlanConditionRolloutProgressing indicates whether the rollout is in progress.
	PartitioningPlanConditionRolloutProgressing = "RolloutProgressing"

	// PartitioningPlanConditionRolloutCompleted indicates whether the rollout has completed.
	PartitioningPlanConditionRolloutCompleted = "RolloutCompleted"

	// PartitioningPlanConditionRolloutDegraded indicates whether the rollout has failures.
	PartitioningPlanConditionRolloutDegraded = "RolloutDegraded"

	// PartitioningPlanConditionOperatorUnavailable indicates whether the GPU operator is unavailable.
	PartitioningPlanConditionOperatorUnavailable = "OperatorUnavailable"

	// PartitioningPlanConditionPaused indicates whether the plan is paused.
	PartitioningPlanConditionPaused = "Paused"
)

// NodePartitioningHistoryEntry records a state transition.
type NodePartitioningHistoryEntry struct {
	// At is the timestamp of the transition.
	At metav1.Time `json:"at"`

	// Phase is the phase at this point in history.
	Phase NodePartitioningPhase `json:"phase"`

	// Message is a human-readable description of the transition.
	Message string `json:"message"`
}

// NodeStatusSummary provides a lightweight summary of a node's partitioning status.
type NodeStatusSummary struct {
	// NodeName is the name of the node.
	NodeName string `json:"nodeName"`

	// DesiredHash is the desired state hash.
	DesiredHash string `json:"desiredHash"`

	// CurrentHash is the current state hash.
	// +optional
	CurrentHash string `json:"currentHash,omitempty"`

	// Phase is the current phase.
	Phase NodePartitioningPhase `json:"phase"`

	// Retries is the number of retry attempts so far.
	// +optional
	Retries int32 `json:"retries,omitempty"`

	// LastError is the most recent error message.
	// +optional
	LastError string `json:"lastError,omitempty"`

	// LastUpdateTime is the timestamp of the last status update.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
}

// RolloutPolicy defines how to orchestrate the rollout across multiple nodes.
type RolloutPolicy struct {
	// MaxParallel is the maximum number of nodes to process in parallel.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MaxParallel int32 `json:"maxParallel,omitempty"`

	// MaxUnavailable is the maximum number of nodes allowed to be unavailable at once.
	// Unavailable means: Draining, Applying, Verifying, or Failed.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	MaxUnavailable int32 `json:"maxUnavailable,omitempty"`

	// DrainPolicy defines how to drain nodes.
	// DrainPolicy DrainPolicy `json:"drainPolicy,omitempty"`
}

// PartitioningRule maps a node selector to a partition profile.
type PartitioningRule struct {
	// Name is an optional human-readable name for this rule.
	// +optional
	Name string `json:"name,omitempty"`

	// Selector selects nodes to partition.
	Selector metav1.LabelSelector `json:"selector"`

	// ProfileRef references the PartitioningProfile to apply.
	ProfileRef ProfileReference `json:"profileRef"`
}

// PlanSummary aggregates node counts by phase.
type PlanSummary struct {
	// TotalNodes is the total number of nodes tracked by the plan.
	// +optional
	TotalNodes int32 `json:"totalNodes,omitempty"`

	// MatchingNodes is the number of nodes matched by the plan's selectors.
	// +optional
	MatchingNodes int32 `json:"matchingNodes,omitempty"`

	// Pending is the number of nodes in Pending phase.
	// +optional
	Pending int32 `json:"pending,omitempty"`

	// Applying is the number of nodes in Applying or related phases.
	// +optional
	Applying int32 `json:"applying,omitempty"`

	// Verifying is the number of nodes in Verifying phase.
	// +optional
	Verifying int32 `json:"verifying,omitempty"`

	// Succeeded is the number of nodes that completed successfully.
	// +optional
	Succeeded int32 `json:"succeeded,omitempty"`

	// Failed is the number of nodes that failed.
	// +optional
	Failed int32 `json:"failed,omitempty"`

	// Skipped is the number of nodes that were skipped.
	// +optional
	Skipped int32 `json:"skipped,omitempty"`
}
