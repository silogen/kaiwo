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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func ObserveBatchJob(job *batchv1.Job, previousStatus kaiwo.WorkloadStatus) (kaiwo.WorkloadStatus, error) {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobSuccessCriteriaMet && condition.Status == corev1.ConditionTrue {
			// Job has succeeded
			return kaiwo.WorkloadStatusComplete, nil
		} else if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			// Job has failed
			return kaiwo.WorkloadStatusFailed, nil
		} else if condition.Type == batchv1.JobSuspended && condition.Status == corev1.ConditionTrue {
			// Job is suspended
			return kaiwo.WorkloadStatusPending, nil
		}
	}

	if job.Status.Failed > 0 {
		return kaiwo.WorkloadStatusFailed, nil
	}

	// Check if job has completed successfully by examining job status fields
	// A job is complete when it has succeeded pods and no active pods
	if job.Status.Succeeded > 0 && job.Status.Active == 0 {
		return kaiwo.WorkloadStatusComplete, nil
	}

	// If we are here, the job is not complete, failed or suspended

	if baseutils.ValueOrDefault(job.Status.Ready) > 0 {
		// Pods are running (not pending)
		return kaiwo.WorkloadStatusRunning, nil
	} else if previousStatus == kaiwo.WorkloadStatusRunning && job.Status.Active > 0 {
		// Before completing, a Job will have active but not ready pods and no other conditions
		// This check is added to avoid going back to starting phase
		// But only if there are still active pods - if no active pods, the job might be completing
		return previousStatus, nil
	} else if job.Status.Active > 0 {
		// Pods are running or pending
		return kaiwo.WorkloadStatusStarting, nil
	}

	return kaiwo.WorkloadStatusPending, nil
}
