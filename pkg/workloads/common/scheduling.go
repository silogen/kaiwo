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
	"math"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceConfig struct {
	TotalGpus      int
	Replicas       int
	GpusPerReplica int
	GpuVendor      string

	DefaultResources *v1.ResourceRequirements
}

// CalculateResourceConfig calculates the workload scheduling config based on the requested GPUs and available cluster resources
func CalculateResourceConfig(
	ctx context.Context,
	clusterCtx ClusterContext,
	workload KaiwoWorkload,
	useAvailability bool,
) ResourceConfig {
	spec := workload.GetCommonSpec()
	gpuStats := clusterCtx.GpuStats

	resourceConfig := ResourceConfig{
		GpuVendor: spec.GpuVendor,
	}
	if spec.Resources != nil {
		resourceConfig.DefaultResources = spec.Resources.DeepCopy()
	}

	userRequestedGpus := 0
	if spec.Replicas != nil {
		userRequestedGpus = *spec.Replicas * spec.GpusPerReplica
	}
	totalUserRequestedGpus := int(math.Max(float64(userRequestedGpus), float64(spec.Gpus)))

	if totalUserRequestedGpus == 0 {
		replicas := 1
		if spec.Replicas != nil {
			replicas = *spec.Replicas
		}
		resourceConfig.Replicas = replicas

		return resourceConfig
	}

	// If user has already provided the GPUs per replica and the replicas, use those
	if spec.Replicas != nil && *spec.Replicas > 0 && spec.GpusPerReplica > 0 && totalUserRequestedGpus <= gpuStats.TotalClusterGPUs {
		resourceConfig.Replicas = *spec.Replicas
		resourceConfig.TotalGpus = totalUserRequestedGpus
		resourceConfig.GpusPerReplica = spec.GpusPerReplica

		return resourceConfig
	}

	minGpusPerNode := gpuStats.MinGPUsPerNode

	if useAvailability {
		if gpuStats.TotalAllocatableGPUs >= totalUserRequestedGpus {
			minGpusPerNode = gpuStats.MinAllocatableGPUsPerNode
		}
	}

	//if totalUserRequestedGpus > gpuStats.TotalClusterGPUs {
	//	klog.Warningf("Requested GPUs exceed total GPUs in the cluster. "+
	//		"GPU request will be reduced to match maximum available GPU capacity. "+
	//		"Requested GPUs: %d, Total GPUs in Cluster: %d",
	//		totalUserRequestedGpus, gpuStats.TotalClusterGPUs,
	//	)
	//	// Adjust totalGpus to the maximum available
	//	totalUserRequestedGpus = gpuStats.TotalClusterGPUs
	//}

	replicas := (totalUserRequestedGpus + minGpusPerNode - 1) / minGpusPerNode // Round up
	gpusPerReplica := totalUserRequestedGpus / replicas

	resourceConfig.TotalGpus = totalUserRequestedGpus
	resourceConfig.GpusPerReplica = gpusPerReplica
	resourceConfig.Replicas = replicas

	return resourceConfig
}

type SchedulingReason string

const (
	SchedulableType = "Schedulable"

	Schedulable SchedulingReason = "Schedulable"

	UnschedulableNoGPUs           SchedulingReason = "NoGPUs"
	UnschedulableInsufficientGPUs SchedulingReason = "InsufficientGPUs"

	UnschedulableWrongQueueNamespace  SchedulingReason = "WrongQueueNamespace"
	UnschedulableClusterQueueNotFound SchedulingReason = "ClusterQueueNotFound"
)

func GetSchedulableCondition(ctx context.Context, clusterCtx ClusterContext, workload KaiwoWorkload) metav1.Condition {
	queueConfigCondition := getQueueConfigCondition(ctx, clusterCtx, workload)

	if queueConfigCondition.Status == metav1.ConditionFalse {
		return queueConfigCondition
	}
	gpuSchedulableCondition := getGpuSchedulableCondition(ctx, clusterCtx, workload)

	// TODO add another reason that indicates the workload can be scheduled, but not immediately?
	// TODO add checks for other cluster resources?

	return gpuSchedulableCondition
}

func getQueueConfigCondition(ctx context.Context, clusterCtx ClusterContext, workload KaiwoWorkload) metav1.Condition {
	kaiwoQueueConfig := clusterCtx.KaiwoQueueConfig
	clusterQueueName := GetClusterQueueName(ctx, workload)
	workloadNamespace := workload.GetKaiwoWorkloadObject().GetNamespace()
	for _, clusterQueue := range kaiwoQueueConfig.Spec.ClusterQueues {
		if clusterQueue.Name == clusterQueueName {
			if len(clusterQueue.Namespaces) == 0 {
				return metav1.Condition{
					Type:    SchedulableType,
					Status:  metav1.ConditionTrue,
					Reason:  string(Schedulable),
					Message: "Cluster queue has no namespace restrictions",
				}
			}
			for _, namespace := range clusterQueue.Namespaces {
				if namespace == workloadNamespace {
					return metav1.Condition{
						Type:    SchedulableType,
						Status:  metav1.ConditionTrue,
						Reason:  string(Schedulable),
						Message: "Cluster queue namespace matches",
					}
				}
			}
			return metav1.Condition{
				Type:    SchedulableType,
				Status:  metav1.ConditionFalse,
				Reason:  string(UnschedulableWrongQueueNamespace),
				Message: fmt.Sprintf("Cluster queue '%s' defines one or more namespaces, but workload namespace '%s' is not one of them", clusterQueueName, workloadNamespace),
			}
		}
	}
	return metav1.Condition{
		Type:    SchedulableType,
		Status:  metav1.ConditionFalse,
		Reason:  string(UnschedulableClusterQueueNotFound),
		Message: fmt.Sprintf("Cluster queue '%s' is not defined in KaiwoQueueConfig '%s'", clusterQueueName, kaiwoQueueConfig.Name),
	}
}

func getGpuSchedulableCondition(ctx context.Context, clusterCtx ClusterContext, workload KaiwoWorkload) metav1.Condition {
	resourceConfig := CalculateResourceConfig(ctx, clusterCtx, workload, true)

	spec := workload.GetCommonSpec()
	gpuStats := clusterCtx.GpuStats

	if resourceConfig.TotalGpus > 0 && gpuStats.TotalClusterGPUs == 0 {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionFalse,
			Reason:  string(UnschedulableNoGPUs),
			Message: fmt.Sprintf("Cluster has 0 GPUs (vendor: %s)", spec.GpuVendor),
		}
	} else if resourceConfig.TotalGpus > gpuStats.TotalClusterGPUs {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionFalse,
			Reason:  string(UnschedulableInsufficientGPUs),
			Message: fmt.Sprintf("Cluster has %d GPUs, but requested for %d GPUs (vendor: %s)", gpuStats.TotalClusterGPUs, resourceConfig.TotalGpus, spec.GpuVendor),
		}
	} else if resourceConfig.TotalGpus > 0 {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionTrue,
			Reason:  string(Schedulable),
			Message: "Cluster has sufficient GPU resources",
		}
	}
	return metav1.Condition{
		Type:    SchedulableType,
		Status:  metav1.ConditionTrue,
		Reason:  string(Schedulable),
		Message: "Workload does not require GPU resources",
	}
}
