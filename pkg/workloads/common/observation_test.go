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

package common

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

func TestReduce(t *testing.T) {
	units := []UnitStatus{
		{
			Name:  "pvc-1",
			Kind:  "PersistentVolumeClaim",
			Group: "Prereqs",
			Phase: UnitReady,
		},
		{
			Name:  "job-1",
			Kind:  "Job",
			Group: "Workload",
			Phase: UnitProgressing,
		},
	}

	result := Reduce(units)

	if !result.PrereqsReady {
		t.Error("Expected prereqs to be ready")
	}
	if result.WorkloadAvailable {
		t.Error("Expected workload to not be available yet")
	}
	if result.AnyFailed {
		t.Error("Expected no failures")
	}
	if result.AnyDegraded {
		t.Error("Expected no degraded units")
	}
}

func TestWorkloadPhaseToStatus(t *testing.T) {
	tests := []struct {
		phase          WorkloadPhase
		expectedStatus v1alpha1.WorkloadStatus
	}{
		{PhasePlanning, v1alpha1.WorkloadStatusNew},
		{PhasePendingPrereqs, v1alpha1.WorkloadStatusPending},
		{PhaseDeploying, v1alpha1.WorkloadStatusStarting},
		{PhaseRunning, v1alpha1.WorkloadStatusRunning},
		{PhaseSucceeded, v1alpha1.WorkloadStatusComplete},
		{PhaseFailed, v1alpha1.WorkloadStatusFailed},
		{PhaseDeleting, v1alpha1.WorkloadStatusTerminating},
		{PhasePaused, v1alpha1.WorkloadStatusPending},
	}

	for _, test := range tests {
		result := WorkloadPhaseToStatus(test.phase)
		if result != test.expectedStatus {
			t.Errorf("Expected phase %s to map to status %s, got %s",
				test.phase, test.expectedStatus, result)
		}
	}
}

func TestDecide(t *testing.T) {
	// Create a mock workload object
	mockWorkload := &v1alpha1.KaiwoJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}
	mockWorkload.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("KaiwoJob"))

	// Test case: All prereqs ready, workload succeeded
	units := []UnitStatus{
		{Name: "pvc-1", Kind: "PVC", Group: "Prereqs", Phase: UnitReady},
		{Name: "job-1", Kind: "Job", Group: "Workload", Phase: UnitSucceeded},
	}
	agg := Reduce(units)
	phase, conditions := Decide(mockWorkload, agg, units)

	if phase != PhaseSucceeded {
		t.Errorf("Expected phase %s, got %s", PhaseSucceeded, phase)
	}

	// Check that appropriate conditions are set
	hasSucceeded := false
	hasReady := false
	for _, condition := range conditions {
		if condition.Type == "Succeeded" && condition.Status == metav1.ConditionTrue {
			hasSucceeded = true
		}
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			hasReady = true
		}
	}

	if !hasSucceeded {
		t.Error("Expected Succeeded condition to be true")
	}
	if !hasReady {
		t.Error("Expected Ready condition to be true")
	}
}
