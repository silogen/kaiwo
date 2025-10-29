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
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// GPUResourceInfo contains GPU resource information for a specific GPU model.
type GPUResourceInfo struct {
	// ResourceName is the full Kubernetes resource name (e.g., "amd.com/gpu").
	ResourceName string
}

// GetClusterGPUResources returns an aggregated view of all GPU resources in the cluster.
// It scans all nodes and aggregates resources that start with "amd.com/" or "nvidia.com/".
// Returns a map where keys are GPU models (e.g., "MI300X", "A100") extracted from node labels,
// and values contain the resource name.
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

		// Temporarily skip aggregateNodeResources(...) and do GPU searching based on Labels
		filterdGPULabelResources(&node, gpuResources)
	}

	return gpuResources, nil
}

// extractGPUModelFromNodeLabels extracts the GPU model from node labels.
// Supports multiple label formats from AMD and NVIDIA GPU labellers:
//   - AMD: amd.com/gpu.product-name, beta.amd.com/gpu.product-name, amd.com/gpu.device-id,
//     amd.com/gpu.family, and count-encoded variants (e.g., amd.com/gpu.product-name.MI300X=4)
//   - NVIDIA: nvidia.com/gpu.product, nvidia.com/mig.product, nvidia.com/gpu.family,
//     and Node Feature Discovery labels (feature.node.kubernetes.io/nvidia-gpu-model)
//
// Returns a normalized GPU model name (e.g., "MI300X", "A100") or empty string if model cannot be determined.
// Nodes with GPU resources but insufficient labels will be excluded from template matching.
func extractGPUModelFromNodeLabels(labels map[string]string, resourceName string) string {
	if strings.HasPrefix(resourceName, "amd.com/") {
		return extractAMDModel(labels)
	}

	if strings.HasPrefix(resourceName, "nvidia.com/") {
		return extractNvidiaModel(labels)
	}

	return ""
}

// normalizeGPUModel normalizes GPU model names for consistency.
// Examples:
//   - "A100-SXM4-40GB" -> "A100"
//   - "MI300X (rev 2)" -> "MI300X"
//   - "Tesla-T4-SHARED" -> "T4"
func normalizeGPUModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}

	model = strings.ToUpper(strings.ReplaceAll(model, "_", "-"))

	tokens := strings.FieldsFunc(model, func(r rune) bool {
		switch r {
		case '-', ' ', '/', ':':
			return true
		default:
			return false
		}
	})

	if token := pickGPUModelToken(tokens); token != "" {
		return token
	}

	for _, token := range tokens {
		if token != "" {
			return token
		}
	}

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
		"744c": "MI250",
		"74e0": "MI300A",
		// TODO: Add more mappings
	}

	if model, ok := knownDevices[deviceID]; ok {
		return model
	}

	return "AMD-" + strings.ToUpper(deviceID)
}

func extractAMDModel(labels map[string]string) string {
	// Preferred direct labels exposed by the AMD labeller
	if product := labelValue(labels, "amd.com/gpu.product-name", "beta.amd.com/gpu.product-name"); product != "" {
		return normalizeGPUModel(product)
	}

	// Labels with counts encoded in the key suffix
	if productKey := extractLabelSuffix(labels, "amd.com/gpu.product-name.", "beta.amd.com/gpu.product-name."); productKey != "" {
		return normalizeGPUModel(productKey)
	}

	// Fall back to device identifiers
	if deviceID := labelValue(labels, "amd.com/gpu.device-id", "beta.amd.com/gpu.device-id"); deviceID != "" {
		return mapAMDDeviceIDToModel(deviceID)
	}
	if deviceKey := extractLabelSuffix(labels, "amd.com/gpu.device-id.", "beta.amd.com/gpu.device-id."); deviceKey != "" {
		return mapAMDDeviceIDToModel(deviceKey)
	}

	// GPU family (AI, NV, etc.) as a last resort
	if family := labelValue(labels, "amd.com/gpu.family", "beta.amd.com/gpu.family"); family != "" {
		return normalizeGPUModel(family)
	}
	if familyKey := extractLabelSuffix(labels, "amd.com/gpu.family.", "beta.amd.com/gpu.family."); familyKey != "" {
		return normalizeGPUModel(familyKey)
	}

	return ""
}

func extractNvidiaModel(labels map[string]string) string {
	if product := labelValue(labels, "nvidia.com/gpu.product", "nvidia.com/mig.product"); product != "" {
		return normalizeGPUModel(product)
	}
	if productKey := extractLabelSuffix(labels, "nvidia.com/gpu.product.", "nvidia.com/mig.product."); productKey != "" {
		return normalizeGPUModel(productKey)
	}
	if family := labelValue(labels, "nvidia.com/gpu.family"); family != "" {
		return normalizeGPUModel(family)
	}
	if familyKey := extractLabelSuffix(labels, "nvidia.com/gpu.family."); familyKey != "" {
		return normalizeGPUModel(familyKey)
	}
	// Node Feature Discovery publishes a descriptive label as well
	if feature := labelValue(labels, "feature.node.kubernetes.io/nvidia-gpu-model"); feature != "" {
		return normalizeGPUModel(feature)
	}
	return ""
}

func labelValue(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value, ok := labels[key]; ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func extractLabelSuffix(labels map[string]string, prefixes ...string) string {
	for _, prefix := range prefixes {
		for key, value := range labels {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			value = strings.TrimSpace(value)
			if value == "" || value == "0" {
				continue
			}
			suffix := strings.TrimPrefix(key, prefix)
			if suffix != "" {
				return suffix
			}
		}
	}
	return ""
}

var gpuModelTokenRegex = regexp.MustCompile(`^(MI|ME|RX|RTX|GTX|A|H|L|T|V|K|P|QUADRO|TESLA|GRID)?[A-Z]*[0-9]+[A-Z0-9]*$`)

func pickGPUModelToken(tokens []string) string {
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		switch token {
		case "AMD", "NVIDIA", "TESLA", "RTX", "INSTINCT":
			continue
		}

		if gpuModelTokenRegex.MatchString(token) {
			token = strings.TrimPrefix(token, "INSTINCT")
			return token
		}
	}
	return ""
}

// GPU discovery on labels only
// Skips GPUs where the model cannot be determined from node labels (strict matching requirement).
func filterdGPULabelResources(node *corev1.Node, aggregate map[string]GPUResourceInfo) {
	for _, resourcePrefix := range []string{"amd.com/", "nvidia.com/"} {

		// Extract GPU model from node labels
		gpuModel := extractGPUModelFromNodeLabels(node.Labels, resourcePrefix)

		// Skip GPUs where model cannot be determined (insufficient node labels)
		if gpuModel == "" {
			continue
		}

		// Add to the aggregate if not already present
		if _, exists := aggregate[gpuModel]; !exists {
			aggregate[gpuModel] = GPUResourceInfo{
				ResourceName: resourcePrefix + "gpu",
			}
		}
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
// The input is normalized to handle variants like "MI300X (rev 2)" or "Instinct MI300X".
func IsGPUAvailable(ctx context.Context, k8sClient client.Client, gpuModel string) (bool, error) {
	resources, err := GetClusterGPUResources(ctx, k8sClient)
	if err != nil {
		return false, err
	}

	// Normalize the input for comparison (handles variants and extra tokens)
	normalizedModel := normalizeGPUModel(gpuModel)

	_, exists := resources[normalizedModel]
	if !exists {
		return false, nil
	}

	// GPU is available even if there's no capacity
	return true, nil
}

// ListAvailableGPUs returns a list of all GPU resource types available in the cluster.
func ListAvailableGPUs(ctx context.Context, k8sClient client.Client) ([]string, error) {
	resources, err := GetClusterGPUResources(ctx, k8sClient)
	if err != nil {
		return nil, err
	}

	gpuTypes := make([]string, 0, len(resources))
	for gpuModel := range resources {
		gpuTypes = append(gpuTypes, gpuModel)
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
// The GPU model is normalized to ensure consistent matching across different label formats.
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
	// Normalize the model name for consistent storage and comparison
	obs.GPUModel = normalizeGPUModel(model)

	available, err := IsGPUAvailable(ctx, k8sClient, model)
	if err != nil {
		return err
	}

	obs.GPUChecked = true
	obs.GPUAvailable = available

	return nil
}
