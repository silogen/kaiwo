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

package observe

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/storage/download"
)

// UnitPhase represents the normalized status of any child resource
type UnitPhase string

const (
	UnitPending     UnitPhase = "Pending"     // created but not doing work yet
	UnitProgressing UnitPhase = "Progressing" // making forward progress
	UnitReady       UnitPhase = "Ready"       // steady-state workloads
	UnitSucceeded   UnitPhase = "Succeeded"   // terminal for batch
	UnitDegraded    UnitPhase = "Degraded"    // kube says it's unhealthy (e.g., deadline exceeded)
	UnitStopped     UnitPhase = "Stopped"     // intentionally terminated by user/action (terminal for batch)
	UnitSuspended   UnitPhase = "Suspended"   // intentionally paused/quiesced (non-terminal)
	UnitFailed      UnitPhase = "Failed"      // terminal error
	UnitUnknown     UnitPhase = "Unknown"
)

// UnitGroup represents the logical grouping of units
type UnitGroup string

const (
	GroupPrereqs  UnitGroup = "Prereqs"
	GroupWorkload UnitGroup = "Workload"
	GroupPost     UnitGroup = "Post"
)

// Reason constants for consistent error and status reporting
const (
	ReasonGetError                 = "GetError"
	ReasonObserveError             = "ObserveError"
	ReasonWaitingForPVC            = "WaitingForPVC"
	ReasonPVCPending               = "PVCPending"
	ReasonPVCLost                  = "PVCLost"
	ReasonPaused                   = "Paused"
	ReasonScaledToZero             = "ScaledToZero"
	ReasonDeleting                 = "Deleting"
	ReasonJobFailed                = "JobFailed"
	ReasonJobStopped               = "JobStopped"
	ReasonUnknownStatus            = "UnknownStatus"
	ReasonUnknownPhase             = "UnknownPhase"
	ReasonRayDeploymentUnhealthy   = "RayDeploymentUnhealthy"
	ReasonRayServiceAppUnhealthy   = "RayServiceAppUnhealthy"
	ReasonPvcResizing              = "PvcResizing"
	ReasonRolloutInProgress        = "RolloutInProgress"
	ReasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"
	ReasonApplicationFailed        = "ApplicationFailed"
	ReasonDownloadJobPending       = "DownloadJobPending"
	ReasonDownloadJobInProgress    = "DownloadJobInProgress"
	ReasonDownloadJobCompleted     = "DownloadJobCompleted"
	ReasonDownloadJobFailed        = "DownloadJobFailed"
)

// UnitStatus provides a normalized view of any child resource status
type UnitStatus struct {
	Name               string
	Kind               string
	Group              UnitGroup // logical grouping: "Prereqs" | "Workload" | "Post"
	Phase              UnitPhase
	Ready              bool // a quick boolean (for Deployments)
	ObservedGeneration int64
	Reason             string
	Message            string
	Conditions         []metav1.Condition // verbatim from the child if useful
}

// WorkloadPhase represents the high-level state machine for the owner workload
type WorkloadPhase string

const (
	PhasePlanning       WorkloadPhase = "Planning"       // new spec or generation bump
	PhasePendingPrereqs WorkloadPhase = "PendingPrereqs" // waiting on PVCs, pre-download
	PhaseDeploying      WorkloadPhase = "Deploying"      // main workload rolling out
	PhaseRunning        WorkloadPhase = "Running"        // steady-state (services)
	PhaseSucceeded      WorkloadPhase = "Succeeded"      // terminal (jobs)
	PhaseSuspended      WorkloadPhase = "Suspended"      // workload deliberately quiesced (service-like)
	PhaseStopped        WorkloadPhase = "Stopped"        // workload intentionally stopped (job-like terminal)
	PhaseFailed         WorkloadPhase = "Failed"         // terminal
	PhaseDeleting       WorkloadPhase = "Deleting"
)

// Observer interface for reading resource status
type Observer interface {
	Observe(ctx context.Context, c client.Client) (UnitStatus, error)
	Kind() string
}

// AggregateResult summarizes the status of all observed units
type AggregateResult struct {
	PrereqsReady      bool
	WorkloadAvailable bool
	AnyDegraded       bool
	AnyFailed         bool
	Messages          []string
}

// Observation contains the complete result of observing a workload
type Observation struct {
	Phase      WorkloadPhase
	Units      []UnitStatus
	Conditions []metav1.Condition
}

// Reduce aggregates a list of UnitStatus into an AggregateResult
func Reduce(units []UnitStatus) AggregateResult {
	var r AggregateResult
	for _, u := range units {
		if u.Message != "" {
			r.Messages = append(r.Messages, fmt.Sprintf("%s/%s: %s", u.Kind, u.Name, u.Message))
		}
	}

	// Groups: Prereqs vs Workload
	prereqs := filter(units, func(u UnitStatus) bool { return u.Group == GroupPrereqs })
	workload := filter(units, func(u UnitStatus) bool { return u.Group == GroupWorkload })

	r.PrereqsReady = all(prereqs, func(u UnitStatus) bool {
		return u.Phase == UnitReady || u.Phase == UnitSucceeded
	})
	r.WorkloadAvailable = all(workload, func(u UnitStatus) bool {
		return u.Phase == UnitReady || u.Phase == UnitSucceeded
	})

	r.AnyFailed = any(units, func(u UnitStatus) bool { return u.Phase == UnitFailed })
	r.AnyDegraded = any(units, func(u UnitStatus) bool { return u.Phase == UnitDegraded })

	return r
}

// ConditionSet helps build conditions consistently
type ConditionSet struct {
	conditions []metav1.Condition
}

func NewConditionSet() *ConditionSet {
	return &ConditionSet{conditions: make([]metav1.Condition, 0)}
}

func (cs *ConditionSet) Set(conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	cs.conditions = append(cs.conditions, condition)
}

func (cs *ConditionSet) Done() []metav1.Condition {
	return cs.conditions
}

// Decide maps AggregateResult + owner kind into (WorkloadPhase, Conditions)
func Decide(owner client.Object, agg AggregateResult, units []UnitStatus) (WorkloadPhase, []metav1.Condition) {
	cs := NewConditionSet()

	// terminal / admin gates
	if isDeleting(owner) {
		cs.Set("Progressing", metav1.ConditionTrue, "Deleting", "Owner is being deleted")
		return PhaseDeleting, cs.Done()
	}

	// failures first
	if agg.AnyFailed {
		cs.Set("Failed", metav1.ConditionTrue, "ChildFailed", firstMessage(agg))
		cs.Set("Ready", metav1.ConditionFalse, "NotReady", "")
		return PhaseFailed, cs.Done()
	}

	// prereqs
	if !agg.PrereqsReady {
		cs.Set("PrereqsReady", metav1.ConditionFalse, "WaitingForPrereqs", firstMessage(agg))
		cs.Set("Progressing", metav1.ConditionTrue, "Reconciling", "")
		
		// Set download job condition based on download job status
		setDownloadJobCondition(cs, units)
		
		return PhasePendingPrereqs, cs.Done()
	}
	cs.Set("PrereqsReady", metav1.ConditionTrue, "AllPrereqsReady", "")
	
	// Set download job succeeded condition when prereqs are ready
	setDownloadJobCondition(cs, units)

	// main workload
	if isJob(owner) {
		if workloadSucceeded(units) {
			cs.Set("Succeeded", metav1.ConditionTrue, "JobSucceeded", "")
			cs.Set("Ready", metav1.ConditionTrue, "Ready", "")
			cs.Set("Progressing", metav1.ConditionFalse, "Idle", "")
			return PhaseSucceeded, cs.Done()
		}
		if workloadDegraded(units) {
			cs.Set("Failed", metav1.ConditionTrue, "JobFailed", firstMessage(agg))
			cs.Set("Ready", metav1.ConditionFalse, "NotReady", "")
			return PhaseFailed, cs.Done()
		}
		cs.Set("Progressing", metav1.ConditionTrue, "JobRunning", "")
		cs.Set("Ready", metav1.ConditionFalse, "NotReady", "")
		return PhaseDeploying, cs.Done()
	}

	// Service-like
	if !agg.WorkloadAvailable {
		cs.Set("WorkloadAvailable", metav1.ConditionFalse, "RollingOut", firstMessage(agg))
		cs.Set("Progressing", metav1.ConditionTrue, "Reconciling", "")
		cs.Set("Ready", metav1.ConditionFalse, "NotReady", "")
		return PhaseDeploying, cs.Done()
	}
	cs.Set("WorkloadAvailable", metav1.ConditionTrue, "Available", "")
	cs.Set("Progressing", metav1.ConditionFalse, "Idle", "")
	cs.Set("Ready", metav1.ConditionTrue, "Ready", "")
	return PhaseRunning, cs.Done()
}

// Helper functions
func filter[T interface{}](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range slice {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

func all[T interface{}](slice []T, predicate func(T) bool) bool {
	for _, item := range slice {
		if !predicate(item) {
			return false
		}
	}
	return true
}

func any[T interface{}](slice []T, predicate func(T) bool) bool {
	for _, item := range slice {
		if predicate(item) {
			return true
		}
	}
	return false
}

func isDeleting(owner client.Object) bool {
	return owner.GetDeletionTimestamp() != nil
}

func firstMessage(agg AggregateResult) string {
	if len(agg.Messages) > 0 {
		return agg.Messages[0]
	}
	return ""
}

func isJob(owner client.Object) bool {
	gvk := owner.GetObjectKind().GroupVersionKind()
	return gvk.Kind == "KaiwoJob"
}

func workloadSucceeded(units []UnitStatus) bool {
	workloadUnits := filter(units, func(u UnitStatus) bool { return u.Group == GroupWorkload })
	return any(workloadUnits, func(u UnitStatus) bool { return u.Phase == UnitSucceeded })
}

func workloadDegraded(units []UnitStatus) bool {
	workloadUnits := filter(units, func(u UnitStatus) bool { return u.Group == GroupWorkload })
	return any(workloadUnits, func(u UnitStatus) bool { return u.Phase == UnitDegraded || u.Phase == UnitFailed })
}

// Map WorkloadPhase to WorkloadStatus for backward compatibility
func WorkloadPhaseToStatus(phase WorkloadPhase) v1alpha1.WorkloadStatus {
	switch phase {
	case PhasePlanning:
		return v1alpha1.WorkloadStatusNew
	case PhasePendingPrereqs:
		return v1alpha1.WorkloadStatusDownloading
	case PhaseDeploying:
		return v1alpha1.WorkloadStatusStarting
	case PhaseRunning:
		return v1alpha1.WorkloadStatusRunning
	case PhaseSucceeded:
		return v1alpha1.WorkloadStatusComplete
	case PhaseFailed:
		return v1alpha1.WorkloadStatusFailed
	case PhaseDeleting:
		return v1alpha1.WorkloadStatusTerminating
	default:
		return v1alpha1.WorkloadStatusNew
	}
}

// setDownloadJobCondition sets the DownloadJobSucceeded condition based on download job status in prereqs
func setDownloadJobCondition(cs *ConditionSet, units []UnitStatus) {
	downloadJobs := filter(units, func(u UnitStatus) bool {
		return u.Group == GroupPrereqs && u.Kind == "Job"
	})
	
	if len(downloadJobs) == 0 {
		// No download jobs, so the condition doesn't apply
		return
	}
	
	// Check if all download jobs are successful
	allDownloadsSucceeded := all(downloadJobs, func(u UnitStatus) bool {
		return u.Phase == UnitSucceeded
	})
	
	// Check if any download jobs failed
	anyDownloadsFailed := any(downloadJobs, func(u UnitStatus) bool {
		return u.Phase == UnitFailed
	})
	
	// Check if any download jobs are still progressing
	anyDownloadsProgressing := any(downloadJobs, func(u UnitStatus) bool {
		return u.Phase == UnitProgressing
	})
	
	if allDownloadsSucceeded {
		cs.Set(download.DownloadJobSucceededConditionType, metav1.ConditionTrue, ReasonDownloadJobCompleted, "All download jobs succeeded")
	} else if anyDownloadsFailed {
		cs.Set(download.DownloadJobSucceededConditionType, metav1.ConditionFalse, ReasonDownloadJobFailed, "Download job failed")
	} else if anyDownloadsProgressing {
		cs.Set(download.DownloadJobSucceededConditionType, metav1.ConditionFalse, ReasonDownloadJobInProgress, "Download job in progress")
	} else {
		cs.Set(download.DownloadJobSucceededConditionType, metav1.ConditionFalse, ReasonDownloadJobPending, "Download job pending")
	}
}
