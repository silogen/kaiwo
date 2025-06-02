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
	"math"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SchedulingConfig struct {
	TotalGpus      int
	Replicas       int
	GpusPerReplica int
	GpuVendor      string

	DefaultResources *v1.ResourceRequirements
}

// CalculateSchedulingConfig calculates the workload scheduling config based on the requested GPUs and available cluster resources
func CalculateSchedulingConfig(
	ctx context.Context,
	clusterCtx ClusterContext,
	workload KaiwoWorkload,
	useAvailability bool,
) SchedulingConfig {
	spec := workload.GetCommonSpec()
	gpuStats := clusterCtx.GpuStats

	schedulingConfig := SchedulingConfig{
		GpuVendor: spec.GpuVendor,
	}
	if spec.Resources != nil {
		schedulingConfig.DefaultResources = spec.Resources.DeepCopy()
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
		schedulingConfig.Replicas = replicas

		return schedulingConfig
	}

	// If user has already provided the GPUs per replica and the replicas, use those
	if spec.Replicas != nil && *spec.Replicas > 0 && spec.GpusPerReplica > 0 && totalUserRequestedGpus <= gpuStats.TotalClusterGPUs {
		schedulingConfig.Replicas = *spec.Replicas
		schedulingConfig.TotalGpus = totalUserRequestedGpus
		schedulingConfig.GpusPerReplica = spec.GpusPerReplica

		return schedulingConfig
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

	schedulingConfig.TotalGpus = totalUserRequestedGpus
	schedulingConfig.GpusPerReplica = gpusPerReplica
	schedulingConfig.Replicas = replicas

	return schedulingConfig
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
	schedulingConfig := CalculateSchedulingConfig(ctx, clusterCtx, workload, true)

	spec := workload.GetCommonSpec()
	gpuStats := clusterCtx.GpuStats

	if schedulingConfig.TotalGpus > 0 && gpuStats.TotalClusterGPUs == 0 {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionFalse,
			Reason:  string(UnschedulableNoGPUs),
			Message: fmt.Sprintf("Cluster has 0 GPUs (vendor: %s)", spec.GpuVendor),
		}
	} else if schedulingConfig.TotalGpus > gpuStats.TotalClusterGPUs {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionFalse,
			Reason:  string(UnschedulableInsufficientGPUs),
			Message: fmt.Sprintf("Cluster has %d GPUs, but requested for %d GPUs (vendor: %s)", gpuStats.TotalClusterGPUs, schedulingConfig.TotalGpus, spec.GpuVendor),
		}
	} else if schedulingConfig.TotalGpus > 0 {
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
