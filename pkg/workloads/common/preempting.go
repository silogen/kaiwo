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
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

type PreemptReason string

const (
	PreemptableConditionType                         string        = "Preemptable"
	PreemptReasonDurationNotExceeded                 PreemptReason = "DurationNotExceeded"
	PreemptReasonDurationExceeded                    PreemptReason = "DurationExceeded"
	PreemptReasonDurationExceededWithActiveGpuDemand PreemptReason = "DurationExceededWithActiveGpuDemand"
)

func GetRemainingTimeBeforeBecomingPreemptable(handler WorkloadReconciler) *time.Duration {
	status := handler.GetCommonStatusSpec()
	spec := handler.GetCommonSpec()
	if status.Status != v1alpha1.WorkloadStatusRunning || spec.Duration == nil || status.StartTime == nil {
		return nil
	}
	elapsed := time.Since(status.StartTime.Time)
	remaining := spec.Duration.Duration - elapsed
	return &remaining
}

// GetPreemptableCondition gets the correct preemptable condition
func GetPreemptableCondition(handler WorkloadReconciler) *v1.Condition {
	remaining := GetRemainingTimeBeforeBecomingPreemptable(handler)
	if remaining == nil {
		return nil
	}
	if remaining.Seconds() > 0 {
		return &v1.Condition{
			Type:    PreemptableConditionType,
			Status:  v1.ConditionFalse,
			Reason:  string(PreemptReasonDurationNotExceeded),
			Message: "Workload is running and the duration has not been exceeded",
		}
	} else {
		return &v1.Condition{
			Type:    PreemptableConditionType,
			Status:  v1.ConditionTrue,
			Reason:  string(PreemptReasonDurationExceeded),
			Message: "Workload duration has exceeded",
		}
	}
}

// ShouldPreempt checks if a workload should be preempted or not based on expired duration and current queue GPU demand
func ShouldPreempt(ctx context.Context, k8sClient client.Client, handler WorkloadReconciler) (bool, error) {
	status := handler.GetCommonStatusSpec()
	spec := handler.GetCommonSpec()
	duration := spec.Duration
	startTime := status.StartTime
	if duration == nil || status.Status != v1alpha1.WorkloadStatusRunning || status.StartTime == nil {
		return false, nil
	}
	now := time.Now()
	deadline := startTime.Add(duration.Duration)
	if now.After(deadline) {
		config := ConfigFromContext(ctx)
		queue := GetClusterQueueName(ctx, handler)
		hasDemand, err := ClusterHasGpuDemand(ctx, k8sClient, queue, spec.GpuVendor, config)
		if err != nil {
			return false, fmt.Errorf("failed to check if cluster has GPU demand: %w", err)
		}
		return hasDemand, nil
	}
	return false, nil
}

func ClusterHasGpuDemand(ctx context.Context, k8sClient client.Client, clusterQueue string, gpuVendor string, config KaiwoConfigContext) (bool, error) {
	var jobs v1alpha1.KaiwoJobList
	if err := k8sClient.List(ctx, &jobs); err != nil {
		return false, fmt.Errorf("failed to list KaiwoJobs: %w", err)
	}
	for _, job := range jobs.Items {
		if job.Spec.ClusterQueue == "" {
			job.Spec.ClusterQueue = config.DefaultClusterQueueName
		}
		if job.Status.Status == v1alpha1.WorkloadStatusPending &&
			job.Spec.Gpus > 0 &&
			job.Spec.ClusterQueue == clusterQueue &&
			job.Spec.GpuVendor == gpuVendor &&
			isPendingForLong(ctx, job.ObjectMeta) {
			return true, nil
		}
	}

	var services v1alpha1.KaiwoServiceList
	if err := k8sClient.List(ctx, &services); err != nil {
		return false, fmt.Errorf("failed to list KaiwoServices: %w", err)
	}
	for _, svc := range services.Items {
		if svc.Spec.ClusterQueue == "" {
			svc.Spec.ClusterQueue = config.DefaultClusterQueueName
		}
		if svc.Status.Status == v1alpha1.WorkloadStatusPending &&
			svc.Spec.Gpus > 0 &&
			svc.Spec.ClusterQueue == clusterQueue &&
			svc.Spec.GpuVendor == gpuVendor &&
			isPendingForLong(ctx, svc.ObjectMeta) {
			return true, nil
		}
	}

	return false, nil
}

func isPendingForLong(ctx context.Context, meta v1.ObjectMeta) bool {
	logger := log.FromContext(ctx)
	config := ConfigFromContext(ctx)
	age := time.Since(meta.CreationTimestamp.Time)
	duration, err := time.ParseDuration(config.Scheduling.PendingThresholdForPreemption)
	if err != nil {
		logger.Error(err, "Failed to parse duration", "duration", config.Scheduling.PendingThresholdForPreemption)
		return false
	}
	return age > duration
}
