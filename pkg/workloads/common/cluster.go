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
	"strconv"
	"strings"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterContext provides context of the cluster and its resources to help build downstream objects
type ClusterContext struct {
	Nodes            []v1.Node
	GpuStats         GPUStats
	KaiwoQueueConfig v1alpha1.KaiwoQueueConfig
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

	kaiwoQueueConfig := &v1alpha1.KaiwoQueueConfig{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: KaiwoQueueConfigName}, kaiwoQueueConfig); err != nil {
		return nil, fmt.Errorf("failed to get KaiwoQueueConfig: %w", err)
	}
	clusterCtx.KaiwoQueueConfig = *kaiwoQueueConfig

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
