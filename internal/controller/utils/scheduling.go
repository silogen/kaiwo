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

package controllerutils

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	klog "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	cpuMemoryDiscountFactor = 0.9
)

type NodeResourceInfo struct {
	Name   string
	CPU    int
	Memory int
	Labels map[string]string
}

func GetNodeResources(ctx context.Context, c client.Client) []NodeResourceInfo {
	var nodeList corev1.NodeList
	err := c.List(ctx, &nodeList)
	if err != nil {
		return []NodeResourceInfo{}
	}

	var nodes []NodeResourceInfo
	for _, node := range nodeList.Items {
		cpu := node.Status.Capacity.Cpu().Value()
		memory := node.Status.Capacity.Memory().Value() / (1024 * 1024 * 1024) // Convert to Gi

		nodes = append(nodes, NodeResourceInfo{
			Name:   node.Name,
			CPU:    int(cpu),
			Memory: int(memory),
			Labels: node.Labels,
		})
	}

	return nodes
}

func MapGPUDeviceIDToName(gpuID string, vendor string) string {
	knownGPUs := map[string]string{
		"740c": "mi250",
		"74a1": "mi300",
	}

	if vendor == "amd" {
		if name, exists := knownGPUs[gpuID]; exists {
			return name
		}
		return fmt.Sprintf("amd-%s", gpuID)
	}
	return gpuID
}

func LabelNode(ctx context.Context, c client.Client, nodeName, key, value string) error {
	var node corev1.Node
	err := c.Get(ctx, client.ObjectKey{Name: nodeName}, &node)
	if err != nil {
		return err
	}

	// Add or update the label
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels[key] = value

	return c.Update(ctx, &node)
}

func getGPUCount(flavorName string) int {
	if strings.Contains(flavorName, "nvidia") || strings.Contains(flavorName, "amd") {
		parts := strings.Split(flavorName, "-")
		for _, p := range parts {
			if strings.HasSuffix(p, "gpu") {
				count, _ := strconv.Atoi(strings.TrimSuffix(p, "gpu"))
				return count
			}
		}
	}
	return 0
}

func getCPUCount(flavorName string) int {
	parts := strings.Split(flavorName, "-")
	for _, p := range parts {
		if strings.HasSuffix(p, "core") {
			count, _ := strconv.Atoi(strings.TrimSuffix(p, "core"))
			return count
		}
	}
	return 0
}

func getMemoryCount(flavorName string) int {
	parts := strings.Split(flavorName, "-")
	for _, p := range parts {
		if strings.HasSuffix(p, "Gi") {
			count, _ := strconv.Atoi(strings.TrimSuffix(p, "Gi"))
			return count
		}
	}
	return 0
}

// CalculateNumberOfReplicas determines the number of replicas and GPUs per replica
// based on node labels and optionally available GPU capacity.
func CalculateNumberOfReplicas(ctx context.Context, k8sClient client.Client, gpuVendor string, totalGpus int, userReplicas int, userGpusPerReplica int, useAvailability bool) (int, int, int, error) {
	logger := log.FromContext(ctx)

	// Fetch all nodes
	var nodeList corev1.NodeList
	if err := k8sClient.List(ctx, &nodeList); err != nil {
		logger.Error(err, "Failed to list Kubernetes nodes")
		return 0, 0, 0, err
	}

	minGpusPerNode := 64
	totalAvailableGpus := 0
	totalClusterGpus := 0
	nodeGpuMap := make(map[string]int) // Map of node name -> available GPUs

	for _, node := range nodeList.Items {

		// Extract GPU info from "kaiwo/nodepool" label
		nodepoolLabel, exists := node.Labels["kaiwo/nodepool"]
		if !exists {
			continue
		}

		// Example format: amd-mi300-8gpu-201core-1813gi
		labelParts := strings.Split(nodepoolLabel, "-")
		if len(labelParts) < 3 {
			logger.Info("Skipping malformed nodepool label", "Node", node.Name, "Label", nodepoolLabel)
			continue
		}

		// Validate GPU vendor match
		nodeGpuVendor := labelParts[0]
		if nodeGpuVendor != gpuVendor {
			continue
		}

		// Extract number of GPUs
		gpuInfo := labelParts[2] // Example: "8gpu"
		gpuCountStr := strings.TrimSuffix(gpuInfo, "gpu")
		gpusPerNode, err := strconv.Atoi(gpuCountStr)
		if err != nil {
			logger.Error(err, "Failed to parse GPU count from node label", "Node", node.Name, "Label", gpuInfo)
			continue
		}

		totalClusterGpus += gpusPerNode

		// Determine GPU availability
		availableGpus := gpusPerNode
		if useAvailability {
			allocatable, ok := node.Status.Allocatable[corev1.ResourceName(fmt.Sprintf("%s.com/gpu", gpuVendor))]
			if ok {
				availableGpus = int(allocatable.Value())
			}
		}

		// Track minimum GPUs per node
		if availableGpus > 0 && availableGpus < minGpusPerNode {
			minGpusPerNode = availableGpus
		}

		// Track total available GPUs
		totalAvailableGpus += availableGpus
		nodeGpuMap[node.Name] = availableGpus
	}

	userRequestedGpus := userReplicas * userGpusPerReplica

	// If user has already set these values, use those
	if userReplicas > 0 && userGpusPerReplica > 0 && userRequestedGpus <= totalClusterGpus {
		// logger.Info("User-defined replicas and GPUs per replica provided", "Replicas", userReplicas, "GPUs per Replica", userGpusPerReplica)
		return totalGpus, userReplicas, userGpusPerReplica, nil
	}

	if userRequestedGpus > totalClusterGpus {
		totalGpus = userRequestedGpus
	}

	if totalGpus > totalClusterGpus {
		klog.Warningf("Requested GPUs exceed total GPUs in the cluster. "+
			"GPU request will be reduced to match maximum available GPU capacity. "+
			"Requested GPUs: %d, Total GPUs in Cluster: %d",
			totalGpus, totalClusterGpus,
		)
		// Adjust totalGpus to the maximum available
		totalGpus = totalClusterGpus
	}

	replicas := (totalGpus + minGpusPerNode - 1) / minGpusPerNode // Round up
	gpusPerReplica := totalGpus / replicas

	// logger.Info("Calculated replicas and GPUs per replica", "Replicas", replicas, "GPUs per Replica", gpusPerReplica)
	return totalGpus, replicas, gpusPerReplica, nil
}
