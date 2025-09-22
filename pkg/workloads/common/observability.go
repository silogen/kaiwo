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
    // Prefer explicit conditions
    for _, condition := range job.Status.Conditions {
        if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
            return kaiwo.WorkloadStatusComplete, nil
        }
        if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
            return kaiwo.WorkloadStatusFailed, nil
        }
        if condition.Type == batchv1.JobSuspended && condition.Status == corev1.ConditionTrue {
            return kaiwo.WorkloadStatusPending, nil
        }
    }

    // Fallback on counters
    if job.Status.Succeeded > 0 {
        return kaiwo.WorkloadStatusComplete, nil
    }
    if job.Status.Failed > 0 {
        return kaiwo.WorkloadStatusFailed, nil
    }

    // Not complete/failed/suspended yet
    if baseutils.ValueOrDefault(job.Status.Ready) > 0 {
        return kaiwo.WorkloadStatusRunning, nil
    }
    if previousStatus == kaiwo.WorkloadStatusRunning {
        // Avoid flapping back to STARTING while terminating pods
        return previousStatus, nil
    }
    if job.Status.Active > 0 {
        return kaiwo.WorkloadStatusStarting, nil
    }
    // If we’ve already started once, don’t regress to PENDING
    if previousStatus == kaiwo.WorkloadStatusStarting {
        return previousStatus, nil
    }

    // No active pods and not yet marked complete -> pending
    return kaiwo.WorkloadStatusPending, nil
}
