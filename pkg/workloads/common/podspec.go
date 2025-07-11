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

func UpdatePodSpec(config KaiwoConfigContext, workload KaiwoWorkload, gpuScheduling *GpuSchedulingResult, template *corev1.PodTemplateSpec) {
	commonMetaSpec := workload.GetCommonSpec()
	workloadName := workload.GetKaiwoWorkloadObject().GetName()

	// Propagate labels
	if template.Labels == nil {
		template.Labels = make(map[string]string)
	}
	for key, value := range commonMetaSpec.PodTemplateSpecLabels {
		template.Labels[key] = value
	}
	UpdateLabels(workload, &template.ObjectMeta)

	// Propagate annotations
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}
	for key, value := range workload.GetKaiwoWorkloadObject().GetAnnotations() {
		template.Annotations[key] = value
	}

	if commonMetaSpec.RequiredTopologyLabel != "" {
		template.Annotations[KueueRequiredTopologyKey] = commonMetaSpec.RequiredTopologyLabel
	} else if commonMetaSpec.PreferredTopologyLabel != "" {
		template.Annotations[KueuePreferredTopologyKey] = commonMetaSpec.PreferredTopologyLabel
	} else {
		template.Annotations[KueuePreferredTopologyKey] = DefaultTopologyHostLabel
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

	// Update node affinity if GPUs requested with node selector terms
	if gpuScheduling.GpusRequested() && gpuScheduling.GpuCountPerReplica > 0 && (len(gpuScheduling.NodeSelectorTerms.MatchExpressions) > 0 || len(gpuScheduling.NodeSelectorTerms.MatchFields) > 0) {
		if template.Spec.Affinity == nil {
			template.Spec.Affinity = &corev1.Affinity{}
		}
		if template.Spec.Affinity.NodeAffinity == nil {
			template.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
		}
		if template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
		}
		template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, gpuScheduling.NodeSelectorTerms)
	}

	// Update container specs
	for i := range template.Spec.Containers {
		updateMainContainer(config, commonMetaSpec, gpuScheduling, &template.Spec.Containers[i])
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
}

// updateMainContainer updates the container specifications for main containers
func updateMainContainer(_ KaiwoConfigContext, kaiwoCommonMetaSpec kaiwo.CommonMetaSpec, gpuScheduling *GpuSchedulingResult, container *corev1.Container) {
	// Update base
	updateContainerBase(kaiwoCommonMetaSpec, container)

	resourceRequirements := gpuScheduling.CreateResourceRequirements(kaiwoCommonMetaSpec.Resources)

	fillContainerResources(container, &resourceRequirements, false)

	replicas := kaiwoCommonMetaSpec.Replicas
	if baseutils.ValueOrDefault(replicas) == 0 {
		replicas = baseutils.Pointer(1)
	}

	numGpusPerReplica := 0
	if gpuScheduling.GpusRequested() {
		numGpusPerReplica = gpuScheduling.GpuCountPerReplica
	}

	envVarsToAppend := []corev1.EnvVar{
		{Name: "NUM_GPUS", Value: fmt.Sprintf("%d", numGpusPerReplica**replicas)},
		{Name: "NUM_REPLICAS", Value: fmt.Sprintf("%d", *replicas)},
		{Name: "NUM_GPUS_PER_REPLICA", Value: fmt.Sprintf("%d", numGpusPerReplica)},
	}
	container.Env = append(container.Env, envVarsToAppend...)

	if container.Image == "" {
		container.Image = kaiwoCommonMetaSpec.Image
	}
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
