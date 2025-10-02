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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	DefaultMemory = resource.MustParse("16Gi")
	DefaultCPU    = resource.MustParse("2")
)

// CreateResourceRequirements converts the scheduling config into ResourceRequirements
// that can be used to modify the workload containers
func CreateResourceRequirements(config KaiwoConfigContext, resourceConfig ResourceConfig, rayhead bool) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if resourceConfig.DefaultResources != nil {
		resources = *resourceConfig.DefaultResources
	}

	if resources.Requests == nil {
		resources.Requests = corev1.ResourceList{}
	}
	if resources.Limits == nil {
		resources.Limits = corev1.ResourceList{}
	}

	if rayhead {
		resourceConfig.GpusPerReplica = 0
		resourceConfig.TotalGpus = 0
		resourceConfig.Replicas = 1
	}

	gpuCount := resourceConfig.GpusPerReplica
	hasGpus := gpuCount > 0

	if hasGpus {
		gpuResourceKey := getGpuResourceKey(resourceConfig.GpuVendor, config.Nodes.DefaultGpuResourceKey)
		quantity := resource.MustParse(fmt.Sprintf("%d", gpuCount))

		// GPU value is always overwritten
		resources.Requests[corev1.ResourceName(gpuResourceKey)] = quantity
		resources.Limits[corev1.ResourceName(gpuResourceKey)] = quantity
	}

	updateResourceList := func(resourceList corev1.ResourceList) {
		if _, exists := resourceList[corev1.ResourceCPU]; !exists {
			if hasGpus {
				resourceList[corev1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%d", gpuCount*4))
			} else {
				resourceList[corev1.ResourceCPU] = DefaultCPU
			}
		}
		if _, exists := resourceList[corev1.ResourceMemory]; !exists {
			if hasGpus {
				resourceList[corev1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32))
			} else {
				resourceList[corev1.ResourceMemory] = DefaultMemory
			}
		}
	}

	updateResourceList(resources.Limits)
	updateResourceList(resources.Requests)

	return resources
}

// fillContainerResources fills container resources with a given template if they are not already set
func fillContainerResources(container *corev1.Container, resources *corev1.ResourceRequirements, override bool) {
	if resources == nil {
		return
	}

	fillResourceList(&container.Resources.Requests, resources.Requests, override)
	fillResourceList(&container.Resources.Limits, resources.Limits, override)
}

func fillResourceList(dest *corev1.ResourceList, src corev1.ResourceList, override bool) {
	if *dest == nil {
		*dest = corev1.ResourceList{}
	}
	for k, v := range src {
		if _, exists := (*dest)[k]; override || !exists {
			(*dest)[k] = v
		}
	}
}

func getGpuResourceKey(vendor string, defaultVendor string) string {
	vendor = strings.ToUpper(vendor)
	switch vendor {
	case "NVIDIA":
		return "nvidia.com/gpu"
	case "AMD":
		return "amd.com/gpu"
	default:
		return defaultVendor
	}
}
