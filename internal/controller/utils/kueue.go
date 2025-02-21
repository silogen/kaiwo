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
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const DefaultKaiwoQueueConfigName = "kaiwo"

var excludeMasterNodes = baseutils.GetEnv("EXCLUDE_MASTER_NODES_FROM_NODE_POOLS", "false")

func CreateDefaultResourceFlavors(ctx context.Context, c client.Client) ([]kaiwov1alpha1.ResourceFlavorSpec, map[string]kueuev1beta1.FlavorQuotas, error) {
	logger := log.FromContext(ctx)

	var resourceFlavors []kaiwov1alpha1.ResourceFlavorSpec
	nodePoolResources := make(map[string]kueuev1beta1.FlavorQuotas)
	nodePools := make(map[string][]string)

	resourceAggregates := make(map[string]map[corev1.ResourceName]*resource.Quantity)

	// Get node list dynamically
	nodeList := GetNodeResources(ctx, c)

	if strings.ToLower(excludeMasterNodes) == "true" {
		logger.Info("Excluding master/control-plane nodes from nodepools. Set EXCLUDE_MASTER_NODES_FROM_NODE_POOLS=false to include them")
	} else {
		logger.Info("Including master/control-plane nodes in nodepools. Set EXCLUDE_MASTER_NODES_FROM_NODE_POOLS=true to exclude them")
	}

	for _, node := range nodeList {
		// **Skip Control Plane Nodes**
		if strings.ToLower(excludeMasterNodes) == "true" {
			logger.Info("Excluding master/control-plane nodes from nodepools. Set EXCLUDE_MASTER_NODES_FROM_NODE_POOLS=false to include them")
			if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
				continue
			}
			if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
				continue
			}
		} else {
			logger.Info("Including master/control-plane nodes in nodepools. Set EXCLUDE_MASTER_NODES_FROM_NODE_POOLS=true to exclude them")
		}
		gpuCount := 0
		gpuVendor := "cpu"
		gpuType := "only"

		// Identify AMD GPU Nodes
		if gpuID, exists := node.Labels["amd.com/gpu.device-id"]; exists {
			gpuType = MapGPUDeviceIDToName(gpuID, "amd")
			if count, ok := node.Labels["beta.amd.com/gpu.family.AI"]; ok {
				gpuCount, _ = strconv.Atoi(count)
				gpuVendor = "amd"
			}
		}

		// Identify NVIDIA GPU Nodes
		if gpuProduct, exists := node.Labels["nvidia.com/gpu.product"]; exists {
			gpuType = fmt.Sprintf("nvidia-%s", gpuProduct)
			if count, ok := node.Labels["nvidia.com/gpu.count"]; ok {
				gpuCount, _ = strconv.Atoi(count)
				gpuVendor = "nvidia"
			}
		}

		// Compute nominal CPU and memory (90% of total)
		nominalCPU := resource.NewQuantity(int64(float64(node.CPU)*cpuMemoryDiscountFactor), resource.DecimalSI)
		nominalMemory := resource.NewQuantity(int64(float64(node.Memory)*cpuMemoryDiscountFactor*1024*1024*1024), resource.BinarySI)

		flavorName := fmt.Sprintf("%s-%s-%dgpu-%dcore-%dgi", gpuVendor, gpuType, gpuCount, nominalCPU.Value(), nominalMemory.Value()/(1024*1024*1024))

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
	logger.Info("Final generated node pool resources", "nodePoolResources", nodePoolResources)
	logger.Info("Final generated resource flavors", "resourceFlavors", resourceFlavors)

	return resourceFlavors, nodePoolResources, nil
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

	// Ensure every flavor has the same set of resources as coveredResources
	for i, flavor := range flavorQuotas {
		missingResources := []corev1.ResourceName{}

		for _, covered := range coveredResourcesSlice {
			found := false
			for _, quota := range flavor.Resources {
				if quota.Name == covered {
					found = true
					break
				}
			}
			if !found {
				missingResources = append(missingResources, covered)
			}
		}

		// If a resource is missing from a flavor, add it with zero quota
		for _, missing := range missingResources {
			flavorQuotas[i].Resources = append(flavorQuotas[i].Resources, kueuev1beta1.ResourceQuota{
				Name:         missing,
				NominalQuota: *resource.NewQuantity(0, resource.DecimalSI), // Set to zero
			})
		}
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

// CreateLocalQueue creates a LocalQueue in the given namespace.
func CreateLocalQueue(ctx context.Context, c client.Client, name string, namespace string) error {
	logger := log.FromContext(ctx)

	// Define the LocalQueue object
	localQueue := &kueuev1beta1.LocalQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kueuev1beta1.LocalQueueSpec{
			ClusterQueue: kueuev1beta1.ClusterQueueReference(name),
		},
	}

	// Check if the LocalQueue already exists
	existingQueue := &kueuev1beta1.LocalQueue{}
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, existingQueue)
	if err == nil {
		return nil
	}

	// Create the LocalQueue
	err = c.Create(ctx, localQueue)
	if err != nil {
		logger.Error(err, "Failed to create LocalQueue", "Name", name, "Namespace", namespace)
		return fmt.Errorf("failed to create LocalQueue %s in namespace %s: %w", name, namespace, err)
	}

	logger.Info("Successfully created LocalQueue", "Name", name, "Namespace", namespace)
	return nil
}
