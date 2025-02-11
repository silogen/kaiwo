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

package controller

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
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

func CreateDefaultResourceFlavors(ctx context.Context, c client.Client) ([]kueuev1beta1.ResourceFlavor, map[string]kueuev1beta1.FlavorQuotas, error) {
	resourceFlavors := []kueuev1beta1.ResourceFlavor{}
	nodePoolResources := make(map[string]kueuev1beta1.FlavorQuotas) // Store computed quotas
	nodePools := make(map[string][]string)                          // Maps nodepool names to node names

	// Get node list dynamically
	nodeList := GetNodeResources(ctx, c)

	for _, node := range nodeList {
		// **Skip Control Plane Nodes**
		// TODO: make this configurable because control planes may be all that exists
		if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
			continue
		}
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
			continue
		}

		var flavorName string
		gpuType := "cpu-only"
		gpuCount := 0

		// Identify AMD GPU Nodes
		if gpuID, exists := node.Labels["amd.com/gpu.device-id"]; exists {
			gpuType = fmt.Sprintf("amd-%s", gpuID)
			if count, ok := node.Labels["beta.amd.com/gpu.family.AI"]; ok {
				gpuCount, _ = strconv.Atoi(count)
			}
		}

		// Identify NVIDIA GPU Nodes
		if gpuProduct, exists := node.Labels["nvidia.com/gpu.product"]; exists {
			gpuType = fmt.Sprintf("nvidia-%s", gpuProduct)
			if count, ok := node.Labels["nvidia.com/gpu.count"]; ok {
				gpuCount, _ = strconv.Atoi(count)
			}
		}

		// Compute nominal CPU and memory (90% of total)
		nominalCPU := resource.NewQuantity(int64(float64(node.CPU)*cpuMemoryDiscountFactor), resource.DecimalSI)
		nominalMemory := resource.NewQuantity(int64(float64(node.Memory)*cpuMemoryDiscountFactor*1024*1024*1024), resource.BinarySI)

		// Define a unique flavor name (GPU type included)
		flavorName = fmt.Sprintf("%s-%dcore-%dGi", gpuType, nominalCPU.Value(), nominalMemory.Value()/(1024*1024*1024))

		// Track node membership in the nodepool
		nodePools[flavorName] = append(nodePools[flavorName], node.Name)

		// Define ResourceQuota for this nodepool
		resourceQuotas := []kueuev1beta1.ResourceQuota{
			{
				Name:         corev1.ResourceCPU,
				NominalQuota: *nominalCPU,
			},
			{
				Name:         corev1.ResourceMemory,
				NominalQuota: *nominalMemory,
			},
		}

		// If the node has GPUs, add GPU quotas
		if gpuCount > 0 {
			gpuResource := corev1.ResourceName("amd.com/gpu")
			if strings.HasPrefix(gpuType, "nvidia") {
				gpuResource = corev1.ResourceName("nvidia.com/gpu")
			}
			resourceQuotas = append(resourceQuotas, kueuev1beta1.ResourceQuota{
				Name:         gpuResource,
				NominalQuota: *resource.NewQuantity(int64(gpuCount), resource.DecimalSI),
			})
		}

		// Store in nodePoolResources
		nodePoolResources[flavorName] = kueuev1beta1.FlavorQuotas{
			Name:      kueuev1beta1.ResourceFlavorReference(flavorName),
			Resources: resourceQuotas,
		}
	}

	// Create ResourceFlavors and label nodes
	for flavorName, nodeNames := range nodePools {
		resourceFlavors = append(resourceFlavors, kueuev1beta1.ResourceFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: flavorName,
			},
			Spec: kueuev1beta1.ResourceFlavorSpec{
				NodeLabels: map[string]string{
					"kaiwo/nodepool": flavorName,
				},
			},
		})

		// Label each node in the nodepool
		for _, nodeName := range nodeNames {
			err := LabelNode(ctx, c, nodeName, "kaiwo/nodepool", flavorName)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to label node %s: %w", nodeName, err)
			}
		}
	}

	return resourceFlavors, nodePoolResources, nil
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

func CreateClusterQueue(nodePoolResources map[string]kueuev1beta1.FlavorQuotas) kueuev1beta1.ClusterQueue {
	var resourceGroups []kueuev1beta1.ResourceGroup

	// Convert map to a sortable slice
	var flavorQuotas []kueuev1beta1.FlavorQuotas
	for _, quota := range nodePoolResources {
		flavorQuotas = append(flavorQuotas, quota)
	}

	// Sort flavors: CPU-only nodes first, then GPU nodes by GPU count, then CPU, then memory
	sort.Slice(flavorQuotas, func(i, j int) bool {
		// Extract resource values
		gpuCountI := getGPUCount(string(flavorQuotas[i].Name))
		gpuCountJ := getGPUCount(string(flavorQuotas[j].Name))
		cpuI := getCPUCount(string(flavorQuotas[i].Name))
		cpuJ := getCPUCount(string(flavorQuotas[j].Name))
		memoryI := getMemoryCount(string(flavorQuotas[i].Name))
		memoryJ := getMemoryCount(string(flavorQuotas[j].Name))

		// Sorting order:
		// 1. CPU-only nodes come first
		if gpuCountI == 0 && gpuCountJ > 0 {
			return true
		}
		if gpuCountJ == 0 && gpuCountI > 0 {
			return false
		}

		// 2. GPU nodes: sort by GPU count (lower count = cheaper)
		if gpuCountI != gpuCountJ {
			return gpuCountI < gpuCountJ
		}

		// 3. If same GPU count, sort by CPU (lower count = cheaper)
		if cpuI != cpuJ {
			return cpuI < cpuJ
		}

		// 4. If same CPU count, sort by memory (lower count = cheaper)
		return memoryI < memoryJ
	})

	// Define a resource group containing the sorted flavors
	resourceGroups = append(resourceGroups, kueuev1beta1.ResourceGroup{
		CoveredResources: []corev1.ResourceName{"cpu", "memory", "nvidia.com/gpu", "amd.com/gpu"},
		Flavors:          flavorQuotas,
	})

	// Create ClusterQueue
	return kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kaiwo-cluster-queue",
		},
		Spec: kueuev1beta1.ClusterQueueSpec{
			NamespaceSelector: &metav1.LabelSelector{},
			ResourceGroups:    resourceGroups,
		},
	}
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
