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
	"fmt"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	pkgk8s "github.com/silogen/kaiwo/pkg/k8s"
)

// FillRayClusterPodSpec fills PodSpec for RayCluster head and worker nodes
func FillRayClusterPodSpec(kaiwoJob *kaiwov1alpha1.KaiwoJob, rayCluster *rayv1.RayClusterSpec) error {
	gpusPerNode := int32(8) // Default, should be fetched dynamically
	numReplicas, nodeGpuRequest := pkgk8s.CalculateNumberOfReplicas(kaiwoJob.Spec.Gpus, int(gpusPerNode), nil)

	numReplicasInt32, nodeGpuRequestInt32 := int32(numReplicas), int32(nodeGpuRequest)

	// Determine GPU resource key
	gpuResourceKey := getGpuResourceKey(kaiwoJob.Spec.GpuVendor)

	configureRayContainer(
		"ray-head",
		&rayCluster.HeadGroupSpec.Template.Spec,
		kaiwoJob,
		gpuResourceKey,
		nodeGpuRequestInt32,
	)

	for i := range rayCluster.WorkerGroupSpecs {
		rayCluster.WorkerGroupSpecs[i].Replicas = &numReplicasInt32

		configureRayContainer(
			fmt.Sprintf("ray-worker-%d", i),
			&rayCluster.WorkerGroupSpecs[i].Template.Spec,
			kaiwoJob,
			gpuResourceKey,
			nodeGpuRequestInt32,
		)
	}

	return nil
}

// configureRayContainer applies container settings for Ray head and worker nodes
func configureRayContainer(name string, podSpec *corev1.PodSpec, kaiwoJob *kaiwov1alpha1.KaiwoJob, gpuResourceKey string, gpuCount int32) {
	podSpec.SecurityContext = &corev1.PodSecurityContext{
		RunAsUser:  Int64Ptr(1000),
		RunAsGroup: Int64Ptr(1000),
		FSGroup:    Int64Ptr(1000),
	}

	container := &corev1.Container{
		Name:            name,
		Image:           kaiwoJob.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env:             kaiwoJob.Spec.EnvVars,
		Ports: []corev1.ContainerPort{
			{ContainerPort: 6379, Name: "gcs-server"},
			{ContainerPort: 8265, Name: "dashboard"},
			{ContainerPort: 10001, Name: "client"},
		},
		Resources:    getGpuResourceRequests(kaiwoJob, gpuResourceKey, gpuCount),
		VolumeMounts: getVolumeMounts(),
	}

	podSpec.Containers = []corev1.Container{*container}
	podSpec.Volumes = getVolumes()
}

// FillPodSpec configures the PodSpec for Jobs & Deployments.
func FillPodSpec(kaiwoJob *kaiwov1alpha1.KaiwoJob, podSpec *corev1.PodSpec) error {
	// Determine GPU resource key
	gpuResourceKey := getGpuResourceKey(kaiwoJob.Spec.GpuVendor)

	// Configure the main container
	container := &corev1.Container{
		Name:            kaiwoJob.Name,
		Image:           kaiwoJob.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Ports:           []corev1.ContainerPort{},
		Resources:       getGpuResourceRequests(kaiwoJob, gpuResourceKey, int32(kaiwoJob.Spec.Gpus)),
	}

	if kaiwoJob.Spec.EntryPoint != "" {
		container.Command = []string{"sh", "-c", kaiwoJob.Spec.EntryPoint}
	}

	podSpec.Containers = []corev1.Container{*container}
	podSpec.Volumes = getVolumes()
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	return nil
}

func getGpuResourceKey(vendor string) string {
	switch vendor {
	case "NVIDIA":
		return "nvidia.com/gpu"
	case "AMD":
		return "amd.com/gpu"
	default:
		return "amd.com/gpu"
	}
}

func getGpuResourceRequests(kaiwoJob *kaiwov1alpha1.KaiwoJob, gpuResourceKey string, gpuCount int32) corev1.ResourceRequirements {
	if kaiwoJob.Spec.Gpus == 0 {
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

func getVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{Name: "dshm", MountPath: "/dev/shm"},
	}

	return volumeMounts
}

func getVolumes() []corev1.Volume {
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

func Int32Ptr(i int32) *int32 {
	return &i
}

func Int64Ptr(i int64) *int64 {
	return &i
}

func BoolPtr(b bool) *bool {
	return &b
}
