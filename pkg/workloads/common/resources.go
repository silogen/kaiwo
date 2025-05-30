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

package common

import (
	"fmt"
	"strings"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	DefaultMemory = resource.MustParse("16Gi")
	DefaultCPU    = resource.MustParse("2")
)

// FillPodResources fills pod resources with a given template if they are not already set
func FillPodResources(podSpec *corev1.PodSpec, resources *corev1.ResourceRequirements, override bool) {
	for i := range podSpec.Containers {
		fillContainerResources(&podSpec.Containers[i], resources, override)
	}
	for i := range podSpec.InitContainers {
		fillContainerResources(&podSpec.InitContainers[i], resources, override)
	}
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

func adjustResourceRequestsAndLimits(config KaiwoConfigContext, gpuVendor string, gpuCount int, replicas int, gpusPerReplica int, podTemplateSpec *corev1.PodTemplateSpec, override bool, rayhead bool) error {
	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	gpuResourceKey := getGpuResourceKey(gpuVendor, config.Nodes.DefaultGpuResourceKey)

	// Modify resource requests/limits only if GPUs are requested
	if gpusPerReplica > 0 && !rayhead {
		updatedResources := getResourceRequestsAndLimits(gpuResourceKey, int32(gpusPerReplica))
		// Update the resources that have not been set yet (allowing the user to override the defaults here)
		fillContainerResources(&podTemplateSpec.Spec.Containers[0], &updatedResources, override)
	}

	// Append new GPU-related environment variables to the container
	envVarsToAppend := []corev1.EnvVar{
		{Name: "NUM_GPUS", Value: fmt.Sprintf("%d", gpuCount)},
		{Name: "NUM_REPLICAS", Value: fmt.Sprintf("%d", replicas)},
		{Name: "NUM_GPUS_PER_REPLICA", Value: fmt.Sprintf("%d", gpusPerReplica)},
	}

	// Append to existing environment variables
	podTemplateSpec.Spec.Containers[0].Env = append(podTemplateSpec.Spec.Containers[0].Env, envVarsToAppend...)

	// Check all containers for any GPU resource requests
	if config.Nodes.AddTaintsToGpuNodes {
		for _, container := range podTemplateSpec.Spec.Containers {
			requests := container.Resources.Requests
			if requests != nil {
				if _, ok := requests[corev1.ResourceName("nvidia.com/gpu")]; ok {
					goto AddToleration
				}
				if _, ok := requests[corev1.ResourceName("amd.com/gpu")]; ok {
					goto AddToleration
				}
			}
		}
	}
	return nil

AddToleration:
	podTemplateSpec.Spec.Tolerations = append(podTemplateSpec.Spec.Tolerations, corev1.Toleration{
		Key:      config.Nodes.DefaultGpuTaintKey,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	})
	return nil
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

func getResourceRequestsAndLimits(gpuResourceKey string, gpuCount int32) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:                  resource.MustParse(fmt.Sprintf("%d", gpuCount*4)),
			corev1.ResourceMemory:               resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32)),
			corev1.ResourceName(gpuResourceKey): resource.MustParse(fmt.Sprintf("%d", gpuCount)),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:                  resource.MustParse(fmt.Sprintf("%d", gpuCount*4)),
			corev1.ResourceMemory:               resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32)),
			corev1.ResourceName(gpuResourceKey): resource.MustParse(fmt.Sprintf("%d", gpuCount)),
		},
	}
}

func SyncGpuMetaFromPodSpec(podSpec corev1.PodSpec, meta *kaiwo.CommonMetaSpec) {
	if meta.Gpus == 0 {
		for _, c := range podSpec.Containers {
			for _, gpuKey := range []string{"amd.com/gpu", "nvidia.com/gpu"} {
				if gpuQty, ok := c.Resources.Requests[corev1.ResourceName(gpuKey)]; ok {
					if gpuVal := gpuQty.Value(); gpuVal > 0 {
						meta.Gpus = int(gpuVal)
						meta.GpuVendor = strings.Split(gpuKey, ".")[0]
						return
					}
				}
			}
		}
	}
}
