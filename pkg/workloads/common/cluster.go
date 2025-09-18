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
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterContext provides context of the cluster and its resources to help build downstream objects
type ClusterContext struct {
	Nodes    []v1.Node
	GpuStats GPUStats
}

type GPUStats struct {
	// TotalClusterGPUs is the total number of GPUs (regardless of current allocation) across all matching nodes.
	TotalClusterGPUs int

	// TotalAllocatableGPUs is the sum of currently allocatable GPUs across all matching nodes. Only populated when useAvailability=true.
	TotalAllocatableGPUs int

	// MinAllocatableGPUsPerNode is the smallest number of allocatable GPUs observed on any single matching node. Only meaningful if useAvailability=true.
	MinAllocatableGPUsPerNode int

	// MinGPUsPerNode is the smallest total GPU count (ignoring allocation) observed on any single matching node.
	MinGPUsPerNode int

	// NodeGPUMap maps each node’s name to its GPU count. If useAvailability=false this is the total GPUs per node, otherwise it is the allocatable GPUs per node.
	NodeGPUMap map[string]int
}

// GetClusterContext gathers the cluster context that is relevant for a particular workload
func GetClusterContext(ctx context.Context, k8sClient client.Client, workload KaiwoWorkload) (*ClusterContext, error) {
	nodeList := &v1.NodeList{}
	if err := k8sClient.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	clusterCtx := ClusterContext{
		Nodes: nodeList.Items,
	}

	clusterCtx.GpuStats = fillGpuStats(ctx, clusterCtx, workload.GetCommonSpec().GpuVendor, true)

	return &clusterCtx, nil
}

// GetGPUStats scans all nodes and returns a summary of GPU counts.
// If useAvailability is false, TotalAllocatableGPUs and MinAllocatableGPUsPerNode
// will be left at zero.
func fillGpuStats(ctx context.Context, clusterCtx ClusterContext, gpuVendor string, useAvailability bool) GPUStats {
	logger := log.FromContext(ctx)

	stats := GPUStats{
		MinGPUsPerNode:            math.MaxInt32,
		MinAllocatableGPUsPerNode: math.MaxInt32,
		NodeGPUMap:                make(map[string]int),
	}

	for _, node := range clusterCtx.Nodes {
		label, ok := node.Labels[DefaultNodePoolLabel]
		if !ok {
			continue
		}

		parts := strings.Split(label, "-")
		if len(parts) < 3 || parts[0] != gpuVendor {
			continue
		}

		countStr := strings.TrimSuffix(parts[2], "gpu")
		gpusPerNode, err := strconv.Atoi(countStr)
		if err != nil {
			logger.Error(err, "parsing GPU count", "node", node.Name, "label", parts[2])
			continue
		}

		stats.TotalClusterGPUs += gpusPerNode

		// always track raw minimum/total
		if gpusPerNode < stats.MinGPUsPerNode {
			stats.MinGPUsPerNode = gpusPerNode
		}

		stats.NodeGPUMap[node.Name] = gpusPerNode

		if useAvailability {
			rName := v1.ResourceName(fmt.Sprintf("%s.com/gpu", gpuVendor))
			if alloc, ok := node.Status.Allocatable[rName]; ok {
				allocG := int(alloc.Value())
				if allocG == 0 {
					continue
				}
				stats.TotalAllocatableGPUs += allocG
				if allocG < stats.MinAllocatableGPUsPerNode {
					stats.MinAllocatableGPUsPerNode = allocG
				}
			}
		}
	}

	return stats
}
