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

package workloadcommon

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

// FillPodSpec configures the PodSpec for Jobs & Deployments.
func FillPodSpec(kaiwoJob *kaiwov1alpha1.KaiwoJob, podSpec *corev1.PodSpec) error {
	// Determine GPU resource key
	vendor := "AMD"
	if kaiwoJob.Spec.GpuVendor != nil {
		vendor = *kaiwoJob.Spec.GpuVendor
	}
	gpuResourceKey := GetGpuResourceKey(vendor)

	// Configure the main container
	container := &corev1.Container{
		Name:            kaiwoJob.Name,
		Image:           *kaiwoJob.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Ports:           []corev1.ContainerPort{},
		Resources:       GetGpuResourceRequests(kaiwoJob, gpuResourceKey, int32(*kaiwoJob.Spec.Gpus)),
	}

	if kaiwoJob.Spec.EntryPoint != nil && *kaiwoJob.Spec.EntryPoint != "" {
		container.Command = []string{"sh", "-c", *kaiwoJob.Spec.EntryPoint}
	}

	podSpec.Containers = []corev1.Container{*container}
	podSpec.Volumes = GetVolumes()
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	return nil
}

func GetGpuResourceKey(vendor string) string {
	switch vendor {
	case "NVIDIA":
		return "nvidia.com/gpu"
	case "AMD":
		return "amd.com/gpu"
	default:
		return "amd.com/gpu"
	}
}

func GetGpuResourceRequests(kaiwoJob *kaiwov1alpha1.KaiwoJob, gpuResourceKey string, gpuCount int32) corev1.ResourceRequirements {
	if *kaiwoJob.Spec.Gpus == 0 {
		return corev1.ResourceRequirements{}
	}

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

func GetVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{Name: "dshm", MountPath: "/dev/shm"},
	}

	return volumeMounts
}

func GetVolumes() []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "dshm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: "Memory"},
			},
		},
	}
	return volumes
}
