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
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

var KnownAmdGpuDevices = map[string]string{
	// AMD Instinct
	"738c": "MI100",
	"738e": "MI100",
	"7408": "MI250X",
	"740c": "MI250X", // MI250/MI250X
	"740f": "MI210",
	"7410": "MI210", // MI210 VF
	"74a0": "MI300A",
	"74a1": "MI300X",
	"74a2": "MI308X",
	"74a5": "MI325X",
	"74a8": "MI308X", // MI308X HF
	"74a9": "MI300X", // MI300X HF
	"74b5": "MI300X", // MI300X VF
	"74b6": "MI308X",
	"74b9": "MI325X", // MI325X VF
	"74bd": "MI300X", // MI300X HF
	"75a0": "MI350X",
	"75a3": "MI355X",
	"75b0": "MI350X", // MI350X VF
	"75b3": "MI355X", // MI355X VF
	// AMD Radeon Pro
	"7460": "V710",
	"7461": "V710", // Radeon Pro V710 MxGPU
	"7448": "W7900",
	"744a": "W7900", // W7900 Dual Slot
	"7449": "W7800", // W7800 48GB
	"745e": "W7800",
	"73a2": "W6900X",
	"73a3": "W6800",  // W6800 GL-XL
	"73ab": "W6800X", // W6800X / W6800X Duo
	"73a1": "V620",
	"73ae": "V620", // Radeon Pro V620 MxGPU
	// AMD Radeon
	"7550": "RX9070", // RX 9070 / 9070 XT
	"744c": "RX7900", // RX 7900 XT / 7900 XTX / 7900 GRE / 7900M
	"73af": "RX6900",
	"73bf": "RX6800", // RX 6800 / 6800 XT / 6900 XT
}

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
//   - AMD: amd.com/gpu.device-id (primary), beta.amd.com/gpu.device-id, amd.com/gpu.family,
//     and count-encoded variants (e.g., amd.com/gpu.device-id.74a1=4)
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
// Comprehensive mapping covering AMD Instinct, Radeon Pro, and Radeon GPUs.
func mapAMDDeviceIDToModel(deviceID string) string {
	// Remove "0x" prefix if present
	deviceID = strings.TrimPrefix(strings.ToLower(deviceID), "0x")

	if model, ok := KnownAmdGpuDevices[deviceID]; ok {
		return model
	}

	return "AMD-" + strings.ToUpper(deviceID)
}

func extractAMDModel(labels map[string]string) string {
	// Primary method: Use device ID for accurate model identification
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

// GetAMDDeviceIDsForModel returns all AMD device IDs that map to a given GPU model name.
// This is the inverse of mapAMDDeviceIDToModel, allowing lookup of all device IDs for a model.
// Example: GetAMDDeviceIDsForModel("MI300X") returns ["74a1", "74a9", "74b5", "74bd"]
// Returns empty slice if the model is not found or is not an AMD GPU.
func GetAMDDeviceIDsForModel(modelName string) []string {
	// Normalize the model name for comparison
	normalized := normalizeGPUModel(modelName)

	var deviceIDs []string
	for deviceID, model := range KnownAmdGpuDevices {
		if model == normalized {
			deviceIDs = append(deviceIDs, deviceID)
		}
	}

	// Sort device IDs to ensure consistent ordering across reconciliations
	// Map iteration order in Go is randomized, so we must sort to avoid
	// triggering unnecessary updates when the list is semantically identical
	sort.Strings(deviceIDs)

	return deviceIDs
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
