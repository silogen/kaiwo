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

	// If we are here, the job is not complete, failed or suspended

	if baseutils.ValueOrDefault(job.Status.Ready) > 0 {
		// Pods are running (not pending)
		return kaiwo.WorkloadStatusRunning, nil
	} else if previousStatus == kaiwo.WorkloadStatusRunning {
		// Before completing, a Job will have active but not ready pods and no other conditions
		// This check is added to avoid going back to starting phase
		return previousStatus, nil
	} else if job.Status.Active > 0 {
		// Pods are running or pending
		return kaiwo.WorkloadStatusStarting, nil
	}

	return kaiwo.WorkloadStatusPending, nil
}
