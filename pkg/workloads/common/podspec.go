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

func UpdatePodSpec(config KaiwoConfigContext, workload KaiwoWorkload, resourceConfig ResourceConfig, template *corev1.PodTemplateSpec) {
	commonMetaSpec := workload.GetCommonSpec()
	workloadName := workload.GetKaiwoWorkloadObject().GetName()

	// Propagate labels
	if template.ObjectMeta.Labels == nil {
		template.ObjectMeta.Labels = make(map[string]string)
	}
	for key, value := range commonMetaSpec.PodTemplateSpecLabels {
		template.ObjectMeta.Labels[key] = value
	}
	UpdateLabels(workload, &template.ObjectMeta)

	if template.ObjectMeta.Annotations == nil {
		template.ObjectMeta.Annotations = make(map[string]string)
	}

	if commonMetaSpec.RequiredTopologyLabel != "" {
		template.ObjectMeta.Annotations[KueueRequiredTopologyKey] = commonMetaSpec.RequiredTopologyLabel
	} else if commonMetaSpec.PreferredTopologyLabel != "" {
		template.ObjectMeta.Annotations[KueuePreferredTopologyKey] = commonMetaSpec.PreferredTopologyLabel
	} else {
		template.ObjectMeta.Annotations[KueuePreferredTopologyKey] = DefaultTopologyHostLabel
	}

	// Add image pull secrets
	if commonMetaSpec.ImagePullSecrets != nil {
		template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, commonMetaSpec.ImagePullSecrets...)
	}

	if commonMetaSpec.Storage != nil && commonMetaSpec.Storage.StorageEnabled {

		addStorageVolume := func(name string, claimName string) {
			template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
				Name: name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			})
		}

		if commonMetaSpec.Storage.Data.IsRequested() {
			addStorageVolume(DataStoragePostfix, baseutils.FormatNameWithPostfix(workloadName, DataStoragePostfix))
		}

		if commonMetaSpec.Storage.HuggingFace.IsRequested() {
			addStorageVolume(HfStoragePostfix, baseutils.FormatNameWithPostfix(workloadName, HfStoragePostfix))
		}
	}

	// Add secret volumes
	for _, volume := range commonMetaSpec.SecretVolumes {
		template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
			Name: volume.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: volume.SecretName,
				},
			},
		})
	}

	if len(commonMetaSpec.GpuModels) > 0 && resourceConfig.GpusPerReplica > 0 {
		UpdatePodSpecWithGPUModelAffinity(template, commonMetaSpec.GpuModels, GPUModelLabel)
	}

	// Update container specs
	for i := range template.Spec.Containers {
		updateMainContainer(config, commonMetaSpec, resourceConfig, &template.Spec.Containers[i])
	}
	for i := range template.Spec.InitContainers {
		updateInitContainer(commonMetaSpec, &template.Spec.InitContainers[i])
	}

	// Add tolerations
	addToleration := func() {
		template.Spec.Tolerations = append(template.Spec.Tolerations, corev1.Toleration{
			Key:      config.Nodes.DefaultGpuTaintKey,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		})
	}

	// Check all containers for any GPU resource requests
	if config.Nodes.AddTaintsToGpuNodes {
		for _, container := range template.Spec.Containers {
			requests := container.Resources.Requests
			if requests != nil {
				if _, ok := requests[corev1.ResourceName("nvidia.com/gpu")]; ok {
					addToleration()
					break
				}
				if _, ok := requests[corev1.ResourceName("amd.com/gpu")]; ok {
					addToleration()
					break
				}
			}
		}
	}

	// Set priority class
	if commonMetaSpec.PriorityClass != "" {
		template.Spec.PriorityClassName = commonMetaSpec.PriorityClass
	}
}

// updateMainContainer updates the container specifications for main containers
func updateMainContainer(config KaiwoConfigContext, kaiwoCommonMetaSpec kaiwo.CommonMetaSpec, resourceConfig ResourceConfig, container *corev1.Container) {
	// Update base
	updateContainerBase(kaiwoCommonMetaSpec, container)

	// Update resources
	containerResourceRequirements := CreateResourceRequirements(config, resourceConfig)
	fillContainerResources(container, &containerResourceRequirements, false)

	envVarsToAppend := []corev1.EnvVar{
		{Name: "NUM_GPUS", Value: fmt.Sprintf("%d", resourceConfig.TotalGpus)},
		{Name: "NUM_REPLICAS", Value: fmt.Sprintf("%d", resourceConfig.Replicas)},
		{Name: "NUM_GPUS_PER_REPLICA", Value: fmt.Sprintf("%d", resourceConfig.GpusPerReplica)},
	}

	if container.Image == "" {
		container.Image = kaiwoCommonMetaSpec.Image
	}

	// Append to existing environment variables
	container.Env = append(container.Env, envVarsToAppend...)
}

// updateMainContainer updates the container specifications for init containers
func updateInitContainer(kaiwoCommonMetaSpec kaiwo.CommonMetaSpec, container *corev1.Container) {
	updateContainerBase(kaiwoCommonMetaSpec, container)
}

// updateContainerBase updates the container specifications for all containers (init and main)
func updateContainerBase(kaiwoCommonMetaSpec kaiwo.CommonMetaSpec, container *corev1.Container) {
	// Add storage mounts
	addVolumeMount := func(name string, path string) {
		// logger.Info(fmt.Sprintf("Adding %s volume mount to %s", name, container.Name))
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: path,
		})
	}
	envVars := kaiwoCommonMetaSpec.Env

	if kaiwoCommonMetaSpec.Storage != nil && kaiwoCommonMetaSpec.Storage.StorageEnabled {
		if kaiwoCommonMetaSpec.Storage.Data.IsRequested() {
			addVolumeMount(DataStoragePostfix, kaiwoCommonMetaSpec.Storage.Data.MountPath)
		}

		if kaiwoCommonMetaSpec.Storage.HuggingFace.IsRequested() {
			addVolumeMount(HfStoragePostfix, kaiwoCommonMetaSpec.Storage.HuggingFace.MountPath)
			envVars = append(envVars, corev1.EnvVar{
				Name:  "HF_HOME",
				Value: kaiwoCommonMetaSpec.Storage.HuggingFace.MountPath,
			})
		}
	}

	// Add env vars
	container.Env = append(container.Env, envVars...)

	// Secret volumes
	for _, volume := range kaiwoCommonMetaSpec.SecretVolumes {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: volume.MountPath,
			SubPath:   volume.SubPath,
		})
	}
}

func UpdatePodSpecWithGPUModelAffinity(template *corev1.PodTemplateSpec, gpuModels []string, gpuModelLabel string) {
	if len(gpuModels) == 0 {
		return
	}

	selectorTerm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      gpuModelLabel,
				Operator: corev1.NodeSelectorOpIn,
				Values:   gpuModels,
			},
		},
	}

	nodeAffinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{selectorTerm},
		},
	}

	if template.Spec.Affinity == nil {
		template.Spec.Affinity = &corev1.Affinity{}
	}

	template.Spec.Affinity.NodeAffinity = nodeAffinity
}

func GetPodTemplate(config KaiwoConfigContext, dshmSize resource.Quantity, dangerous bool, workloadContainerName string) corev1.PodTemplateSpec {
	podTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SchedulerName: config.Scheduling.KubeSchedulerName,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            workloadContainerName,
					ImagePullPolicy: corev1.PullAlways,
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
