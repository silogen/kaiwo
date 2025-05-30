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

	"k8s.io/apimachinery/pkg/api/resource"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	corev1 "k8s.io/api/core/v1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

func UpdatePodSpec(config KaiwoConfigContext, kaiwoCommonMetaSpec kaiwo.CommonMetaSpec, labelContext KaiwoLabelContext, template *corev1.PodTemplateSpec, name string, replicas int, gpusPerReplica int, override bool, rayhead bool) error {
	// Update labels
	if template.ObjectMeta.Labels == nil {
		template.ObjectMeta.Labels = map[string]string{}
	}
	CopyLabels(kaiwoCommonMetaSpec.PodTemplateSpecLabels, &template.ObjectMeta)
	SetKaiwoSystemLabels(labelContext, &template.ObjectMeta)

	// Make sure there is an image set for each container
	// init containers are not included, as they are assumed to always be user given
	for i := range template.Spec.Containers {
		// If the container has no image set
		if template.Spec.Containers[i].Image == "" {
			if kaiwoCommonMetaSpec.Image != "" {
				// If a default image is provided, use it
				template.Spec.Containers[i].Image = kaiwoCommonMetaSpec.Image
			} else {
				// Otherwise use the default Ray image
				template.Spec.Containers[i].Image = config.Ray.DefaultRayImage
			}
		}
	}

	// Ensure that all image pull secrets are set
	if kaiwoCommonMetaSpec.ImagePullSecrets != nil {
		template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, kaiwoCommonMetaSpec.ImagePullSecrets...)
	}

	if kaiwoCommonMetaSpec.SecretVolumes != nil {
		addSecretVolumes(template, kaiwoCommonMetaSpec.SecretVolumes)
	}

	// Update resources if specified in the CRD

	FillPodResources(&template.Spec, kaiwoCommonMetaSpec.Resources, false)

	vendor := "amd"
	if kaiwoCommonMetaSpec.GpuVendor != "" {
		vendor = kaiwoCommonMetaSpec.GpuVendor
	}

	gpus := kaiwoCommonMetaSpec.Gpus

	if kaiwoCommonMetaSpec.Gpus == 0 {
		gpuResourceKey := corev1.ResourceName(getGpuResourceKey(vendor, config.Nodes.DefaultGpuResourceKey))
		if gpuQuantity, exists := template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			gpus = int(gpuQuantity.Value())
		}
	}

	// Adjust resource requests and limits based on GPUs
	if err := adjustResourceRequestsAndLimits(config, vendor, gpus, replicas, gpusPerReplica, template, override, rayhead); err != nil {
		return fmt.Errorf("failed to adjust resource requests and limits: %w", err)
	}

	// Add environmental variables
	if err := addEnvVars(kaiwoCommonMetaSpec.Env, template); err != nil {
		return fmt.Errorf("failed to add env vars: %w", err)
	}

	// Attach storage
	if kaiwoCommonMetaSpec.Storage != nil && kaiwoCommonMetaSpec.Storage.StorageEnabled {
		updatePodSpecStorage(&template.Spec, *kaiwoCommonMetaSpec.Storage, name)
	}

	return nil
}

func addSecretVolumes(template *corev1.PodTemplateSpec, secretVolumes []kaiwo.SecretVolume) {
	for _, volume := range secretVolumes {
		template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
			Name: volume.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: volume.SecretName,
				},
			},
		})
		for i := range template.Spec.Containers {
			template.Spec.Containers[i].VolumeMounts = append(template.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
				Name:      volume.Name,
				MountPath: volume.MountPath,
				SubPath:   volume.SubPath,
			})
		}
		for i := range template.Spec.InitContainers {
			template.Spec.InitContainers[i].VolumeMounts = append(template.Spec.InitContainers[i].VolumeMounts, corev1.VolumeMount{
				Name:      volume.Name,
				MountPath: volume.MountPath,
				SubPath:   volume.SubPath,
			})
		}
	}
}

func GetPodTemplate(config KaiwoConfigContext, dshmSize resource.Quantity, dangerous bool, resources corev1.ResourceRequirements, workloadContainerName string) corev1.PodTemplateSpec {
	resourceRequirements := resources.DeepCopy()
	if resourceRequirements.Requests == nil {
		resourceRequirements.Requests = corev1.ResourceList{}
	}
	if resourceRequirements.Limits == nil {
		resourceRequirements.Limits = corev1.ResourceList{}
	}
	if _, ok := resourceRequirements.Limits[corev1.ResourceMemory]; !ok {
		resourceRequirements.Limits[corev1.ResourceMemory] = DefaultMemory
	}
	if _, ok := resourceRequirements.Limits[corev1.ResourceCPU]; !ok {
		resourceRequirements.Limits[corev1.ResourceCPU] = DefaultCPU
	}
	if _, ok := resourceRequirements.Requests[corev1.ResourceMemory]; !ok {
		resourceRequirements.Requests[corev1.ResourceMemory] = DefaultMemory
	}
	if _, ok := resourceRequirements.Requests[corev1.ResourceCPU]; !ok {
		resourceRequirements.Requests[corev1.ResourceCPU] = DefaultCPU
	}

	podTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SchedulerName: config.Scheduling.KubeSchedulerName,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            workloadContainerName,
					ImagePullPolicy: corev1.PullAlways,
					Resources:       *resourceRequirements,
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
