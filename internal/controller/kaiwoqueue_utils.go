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

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
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

func CreateDefaultResourceFlavors(ctx context.Context, c client.Client) ([]kaiwov1alpha1.ResourceFlavorSpec, map[string]kueuev1beta1.FlavorQuotas, error) {
	var resourceFlavors []kaiwov1alpha1.ResourceFlavorSpec
	nodePoolResources := make(map[string]kueuev1beta1.FlavorQuotas)
	nodePools := make(map[string][]string)

	resourceAggregates := make(map[string]map[corev1.ResourceName]*resource.Quantity)

	// Get node list dynamically
	nodeList := GetNodeResources(ctx, c)

	for _, node := range nodeList {
		// **Skip Control Plane Nodes**
		if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
			continue
		}
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
			continue
		}

		gpuType := "cpu-only"
		gpuCount := 0

		// Identify AMD GPU Nodes
		if gpuID, exists := node.Labels["amd.com/gpu.device-id"]; exists {
			gpuType = MapGPUDeviceIDToName(gpuID, "amd")
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

		flavorName := fmt.Sprintf("%s-%dgpu-%dcore-%dgi", gpuType, gpuCount, nominalCPU.Value(), nominalMemory.Value()/(1024*1024*1024))

		// Track node membership in the nodepool
		nodePools[flavorName] = append(nodePools[flavorName], node.Name)

		if _, exists := resourceAggregates[flavorName]; !exists {
			resourceAggregates[flavorName] = map[corev1.ResourceName]*resource.Quantity{
				corev1.ResourceCPU:    resource.NewQuantity(0, resource.DecimalSI),
				corev1.ResourceMemory: resource.NewQuantity(0, resource.BinarySI),
			}
			if gpuCount > 0 {
				gpuResource := corev1.ResourceName("amd.com/gpu")
				if strings.HasPrefix(gpuType, "nvidia") {
					gpuResource = corev1.ResourceName("nvidia.com/gpu")
				}
				resourceAggregates[flavorName][gpuResource] = resource.NewQuantity(0, resource.DecimalSI)
			}
		}

		resourceAggregates[flavorName][corev1.ResourceCPU].Add(*nominalCPU)
		resourceAggregates[flavorName][corev1.ResourceMemory].Add(*nominalMemory)

		if gpuCount > 0 {
			gpuResource := corev1.ResourceName("amd.com/gpu")
			if strings.HasPrefix(gpuType, "nvidia") {
				gpuResource = corev1.ResourceName("nvidia.com/gpu")
			}
			resourceAggregates[flavorName][gpuResource].Add(*resource.NewQuantity(int64(gpuCount), resource.DecimalSI))
		}
	}

	for flavorName := range nodePools {
		resourceQuotas := []kueuev1beta1.ResourceQuota{
			{
				Name:         corev1.ResourceCPU,
				NominalQuota: *resourceAggregates[flavorName][corev1.ResourceCPU],
			},
			{
				Name:         corev1.ResourceMemory,
				NominalQuota: *resourceAggregates[flavorName][corev1.ResourceMemory],
			},
		}

		for resourceName, quota := range resourceAggregates[flavorName] {
			if resourceName == corev1.ResourceCPU || resourceName == corev1.ResourceMemory {
				continue
			}
			resourceQuotas = append(resourceQuotas, kueuev1beta1.ResourceQuota{
				Name:         resourceName,
				NominalQuota: *quota,
			})
		}

		// Store in nodePoolResources
		nodePoolResources[flavorName] = kueuev1beta1.FlavorQuotas{
			Name:      kueuev1beta1.ResourceFlavorReference(flavorName),
			Resources: resourceQuotas,
		}

		// Define ResourceFlavor
		resourceFlavors = append(resourceFlavors, kaiwov1alpha1.ResourceFlavorSpec{
			Name: flavorName,
			NodeLabels: map[string]string{
				"kaiwo/nodepool": flavorName,
			},
		})
	}

	resourceFlavors = RemoveDuplicateResourceFlavors(resourceFlavors)

	for flavorName, nodeNames := range nodePools {
		for _, nodeName := range nodeNames {
			err := LabelNode(ctx, c, nodeName, "kaiwo/nodepool", flavorName)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to label node %s: %w", nodeName, err)
			}
		}
	}

	return resourceFlavors, nodePoolResources, nil
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

func RemoveDuplicateResourceFlavors(flavors []kaiwov1alpha1.ResourceFlavorSpec) []kaiwov1alpha1.ResourceFlavorSpec {
	uniqueMap := make(map[string]kaiwov1alpha1.ResourceFlavorSpec)
	for _, flavor := range flavors {
		uniqueMap[flavor.Name] = flavor
	}

	uniqueFlavors := make([]kaiwov1alpha1.ResourceFlavorSpec, 0, len(uniqueMap))
	for _, flavor := range uniqueMap {
		uniqueFlavors = append(uniqueFlavors, flavor)
	}
	return uniqueFlavors
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

func CreateClusterQueue(nodePoolResources map[string]kueuev1beta1.FlavorQuotas, name string) kaiwov1alpha1.ClusterQueue {
	var resourceGroups []kueuev1beta1.ResourceGroup
	coveredResources := make(map[corev1.ResourceName]struct{})

	// Convert map to slice
	var flavorQuotas []kueuev1beta1.FlavorQuotas
	for _, quota := range nodePoolResources {
		flavorQuotas = append(flavorQuotas, quota)

		// Extract resources dynamically
		for _, quotaResource := range quota.Resources {
			resourceName := quotaResource.Name
			normalizedResourceName := corev1.ResourceName(strings.TrimSpace(string(resourceName)))
			coveredResources[normalizedResourceName] = struct{}{}
		}
	}

	// Sort flavors: CPU-only nodes first, then GPU nodes by GPU count, then CPU, then memory
	sort.Slice(flavorQuotas, func(i, j int) bool {
		gpuCountI := getGPUCount(string(flavorQuotas[i].Name))
		gpuCountJ := getGPUCount(string(flavorQuotas[j].Name))
		cpuI := getCPUCount(string(flavorQuotas[i].Name))
		cpuJ := getCPUCount(string(flavorQuotas[j].Name))
		memoryI := getMemoryCount(string(flavorQuotas[i].Name))
		memoryJ := getMemoryCount(string(flavorQuotas[j].Name))

		if gpuCountI == 0 && gpuCountJ > 0 {
			return true
		}
		if gpuCountJ == 0 && gpuCountI > 0 {
			return false
		}
		if gpuCountI != gpuCountJ {
			return gpuCountI < gpuCountJ
		}
		if cpuI != cpuJ {
			return cpuI < cpuJ
		}
		return memoryI < memoryJ
	})

	// Convert collected resources to a slice for `CoveredResources`
	var coveredResourcesSlice []corev1.ResourceName
	for resource := range coveredResources {
		coveredResourcesSlice = append(coveredResourcesSlice, resource)
	}

	// Define the resource group dynamically based on collected resources
	resourceGroups = append(resourceGroups, kueuev1beta1.ResourceGroup{
		CoveredResources: coveredResourcesSlice,
		Flavors:          flavorQuotas,
	})

	// Create ClusterQueue
	return kaiwov1alpha1.ClusterQueue{
		Name: name,
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

func ConvertKaiwoToKueueResourceFlavors(kaiwoFlavors []kaiwov1alpha1.ResourceFlavorSpec) []kueuev1beta1.ResourceFlavor {
	var kueueFlavors []kueuev1beta1.ResourceFlavor
	for _, rf := range kaiwoFlavors {
		kueueFlavors = append(kueueFlavors, ConvertKaiwoToKueueResourceFlavor(rf))
	}
	return kueueFlavors
}

func ConvertKaiwoToKueueResourceFlavor(kaiwoFlavor kaiwov1alpha1.ResourceFlavorSpec) kueuev1beta1.ResourceFlavor {
	return kueuev1beta1.ResourceFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: kaiwoFlavor.Name},
		Spec: kueuev1beta1.ResourceFlavorSpec{
			NodeLabels: kaiwoFlavor.NodeLabels,
			// Copy other fields if needed
		},
	}
}

func ConvertKaiwoToKueueClusterQueue(kaiwoQueue kaiwov1alpha1.ClusterQueue) kueuev1beta1.ClusterQueue {
	return kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name: kaiwoQueue.Name,
		},
		Spec: kaiwoQueue.Spec,
	}
}
