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
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

type PreemptReason string

const (
	PreemptableConditionType                         string        = "Preemptable"
	PreemptReasonDurationNotExceeded                 PreemptReason = "DurationNotExceeded"
	PreemptReasonDurationExceeded                    PreemptReason = "DurationExceeded"
	PreemptReasonDurationExceededWithActiveGpuDemand PreemptReason = "DurationExceededWithActiveGpuDemand"
)

func GetRemainingTimeBeforeBecomingPreemptable(handler KaiwoWorkload) *time.Duration {
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
func GetPreemptableCondition(ctx context.Context, handler KaiwoWorkload) *v1.Condition {
	logger := log.FromContext(ctx)
	remaining := GetRemainingTimeBeforeBecomingPreemptable(handler)
	obj := handler.GetKaiwoWorkloadObject()
	if remaining == nil {
		return nil
	}
	logger.Info(fmt.Sprintf("Preemptable condition found, remaining: %v", *remaining))
	if remaining.Seconds() > 0 {
		return &v1.Condition{
			Type:               PreemptableConditionType,
			Status:             v1.ConditionFalse,
			Reason:             string(PreemptReasonDurationNotExceeded),
			Message:            "Workload is running and the duration has not been exceeded",
			ObservedGeneration: obj.GetGeneration(),
		}
	} else {
		return &v1.Condition{
			Type:               PreemptableConditionType,
			Status:             v1.ConditionTrue,
			Reason:             string(PreemptReasonDurationExceeded),
			Message:            "Workload duration has exceeded",
			ObservedGeneration: obj.GetGeneration(),
		}
	}
}

func CleanupExpiredWorkloads(ctx context.Context, k8sClient client.Client) (bool, error) {
	logger := log.FromContext(ctx).WithName("CleanupExpiredWorkloads")

	var workloads []KaiwoWorkload

	jobList := &v1alpha1.KaiwoJobList{}
	if err := k8sClient.List(ctx, jobList); err != nil {
		return false, fmt.Errorf("failed to list Kaiwo jobs: %w", err)
	}
	for _, job := range jobList.Items {
		workloads = append(workloads, &job)
	}

	serviceList := &v1alpha1.KaiwoServiceList{}
	if err := k8sClient.List(ctx, serviceList); err != nil {
		return false, fmt.Errorf("failed to list Kaiwo services: %w", err)
	}
	for _, service := range serviceList.Items {
		workloads = append(workloads, &service)
	}

	workloadsCleaned := false
	for _, workload := range workloads {
		status := workload.GetCommonStatusSpec()
		if status.Status == v1alpha1.WorkloadStatusRunning && meta.IsStatusConditionTrue(status.Conditions, PreemptableConditionType) {
			status.Status = v1alpha1.WorkloadStatusTerminating
			meta.SetStatusCondition(&status.Conditions, v1.Condition{
				Type:    WorkloadEarlyTerminationConditionType,
				Status:  v1.ConditionFalse,
				Reason:  string(PreemptReasonDurationExceeded),
				Message: "Workload is terminating due to exceeding its duration and active GPU demand",
			})
			if err := k8sClient.Status().Update(ctx, workload.GetKaiwoWorkloadObject()); err != nil {
				logger.Error(err, "failed to update Kaiwo workload status")
			} else {
				workloadsCleaned = true
			}
		}
	}

	return workloadsCleaned, nil
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

func ShouldRequeueAfter(workload KaiwoWorkload) (bool, *time.Duration) {
	condition := meta.FindStatusCondition(workload.GetCommonStatusSpec().Conditions, PreemptableConditionType)
	if condition != nil &&
		condition.Status == v1.ConditionFalse &&
		condition.ObservedGeneration == workload.GetKaiwoWorkloadObject().GetGeneration() {
		requeueAfter := GetRemainingTimeBeforeBecomingPreemptable(workload)
		if requeueAfter != nil {
			return true, requeueAfter
		}
	}
	return false, nil
}
