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
	"reflect"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

func EnsureNamespaceKueueManaged(ctx context.Context, k8sClient client.Client, namespaceName string) error {
	logger := log.FromContext(ctx)

	namespace := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace); err != nil {
		logger.Error(err, "Failed to get namespace", "namespace", namespaceName)
		return fmt.Errorf("failed to get namespace: %w", err)
	}

	if value, exists := namespace.Labels["kueue-managed"]; exists && value == "true" { //nolint:goconst
		return nil
	}

	if namespace.Labels == nil {
		namespace.Labels = make(map[string]string)
	}
	namespace.Labels["kueue-managed"] = "true" //nolint:goconst

	logger.Info("Adding 'kueue-managed: true' label to namespace", "namespace", namespaceName)

	if err := k8sClient.Update(ctx, namespace); err != nil {
		logger.Error(err, "Failed to update namespace with 'kueue-managed: true' label", "namespace", namespaceName)
		return fmt.Errorf("failed to update namespace: %w", err)
	}

	logger.Info("Successfully labeled namespace", "namespace", namespaceName)
	return nil
}

func CreateDefaultResourceFlavors(ctx context.Context, c client.Client) ([]kaiwo.ResourceFlavorSpec, map[string]kueuev1beta1.FlavorQuotas, error) {
	logger := log.FromContext(ctx)
	config := common.ConfigFromContext(ctx)

	var resourceFlavors []kaiwo.ResourceFlavorSpec
	nodePoolResources := make(map[string]kueuev1beta1.FlavorQuotas)
	nodePools := make(map[string][]string)

	resourceAggregates := make(map[string]map[corev1.ResourceName]*resource.Quantity)

	// Get node list dynamically
	nodeList := GetNodeResources(ctx, c)

	if config.Nodes.ExcludeMasterNodesFromNodePools {
		logger.Info("Excluding master/control-plane nodes from nodepools. Set EXCLUDE_MASTER_NODES_FROM_NODE_POOLS=false to include them")
	} else {
		logger.Info("Including master/control-plane nodes in nodepools. Set EXCLUDE_MASTER_NODES_FROM_NODE_POOLS=true to exclude them")
	}

	for _, node := range nodeList {
		if node.Unschedulable {
			logger.Info("Skipping cordoned node", "node", node.Name)
			continue
		}
		// **Skip Control Plane Nodes**
		if config.Nodes.ExcludeMasterNodesFromNodePools {
			if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
				continue
			}
			if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
				continue
			}
		}
		gpuCount := 0
		gpuVendor := "cpu"
		gpuType := "only"

		// Identify AMD GPU Nodes
		if gpuID, exists := node.Labels["amd.com/gpu.product-name"]; exists {
			gpuType = CleanAMDGPUName(gpuID)
			if count, ok := node.Labels["beta.amd.com/gpu.family.AI"]; ok {
				gpuCount, _ = strconv.Atoi(count)
				gpuVendor = "amd"
			}
		}

		// Identify NVIDIA GPU Nodes
		if gpuProduct, exists := node.Labels["nvidia.com/gpu.product"]; exists {
			gpuType = strings.ReplaceAll(strings.ToLower(gpuProduct), "-", "")
			if count, ok := node.Labels["nvidia.com/gpu.count"]; ok {
				gpuCount, _ = strconv.Atoi(count)
				gpuVendor = "nvidia" //nolint:goconst
			}
		}

		// Compute nominal CPU and memory (90% of total)
		nominalCPU := resource.NewQuantity(int64(float64(node.CPU)*cpuMemoryDiscountFactor), resource.DecimalSI)
		nominalMemory := resource.NewQuantity(int64(float64(node.Memory)*cpuMemoryDiscountFactor*1024*1024*1024), resource.BinarySI)

		flavorName := fmt.Sprintf("%s-%s-%dgpu-%dcore-%dgi", gpuVendor, gpuType, gpuCount, nominalCPU.Value(), nominalMemory.Value()/(1024*1024*1024))

		// Track node membership in the nodepool
		nodePools[flavorName] = append(nodePools[flavorName], node.Name)
		logger.Info("Node added to node pool", "node", node.Name, "nodePool", flavorName)

		if _, exists := resourceAggregates[flavorName]; !exists {
			resourceAggregates[flavorName] = map[corev1.ResourceName]*resource.Quantity{
				corev1.ResourceCPU:    resource.NewQuantity(0, resource.DecimalSI),
				corev1.ResourceMemory: resource.NewQuantity(0, resource.BinarySI),
			}
			if gpuCount > 0 {
				gpuResource := corev1.ResourceName("amd.com/gpu")
				if gpuVendor == "nvidia" {
					gpuResource = corev1.ResourceName("nvidia.com/gpu")
				}
				resourceAggregates[flavorName][gpuResource] = resource.NewQuantity(0, resource.DecimalSI)
			}
		}

		resourceAggregates[flavorName][corev1.ResourceCPU].Add(*nominalCPU)
		resourceAggregates[flavorName][corev1.ResourceMemory].Add(*nominalMemory)

		if gpuCount > 0 {
			gpuResource := corev1.ResourceName("amd.com/gpu")
			if gpuVendor == "nvidia" {
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

		flavor := kaiwo.ResourceFlavorSpec{
			Name: flavorName,
			NodeLabels: map[string]string{
				common.DefaultNodePoolLabel: flavorName,
			},
			TopologyName: common.DefaultTopologyName,
		}

		// TODO: Look into why automatic scheduling is not working
		// At the moment, we are adding toleration to pod spec ourselves
		// https://kueue.sigs.k8s.io/docs/concepts/resource_flavor/#resourceflavor-tolerations-for-automatic-scheduling
		// if addTaints && !strings.HasPrefix(flavorName, "cpu-only") {
		// 	flavor.Tolerations = []corev1.Toleration{GPUToleration}
		// }

		resourceFlavors = append(resourceFlavors, flavor)

	}

	resourceFlavors = RemoveDuplicateResourceFlavors(resourceFlavors)

	gpuTaint := corev1.Taint{
		Key:    config.Nodes.DefaultGpuTaintKey,
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	}

	for flavorName, nodeNames := range nodePools {
		for _, nodeName := range nodeNames {
			err := LabelNode(ctx, c, nodeName, common.DefaultKaiwoWorkerLabel, "true")
			if err == nil {
				logger.Info("Labeled node", "node", nodeName, "label", common.DefaultKaiwoWorkerLabel, "value", "true")
			} else {
				return nil, nil, fmt.Errorf("failed to label node %s: %w", nodeName, err)
			}

			err = LabelNode(ctx, c, nodeName, common.DefaultNodePoolLabel, flavorName)
			if err == nil {
				logger.Info("Labeled node", "node", nodeName, "label", common.DefaultNodePoolLabel, "value", flavorName)
			} else {
				return nil, nil, fmt.Errorf("failed to label node %s: %w", nodeName, err)
			}

			if !strings.Contains(flavorName, common.CPUOnly) {
				err = LabelNode(ctx, c, nodeName, common.GPUModelLabel, strings.Split(flavorName, "-")[1])
				if err == nil {
					logger.Info("Labeled node", "node", nodeName, "label", common.GPUModelLabel, "value", strings.Split(flavorName, "-")[1])
				} else {
					return nil, nil, fmt.Errorf("failed to label node %s: %w", nodeName, err)
				}
			}

			if config.Nodes.AddTaintsToGpuNodes {
				if !strings.HasPrefix(flavorName, common.CPUOnly) {
					err = TaintNode(ctx, c, nodeName, gpuTaint)
					if err != nil {
						logger.Error(err, "Failed to taint GPU node", "node", nodeName)
					}
				}
			}
		}
	}
	logger.Info("Final generated node pools", "nodePools", nodePools)
	logger.Info("Final generated node pool resources", "nodePoolResources", nodePoolResources)
	logger.Info("Final generated resource flavors", "resourceFlavors", resourceFlavors)

	return resourceFlavors, nodePoolResources, nil
}

func RemoveDuplicateResourceFlavors(flavors []kaiwo.ResourceFlavorSpec) []kaiwo.ResourceFlavorSpec {
	uniqueMap := make(map[string]kaiwo.ResourceFlavorSpec)
	for _, flavor := range flavors {
		uniqueMap[flavor.Name] = flavor
	}

	uniqueFlavors := make([]kaiwo.ResourceFlavorSpec, 0, len(uniqueMap))
	for _, flavor := range uniqueMap {
		uniqueFlavors = append(uniqueFlavors, flavor)
	}
	return uniqueFlavors
}

func CreateClusterQueue(nodePoolResources map[string]kueuev1beta1.FlavorQuotas, name string, cohort string) kaiwo.ClusterQueue {
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
	return kaiwo.ClusterQueue{
		Name:       name,
		Namespaces: []string{},
		Spec: kaiwo.ClusterQueueSpec{
			NamespaceSelector: &metav1.LabelSelector{},
			Cohort:            kueuev1beta1.CohortReference(cohort),
			ResourceGroups:    resourceGroups,
		},
	}
}

func ConvertKaiwoToKueueResourceFlavors(kaiwoFlavors []kaiwo.ResourceFlavorSpec) []kueuev1beta1.ResourceFlavor {
	var kueueFlavors []kueuev1beta1.ResourceFlavor
	for _, rf := range kaiwoFlavors {
		kueueFlavors = append(kueueFlavors, ConvertKaiwoToKueueResourceFlavor(rf))
	}
	return kueueFlavors
}

func ConvertKaiwoToKueueResourceFlavor(kaiwoFlavor kaiwo.ResourceFlavorSpec) kueuev1beta1.ResourceFlavor {
	// Copy the node labels
	nodeLabels := make(map[string]string, len(kaiwoFlavor.NodeLabels))
	for k, v := range kaiwoFlavor.NodeLabels {
		nodeLabels[k] = v
	}

	var topologyRef *kueuev1beta1.TopologyReference
	if kaiwoFlavor.TopologyName != "" {
		ref := kueuev1beta1.TopologyReference(kaiwoFlavor.TopologyName)
		topologyRef = &ref
	} else {
		ref := kueuev1beta1.TopologyReference(common.DefaultTopologyName)
		topologyRef = &ref
		nodeLabels[common.DefaultKaiwoWorkerLabel] = "true"
	}

	return kueuev1beta1.ResourceFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: kaiwoFlavor.Name},
		Spec: kueuev1beta1.ResourceFlavorSpec{
			NodeLabels:   nodeLabels,
			TopologyName: topologyRef,
		},
	}
}

func ConvertKaiwoToKueueTopologies(kaiwoTopologies []kaiwo.Topology) []kueuev1alpha1.Topology {
	var kueueTopologies []kueuev1alpha1.Topology
	for _, topo := range kaiwoTopologies {
		kueueTopologies = append(kueueTopologies, ConvertKaiwoToKueueTopology(topo))
	}
	return kueueTopologies
}

func ConvertKaiwoToKueueTopology(kaiwoTopology kaiwo.Topology) kueuev1alpha1.Topology {
	levels := make([]kueuev1alpha1.TopologyLevel, len(kaiwoTopology.Spec.Levels))
	for i, l := range kaiwoTopology.Spec.Levels {
		levels[i] = kueuev1alpha1.TopologyLevel{
			NodeLabel: l.NodeLabel,
		}
	}

	return kueuev1alpha1.Topology{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Topology",
			APIVersion: "kueue.x-k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: kaiwoTopology.Name,
		},
		Spec: kueuev1alpha1.TopologySpec{
			Levels: levels,
		},
	}
}

func ConvertKaiwoToKueueClusterQueue(kaiwoQueue kaiwo.ClusterQueue) kueuev1beta1.ClusterQueue {
	return kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name: kaiwoQueue.Name,
		},
		Spec: ConvertKaiwoToKueueSpec(kaiwoQueue.Spec),
	}
}

// ConvertKaiwoToKueueSpec converts from Kaiwo's simplified ClusterQueueSpec to the actual Kueue version
func ConvertKaiwoToKueueSpec(in kaiwo.ClusterQueueSpec) kueuev1beta1.ClusterQueueSpec {
	return kueuev1beta1.ClusterQueueSpec{
		ResourceGroups:          in.ResourceGroups,
		Cohort:                  in.Cohort,
		QueueingStrategy:        in.QueueingStrategy,
		NamespaceSelector:       in.NamespaceSelector,
		FlavorFungibility:       in.FlavorFungibility,
		Preemption:              in.Preemption,
		AdmissionChecks:         in.AdmissionChecks,
		AdmissionChecksStrategy: in.AdmissionChecksStrategy,
		StopPolicy:              in.StopPolicy,
		FairSharing:             in.FairSharing,
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

func FindFlavor(flavors []kueuev1beta1.ResourceFlavor, name string) (kueuev1beta1.ResourceFlavor, bool) {
	for _, flavor := range flavors {
		if flavor.Name == name {
			return flavor, true
		}
	}
	return kueuev1beta1.ResourceFlavor{}, false
}

func FindTopology(topologies []kueuev1alpha1.Topology, name string) (kueuev1alpha1.Topology, bool) {
	for _, topo := range topologies {
		if topo.Name == name {
			return topo, true
		}
	}
	return kueuev1alpha1.Topology{}, false
}

func CompareResourceFlavors(a, b kueuev1beta1.ResourceFlavor) bool {
	return reflect.DeepEqual(a.Spec, b.Spec)
}

func CompareClusterQueues(a, b kueuev1beta1.ClusterQueue) bool {
	return reflect.DeepEqual(a.Spec, b.Spec)
}

func CompareTopologies(a, b kueuev1alpha1.Topology) bool {
	return reflect.DeepEqual(a.Spec, b.Spec)
}

func ComparePriorityClasses(a, b kueuev1beta1.WorkloadPriorityClass) bool {
	return reflect.DeepEqual(a.Value, b.Value)
}

func CreateDefaultTopology(ctx context.Context, c client.Client) ([]kaiwo.Topology, error) {
	defaultTopology := kaiwo.Topology{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.DefaultTopologyName,
		},
		Spec: kaiwo.TopologySpec{
			Levels: []kueuev1alpha1.TopologyLevel{
				{NodeLabel: common.DefaultTopologyBlockLabel},
				{NodeLabel: common.DefaultTopologyRackLabel},
				{NodeLabel: common.DefaultTopologyHostLabel},
			},
		},
	}
	return []kaiwo.Topology{defaultTopology}, nil
}
