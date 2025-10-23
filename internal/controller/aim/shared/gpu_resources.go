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

package shared

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// GPUResourceInfo contains aggregated GPU resource information for a specific GPU model.
type GPUResourceInfo struct {
	// ResourceName is the full Kubernetes resource name (e.g., "amd.com/gpu").
	ResourceName string

	// Allocatable is the total allocatable GPU resources across all nodes.
	Allocatable resource.Quantity

	// Capacity is the total GPU capacity across all nodes.
	Capacity resource.Quantity
}

// GetClusterGPUResources returns an aggregated view of all GPU resources in the cluster.
// It scans all nodes and aggregates resources that start with "amd.com/" or "nvidia.com/".
// Returns a map where keys are GPU models (e.g., "MI300X", "A100") extracted from node labels,
// and values contain the resource name and total allocatable/capacity across all nodes.
func GetClusterGPUResources(ctx context.Context, k8sClient client.Client) (map[string]GPUResourceInfo, error) {
	// List all nodes in the cluster
	var nodes corev1.NodeList
	if err := k8sClient.List(ctx, &nodes); err != nil {
		return nil, err
	}

	// Aggregate GPU resources by model
	gpuResources := make(map[string]GPUResourceInfo)

	for _, node := range nodes.Items {
		// Process GPU resources on this node
		aggregateNodeResources(&node, gpuResources)
	}

	return gpuResources, nil
}

// extractGPUModelFromNodeLabels extracts the GPU model from node labels.
// AMD and NVIDIA use different label schemes:
//   - AMD: amd.com/gpu.device.id (e.g., "0x74a1" for MI300X) or amd.com/gpu.product
//   - NVIDIA: nvidia.com/gpu.product (e.g., "A100-SXM4-40GB") or nvidia.com/gpu.family
func extractGPUModelFromNodeLabels(labels map[string]string, resourceName string) string {
	if strings.HasPrefix(resourceName, "amd.com/") {
		// Try AMD-specific labels
		if product, ok := labels["amd.com/gpu.product"]; ok {
			return normalizeGPUModel(product)
		}
		if deviceID, ok := labels["amd.com/gpu.device.id"]; ok {
			// Map device IDs to model names
			return mapAMDDeviceIDToModel(deviceID)
		}
		// Fallback
		return "AMD-GPU"
	}

	if strings.HasPrefix(resourceName, "nvidia.com/") {
		// Try NVIDIA-specific labels
		if product, ok := labels["nvidia.com/gpu.product"]; ok {
			return normalizeGPUModel(product)
		}
		if family, ok := labels["nvidia.com/gpu.family"]; ok {
			return normalizeGPUModel(family)
		}
		// Fallback
		return "NVIDIA-GPU"
	}

	return "Unknown-GPU"
}

// normalizeGPUModel normalizes GPU model names for consistency.
// Examples:
//   - "A100-SXM4-40GB" -> "A100"
//   - "mi300x" -> "MI300X"
func normalizeGPUModel(model string) string {
	model = strings.ToUpper(model)
	// Remove common suffixes
	model = strings.Split(model, "-")[0]
	return model
}

// mapAMDDeviceIDToModel maps AMD device IDs to model names.
// TODO: Expand this mapping as needed
func mapAMDDeviceIDToModel(deviceID string) string {
	// Remove "0x" prefix if present
	deviceID = strings.TrimPrefix(strings.ToLower(deviceID), "0x")

	knownDevices := map[string]string{
		"74a1": "MI300X",
		"740f": "MI210",
		"7408": "MI250X",
		// TODO: Add more mappings
	}

	if model, ok := knownDevices[deviceID]; ok {
		return model
	}

	return "AMD-" + strings.ToUpper(deviceID)
}

// aggregateNodeResources processes a single node's resources and adds them to the aggregate map.
func aggregateNodeResources(node *corev1.Node, aggregate map[string]GPUResourceInfo) {
	allocatable := node.Status.Allocatable
	capacity := node.Status.Capacity

	// Process all resources in the node
	for resourceName, allocatableQty := range allocatable {
		resourceNameStr := string(resourceName)

		// Filter for GPU resources (amd.com/* or nvidia.com/*)
		if !isGPUResource(resourceNameStr) {
			continue
		}

		// Get the corresponding capacity
		capacityQty, hasCapacity := capacity[resourceName]
		if !hasCapacity {
			// If no capacity is defined, skip this resource
			continue
		}

		// Extract GPU model from node labels
		gpuModel := extractGPUModelFromNodeLabels(node.Labels, resourceNameStr)

		// Add to or update the aggregate
		info, exists := aggregate[gpuModel]
		if !exists {
			info = GPUResourceInfo{
				ResourceName: resourceNameStr,
				Allocatable:  allocatableQty.DeepCopy(),
				Capacity:     capacityQty.DeepCopy(),
			}
		} else {
			// Add the quantities
			info.Allocatable.Add(allocatableQty)
			info.Capacity.Add(capacityQty)
		}

		aggregate[gpuModel] = info
	}
}

// isGPUResource checks if a resource name represents a GPU resource.
// Returns true if the resource name starts with "amd.com/" or "nvidia.com/".
func isGPUResource(resourceName string) bool {
	return strings.HasPrefix(resourceName, "amd.com/") ||
		strings.HasPrefix(resourceName, "nvidia.com/")
}

// IsGPUAvailable checks if a specific GPU model is available in the cluster.
// The gpuModel parameter should be the GPU model name (e.g., "MI300X", "A100"), not the resource name.
func IsGPUAvailable(ctx context.Context, k8sClient client.Client, gpuModel string) (bool, error) {
	resources, err := GetClusterGPUResources(ctx, k8sClient)
	if err != nil {
		return false, err
	}

	// Normalize the input for comparison
	normalizedModel := strings.ToUpper(gpuModel)

	info, exists := resources[normalizedModel]
	if !exists {
		return false, nil
	}

	// GPU is available if there's any capacity
	return !info.Capacity.IsZero(), nil
}

// ListAvailableGPUs returns a list of all GPU resource types available in the cluster.
func ListAvailableGPUs(ctx context.Context, k8sClient client.Client) ([]string, error) {
	resources, err := GetClusterGPUResources(ctx, k8sClient)
	if err != nil {
		return nil, err
	}

	gpuTypes := make([]string, 0, len(resources))
	for resourceName, info := range resources {
		// Only include GPUs with non-zero capacity
		if !info.Capacity.IsZero() {
			gpuTypes = append(gpuTypes, resourceName)
		}
	}

	return gpuTypes, nil
}

// TemplateRequiresGPU returns true if the template spec declares a GPU selector with a model.
func TemplateRequiresGPU(spec aimv1alpha1.AIMServiceTemplateSpecCommon) bool {
	if spec.GpuSelector == nil {
		return false
	}
	return strings.TrimSpace(spec.GpuSelector.Model) != ""
}

// UpdateTemplateGPUAvailability checks whether the GPU model declared by the template exists in the cluster.
// It updates the provided TemplateObservation with the result of the check.
func UpdateTemplateGPUAvailability(
	ctx context.Context,
	k8sClient client.Client,
	spec aimv1alpha1.AIMServiceTemplateSpecCommon,
	obs *TemplateObservation,
) error {
	if obs == nil {
		return nil
	}

	obs.GPUModel = ""
	obs.GPUAvailable = true
	obs.GPUChecked = false

	if !TemplateRequiresGPU(spec) {
		obs.GPUChecked = true
		return nil
	}

	model := strings.TrimSpace(spec.GpuSelector.Model)
	obs.GPUModel = strings.ToUpper(model)

	available, err := IsGPUAvailable(ctx, k8sClient, model)
	if err != nil {
		return err
	}

	obs.GPUChecked = true
	obs.GPUAvailable = available

	return nil
}
