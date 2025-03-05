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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func UpdatePodSpec(kaiwoCommonMetaSpec v1alpha1.CommonMetaSpec, labelContext baseutils.KaiwoLabelContext, template *corev1.PodTemplateSpec, name string, replicas int, gpusPerReplica int, override bool) error {
	// Update labels
	if template.ObjectMeta.Labels == nil {
		template.ObjectMeta.Labels = map[string]string{}
	}
	baseutils.CopyLabels(kaiwoCommonMetaSpec.PodTemplateSpecLabels, &template.ObjectMeta)
	baseutils.SetKaiwoSystemLabels(labelContext, &template.ObjectMeta)

	// Make sure there is an image set for each container
	// init containers are not included, as they are assumed to always be user given
	for i := range template.Spec.Containers {
		// If the container has no image set
		if template.Spec.Containers[i].Image == "" {
			if baseutils.ValueOrDefault(kaiwoCommonMetaSpec.Image) != "" {
				// If a default image is provided, use it
				template.Spec.Containers[i].Image = *kaiwoCommonMetaSpec.Image
			} else {
				// Otherwise use the default Ray image
				template.Spec.Containers[i].Image = baseutils.DefaultRayImage
			}
		}
	}

	// Ensure that all image pull secrets are set
	if kaiwoCommonMetaSpec.ImagePullSecrets != nil {
		template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, *kaiwoCommonMetaSpec.ImagePullSecrets...)
	}

	if kaiwoCommonMetaSpec.SecretVolumes != nil {
		addSecretVolumes(template, *kaiwoCommonMetaSpec.SecretVolumes)
	}

	// Fill resources
	FillPodResources(&template.Spec, kaiwoCommonMetaSpec.Resources, false)

	vendor := "AMD"
	if kaiwoCommonMetaSpec.GpuVendor != nil {
		vendor = *kaiwoCommonMetaSpec.GpuVendor
	}

	gpus := kaiwoCommonMetaSpec.Gpus

	if baseutils.ValueOrDefault(kaiwoCommonMetaSpec.Gpus) == 0 {
		gpuResourceKey := corev1.ResourceName(getGpuResourceKey(vendor))
		if gpuQuantity, exists := template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			gpus = baseutils.Pointer(int(gpuQuantity.Value()))
		}
	}

	// Adjust resource requests and limits based on GPUs
	if err := adjustResourceRequestsAndLimits(vendor, baseutils.ValueOrDefault(gpus), replicas, gpusPerReplica, template, override); err != nil {
		return fmt.Errorf("failed to adjust resource requests and limits: %w", err)
	}

	// Add environmental variables
	if err := addEnvVars(baseutils.ValueOrDefault(kaiwoCommonMetaSpec.Env), template); err != nil {
		return fmt.Errorf("failed to add env vars: %w", err)
	}

	// Attach storage
	if kaiwoCommonMetaSpec.Storage != nil && kaiwoCommonMetaSpec.Storage.StorageEnabled {
		updatePodSpecStorage(&template.Spec, *kaiwoCommonMetaSpec.Storage, name)
	}

	return nil
}

func addSecretVolumes(template *corev1.PodTemplateSpec, secretVolumes []v1alpha1.SecretVolume) {
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

var DefaultGpuResourceKey = baseutils.GetEnv("DEFAULT_GPU_RESOURCE_KEY", "amd.com/gpu")

var (
	DefaultMemory = resource.MustParse("16Gi")
	DefaultCPU    = resource.MustParse("2")
)

func GetPodTemplate(dshmSize resource.Quantity, dangerous bool, resources corev1.ResourceRequirements) corev1.PodTemplateSpec {
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
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "workload",
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

func adjustResourceRequestsAndLimits(gpuVendor string, gpuCount int, replicas int, gpusPerReplica int, podTemplateSpec *corev1.PodTemplateSpec, override bool) error {
	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	gpuResourceKey := getGpuResourceKey(gpuVendor)

	// Modify resource requests/limits only if GPUs are requested
	if gpusPerReplica > 0 {
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

	return nil
}

// AddEntrypoint updates the entrypoint command in the PodTemplateSpec.
func AddEntrypoint(entrypoint string, podTemplateSpec *corev1.PodTemplateSpec) error {
	if entrypoint == "" {
		// logger.Info("Entrypoint is empty, skipping modification")
		return nil
	}

	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	podTemplateSpec.Spec.Containers[0].Command = []string{"sh", "-c", entrypoint}

	return nil
}

func addEnvVars(UserEnvVars []corev1.EnvVar, podTemplateSpec *corev1.PodTemplateSpec) error {
	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	container := &podTemplateSpec.Spec.Containers[0]

	// Append UserEnvVars without overriding existing ones
	container.Env = append(container.Env, UserEnvVars...)

	return nil
}

func getGpuResourceKey(vendor string) string {
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

func updatePodSpecStorage(podSpec *corev1.PodSpec, storageSpec v1alpha1.StorageSpec, ownerName string) {
	if !storageSpec.StorageEnabled || storageSpec.Data == nil {
		return
	}

	addStorageVolume := func(name string, claimName string) {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	// Add volumes
	if storageSpec.Data.IsRequested() {
		addStorageVolume(DataStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, DataStoragePostfix))
	}

	if storageSpec.HuggingFace.IsRequested() {
		addStorageVolume(HfStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, HfStoragePostfix))
	}

	addVolumeMount := func(container *corev1.Container, name string, path string) {
		// logger.Info(fmt.Sprintf("Adding %s volume mount to %s", name, container.Name))
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: path,
		})
	}

	addEnvVar := func(container *corev1.Container, name string, value string) {
		// logger.Info(fmt.Sprintf("Adding %s env var to %s", name, container.Name))
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	addContainerInfo := func(container *corev1.Container) {
		if storageSpec.Data.IsRequested() {
			addVolumeMount(container, DataStoragePostfix, storageSpec.Data.MountPath)
		}
		if storageSpec.HuggingFace.IsRequested() {
			addVolumeMount(container, HfStoragePostfix, storageSpec.HuggingFace.MountPath)
			addEnvVar(container, "HF_HOME", storageSpec.HuggingFace.MountPath)
		}
	}

	for i := range podSpec.Containers {
		addContainerInfo(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		addContainerInfo(&podSpec.InitContainers[i])
	}
}
