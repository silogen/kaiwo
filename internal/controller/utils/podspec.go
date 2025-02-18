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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var DefaultGpuResourceKey = baseutils.GetEnv("DEFAULT_GPU_RESOURCE_KEY", "amd.com/gpu")

// GetDefaultPodTemplate defines a reusable Pod template with security and resource settings.
func GetDefaultPodTemplate() corev1.PodTemplateSpec {
	return GetPodTemplate(*resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), false)
}

func GetPodTemplate(dshmSize resource.Quantity, dangerous bool) corev1.PodTemplateSpec {
	podTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "workload",
					Image:           baseutils.DefaultRayImage,
					ImagePullPolicy: corev1.PullAlways,
					Env: []corev1.EnvVar{
						{Name: "HF_HOME", Value: "/workload/.cache/huggingface"},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: *resource.NewQuantity(16*1024*1024*1024, resource.BinarySI), // Minimum requirement for Ray Head pod
							corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),                // Minimum requirement for Ray Head pod
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: *resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
							corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "dshm", MountPath: "/dev/shm"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
							// Default size limit, can be overridden in `getPodTemplate`
							SizeLimit: &dshmSize,
						},
					},
				},
			},
		},
	}
	if !dangerous {
		podTemplate.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser:  baseutils.Pointer(int64(1000)),
			RunAsGroup: baseutils.Pointer(int64(1000)),
			FSGroup:    baseutils.Pointer(int64(1000)),
		}
	}

	return podTemplate
}

func AdjustResourceRequestsAndLimits(ctx context.Context, gpuVendor string, gpuCount int, replicas int, gpusPerReplica int, podTemplateSpec *corev1.PodTemplateSpec) error {
	logger := log.FromContext(ctx)

	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		logger.Error(err, "Failed to adjust resource requests and limits")
		return err
	}

	gpuResourceKey := GetGpuResourceKey(gpuVendor)

	// Modify resource requests/limits only if GPUs are requested
	if gpuCount > 0 {
		podTemplateSpec.Spec.Containers[0].Resources = getResourceRequestsAndLimits(gpuResourceKey, int32(gpuCount))
		logger.Info("Successfully adjusted resource requests and limits",
			"Container", podTemplateSpec.Spec.Containers[0].Name)
	}

	// Append new GPU-related environment variables to the container
	envVarsToAppend := []corev1.EnvVar{
		{Name: "NUM_GPUS", Value: fmt.Sprintf("%d", gpuCount)},
		{Name: "NUM_REPLICAS", Value: fmt.Sprintf("%d", replicas)},
		{Name: "NUM_GPUS_PER_REPLICA", Value: fmt.Sprintf("%d", gpusPerReplica)},
	}

	// Append to existing environment variables
	podTemplateSpec.Spec.Containers[0].Env = append(podTemplateSpec.Spec.Containers[0].Env, envVarsToAppend...)

	logger.Info("Successfully added GPU environment variables",
		"Container", podTemplateSpec.Spec.Containers[0].Name,
		"Total Env Vars", len(podTemplateSpec.Spec.Containers[0].Env))

	return nil
}

// AddEntrypoint updates the entrypoint command in the PodTemplateSpec.
func AddEntrypoint(ctx context.Context, entrypoint string, podTemplateSpec *corev1.PodTemplateSpec) error {
	logger := log.FromContext(ctx)

	if entrypoint == "" {
		logger.Info("Entrypoint is empty, skipping modification")
		return nil
	}

	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		logger.Error(err, "Failed to add entrypoint")
		return err
	}

	logger.Info("Adding entrypoint to container", "Entrypoint", entrypoint, "Container", podTemplateSpec.Spec.Containers[0].Name)
	podTemplateSpec.Spec.Containers[0].Command = []string{"sh", "-c", entrypoint}

	logger.Info("Successfully added entrypoint", "Entrypoint", entrypoint)
	return nil
}

func AddEnvVars(ctx context.Context, UserEnvVars []corev1.EnvVar, podTemplateSpec *corev1.PodTemplateSpec) error {
	logger := log.FromContext(ctx)

	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		logger.Error(err, "Failed to add environment variables")
		return err
	}

	container := &podTemplateSpec.Spec.Containers[0]

	logger.Info("Appending user environment variables", "Container", container.Name, "ExistingVars", len(container.Env), "NewVars", len(UserEnvVars))

	// Append UserEnvVars without overriding existing ones
	container.Env = append(container.Env, UserEnvVars...)

	logger.Info("Updated container environment variables", "Container", container.Name, "TotalVars", len(container.Env))

	logger.Info("Successfully added environment variables")
	return nil
}

func GetGpuResourceKey(vendor string) string {
	vendor = strings.ToUpper(vendor)
	switch vendor {
	case "NVIDIA":
		return "nvidia.com/gpu"
	case "AMD":
		return "amd.com/gpu"
	default:
		return DefaultGpuResourceKey
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
