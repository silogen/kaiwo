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
	"context"
	"fmt"
	"strings"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	pkgk8s "github.com/silogen/kaiwo/pkg/k8s"
)

// FillRayClusterPodSpec fills PodSpec for RayCluster head and worker nodes
func FillRayClusterPodSpec(ctx context.Context, client client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob, rayCluster *rayv1.RayClusterSpec) error {
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

	addS3InitContainer(podSpec, kaiwoJob)

	container := &corev1.Container{
		Name:            name,
		Image:           kaiwoJob.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env: append([]corev1.EnvVar{
			{Name: "HF_HOME", Value: "/workload/.cache/huggingface"},
		}, kaiwoJob.Spec.EnvVars...),
		Ports: []corev1.ContainerPort{
			{ContainerPort: 6379, Name: "gcs-server"},
			{ContainerPort: 8265, Name: "dashboard"},
			{ContainerPort: 10001, Name: "client"},
		},
		Resources: getGpuResourceRequests(kaiwoJob, gpuResourceKey, gpuCount),
		VolumeMounts: getVolumeMounts(
			kaiwoJob,
			"/workload/mounted",
		),
	}

	podSpec.Containers = []corev1.Container{*container}
	podSpec.Volumes = getVolumes(kaiwoJob)
}

// addS3InitContainer adds an init container to pre-download S3 data
func addS3InitContainer(podSpec *corev1.PodSpec, kaiwoJob *kaiwov1alpha1.KaiwoJob) {
	if kaiwoJob.Spec.ConfigSource == nil || strings.ToLower(kaiwoJob.Spec.ConfigSource.Type) != "s3" {
		return
	}

	secretRef := kaiwoJob.Spec.ConfigSource.S3Credentials
	envVars := []corev1.EnvVar{}

	if secretRef != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "AWS_ACCESS_KEY_ID", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretRef.Name}, Key: "access-key"}}},
			corev1.EnvVar{Name: "AWS_SECRET_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretRef.Name}, Key: "secret-key"}}},
			corev1.EnvVar{Name: "S3_ENDPOINT", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretRef.Name}, Key: "endpoint"}}},
		)
	}

	initContainer := corev1.Container{
		Name:  "s3-downloader",
		Image: "minio/mc",
		Command: []string{
			"sh", "-c",
			fmt.Sprintf(`
				mc alias set myminio $S3_ENDPOINT $AWS_ACCESS_KEY_ID $AWS_SECRET_ACCESS_KEY &&
				mc cp --recursive myminio/%s/%s /workload
			`, kaiwoJob.Spec.ConfigSource.S3BucketName, kaiwoJob.Spec.ConfigSource.S3Path),
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workload", MountPath: "/workload"},
		},
		Env: envVars,
	}

	podSpec.InitContainers = append(podSpec.InitContainers, initContainer)
}

// FillPodSpec configures the PodSpec for Jobs & Deployments.
func FillPodSpec(ctx context.Context, client client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob, podSpec *corev1.PodSpec) error {
	// Determine GPU resource key
	gpuResourceKey := getGpuResourceKey(kaiwoJob.Spec.GpuVendor)

	// Add S3 init container if necessary
	// addS3InitContainer(podSpec, kaiwoJob)

	// Configure the main container
	container := &corev1.Container{
		Name:            kaiwoJob.Name,
		Image:           kaiwoJob.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env: append([]corev1.EnvVar{
			{Name: "HF_HOME", Value: "/workload/.cache/huggingface"},
		}, kaiwoJob.Spec.EnvVars...),
		Ports:     []corev1.ContainerPort{},
		Resources: getGpuResourceRequests(kaiwoJob, gpuResourceKey, int32(kaiwoJob.Spec.Gpus)),
		VolumeMounts: getVolumeMounts(
			kaiwoJob,
			"/workload/mounted",
		),
	}

	if kaiwoJob.Spec.EntryPoint != "" {
		container.Command = []string{"sh", "-c", kaiwoJob.Spec.EntryPoint}
	}

	podSpec.Containers = []corev1.Container{*container}
	podSpec.Volumes = getVolumes(kaiwoJob)
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

func getVolumeMounts(kaiwoJob *kaiwov1alpha1.KaiwoJob, mountedPath string) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{Name: "dshm", MountPath: "/dev/shm"},
	}

	if kaiwoJob.Spec.Storage.StorageEnabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      kaiwoJob.Name + "-main",
			MountPath: mountedPath,
		})
	}

	return volumeMounts
}

func getVolumes(kaiwoJob *kaiwov1alpha1.KaiwoJob) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "dshm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: "Memory"},
			},
		},
	}

	if kaiwoJob.Spec.Storage.StorageEnabled {
		storageRequest := resource.MustParse(kaiwoJob.Spec.Storage.StorageSize)

		volumes = append(volumes, corev1.Volume{
			Name: kaiwoJob.Name + "-main",
			VolumeSource: corev1.VolumeSource{
				Ephemeral: &corev1.EphemeralVolumeSource{
					VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
							StorageClassName: &kaiwoJob.Spec.Storage.StorageClassName,
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: storageRequest,
								},
							},
						},
					},
				},
			},
		})
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
