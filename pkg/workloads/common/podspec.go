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

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// fillContainerResources fills container resources with a given template if they are not already set
func fillContainerResources(container *corev1.Container, resources corev1.ResourceRequirements, override bool) {
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

type PodTemplateSpecOption func(*corev1.PodTemplateSpec)

// UpdatePodTemplateSpecNonRay updates the pod template spec with the default options (for non-Ray use cases)
func UpdatePodTemplateSpecNonRay(config KaiwoConfigContext, workload KaiwoWorkload, gpuSchedulingResult *GpuSchedulingResult, template *corev1.PodTemplateSpec) {
	commonSpec := workload.GetCommonSpec()
	options := WithBaseOptions(workload)
	options = append(options, WithGpuSchedulingOptions(config, commonSpec.Resources, gpuSchedulingResult)...)
	UpdatePodTemplateSpec(template, options...)
}

// UpdatePodTemplateSpec ensures maps are non-nil, then applies each option
func UpdatePodTemplateSpec(tpl *corev1.PodTemplateSpec, opts ...PodTemplateSpecOption) {
	// metadata
	if tpl.Labels == nil {
		tpl.Labels = make(map[string]string)
	}
	if tpl.Annotations == nil {
		tpl.Annotations = make(map[string]string)
	}

	for _, opt := range opts {
		opt(tpl)
	}
}

// WithBaseOptions returns all the base options that are applied to all workloads
func WithBaseOptions(
	workload KaiwoWorkload,
) []PodTemplateSpecOption {
	commonMetaSpec := workload.GetCommonSpec()
	return []PodTemplateSpecOption{
		WithImage(commonMetaSpec.Image),
		WithCommonEnvVars(commonMetaSpec),
		WithCommonLabels(workload),
		WithKueueTopology(commonMetaSpec.RequiredTopologyLabel, commonMetaSpec.PreferredTopologyLabel),
		WithImagePullSecrets(commonMetaSpec.ImagePullSecrets),
		WithStorageVolumes(commonMetaSpec.Storage, workload.GetKaiwoWorkloadObject().GetName()),
		WithSecretVolumes(commonMetaSpec.SecretVolumes),
	}
}

// WithImage sets each container's image if it is missing
func WithImage(image string) PodTemplateSpecOption {
	return func(template *corev1.PodTemplateSpec) {
		forContainers(template, true, func(container *corev1.Container) {
			if container.Image == "" {
				container.Image = image
			}
		})
	}
}

// WithCommonEnvVars updates the containers' common (GPU or non-GPU) environmental variables
func WithCommonEnvVars(commonMetaSpec kaiwo.CommonMetaSpec) PodTemplateSpecOption {
	return func(podTemplateSpec *corev1.PodTemplateSpec) {
		envVars := commonMetaSpec.Env
		if commonMetaSpec.Storage != nil && commonMetaSpec.Storage.StorageEnabled && commonMetaSpec.Storage.HuggingFace.IsRequested() {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "HF_HOME",
				Value: commonMetaSpec.Storage.HuggingFace.MountPath,
			})
		}
		forContainers(podTemplateSpec, true, func(container *corev1.Container) {
			container.Env = append(container.Env, envVars...)
		})
	}
}

// WithCommonLabels adds labels to the pod template spec
func WithCommonLabels(workload KaiwoWorkload) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		for k, v := range workload.GetCommonSpec().PodTemplateSpecLabels {
			tpl.Labels[k] = v
		}
		UpdateLabels(workload, &tpl.ObjectMeta)
	}
}

// WithKueueTopology adds the Kueue topology annotations
func WithKueueTopology(required, preferred string) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		switch {
		case required != "":
			tpl.Annotations[KueueRequiredTopologyKey] = required
		case preferred != "":
			tpl.Annotations[KueuePreferredTopologyKey] = preferred
		default:
			tpl.Annotations[KueuePreferredTopologyKey] = DefaultTopologyHostLabel
		}
	}
}

// WithImagePullSecrets adds the required image pull secrets
func WithImagePullSecrets(secrets []corev1.LocalObjectReference) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		tpl.Spec.ImagePullSecrets = append(tpl.Spec.ImagePullSecrets, secrets...)
	}
}

// WithStorageVolumes adds the required storage volumes
func WithStorageVolumes(storage *kaiwo.StorageSpec, workloadName string) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		addVol := func(postfix string, claimName string) {
			tpl.Spec.Volumes = append(tpl.Spec.Volumes, corev1.Volume{
				Name: postfix,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			})
		}

		addVolumeMounts := func(name string, path string) {
			forContainers(tpl, true, func(container *corev1.Container) {
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      name,
					MountPath: path,
				})
			})
		}

		if storage != nil && storage.StorageEnabled {
			if storage.Data.IsRequested() {
				addVol(DataStoragePostfix, baseutils.FormatNameWithPostfix(workloadName, DataStoragePostfix))
				addVolumeMounts(DataStoragePostfix, storage.Data.MountPath)
			}
			if storage.HuggingFace.IsRequested() {
				addVol(HfStoragePostfix, baseutils.FormatNameWithPostfix(workloadName, HfStoragePostfix))
				addVolumeMounts(HfStoragePostfix, storage.HuggingFace.MountPath)
			}
		}
	}
}

func forContainers(template *corev1.PodTemplateSpec, initContainers bool, fn func(container *corev1.Container)) {
	if initContainers {
		for i := range template.Spec.InitContainers {
			fn(&template.Spec.InitContainers[i])
		}
	}
	for i := range template.Spec.Containers {
		fn(&template.Spec.Containers[i])
	}
}

// WithSecretVolumes adds the required secret volumes
func WithSecretVolumes(vols []kaiwo.SecretVolume) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		for _, v := range vols {
			tpl.Spec.Volumes = append(tpl.Spec.Volumes, corev1.Volume{
				Name: v.Name,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: v.SecretName},
				},
			})
			forContainers(tpl, true, func(container *corev1.Container) {
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      v.Name,
					MountPath: v.MountPath,
					SubPath:   v.SubPath,
				})
			})
		}
	}
}

// ----------------------------------------------------------------
// 3) GPU scheduling bits
// ----------------------------------------------------------------

// WithGpuSchedulingOptions adds the required GPU scheduling configuration to the pod spec.
func WithGpuSchedulingOptions(config KaiwoConfigContext, resources *corev1.ResourceRequirements, gpuSchedulingResult *GpuSchedulingResult) []PodTemplateSpecOption {
	resourceRequirements := &corev1.ResourceRequirements{}
	if resources != nil {
		resourceRequirements = resources.DeepCopy()
	}
	return []PodTemplateSpecOption{
		WithGpuEnvVars(gpuSchedulingResult),
		WithGpuNodeAffinity(gpuSchedulingResult),
		WithGpuToleration(config, gpuSchedulingResult),
		WithGpuResourceRequirements(resourceRequirements, gpuSchedulingResult),
	}
}

// WithGpuEnvVars updates the GPU environment variables of the container
func WithGpuEnvVars(gpuSchedulingResult *GpuSchedulingResult) PodTemplateSpecOption {
	return func(templateSpec *corev1.PodTemplateSpec) {
		num := 0
		if gpuSchedulingResult.GpusRequested() {
			num = gpuSchedulingResult.GpuCountPerReplica
		}
		env := []corev1.EnvVar{
			{Name: "NUM_GPUS", Value: fmt.Sprintf("%d", num**gpuSchedulingResult.Replicas)},
			{Name: "NUM_REPLICAS", Value: fmt.Sprintf("%d", *gpuSchedulingResult.Replicas)},
			{Name: "NUM_GPUS_PER_REPLICA", Value: fmt.Sprintf("%d", num)},
		}
		forContainers(templateSpec, false, func(container *corev1.Container) {
			container.Env = append(container.Env, env...)
		})
	}
}

// WithGpuNodeAffinity adds node affinity to the spec
func WithGpuNodeAffinity(gpuSchedulingResult *GpuSchedulingResult) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		if !gpuSchedulingResult.GpusRequested() || gpuSchedulingResult.GpuCountPerReplica == 0 {
			return
		}
		terms := gpuSchedulingResult.NodeSelectorTerms
		// ensure all the nesting pointers exist
		if tpl.Spec.Affinity == nil {
			tpl.Spec.Affinity = &corev1.Affinity{}
		}
		if tpl.Spec.Affinity.NodeAffinity == nil {
			tpl.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
		}
		if tpl.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			tpl.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
		}
		tpl.Spec.Affinity.NodeAffinity.
			RequiredDuringSchedulingIgnoredDuringExecution.
			NodeSelectorTerms = append(
			tpl.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
			terms,
		)
	}
}

// WithGpuToleration adds GPU tolerations to the podspec if nodes have taints and the container
// requests one or more GPUs
func WithGpuToleration(config KaiwoConfigContext, gpuSchedulingResult *GpuSchedulingResult) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		if !config.Nodes.AddTaintsToGpuNodes {
			return
		}
		// scan containers for any GPU request
		for _, c := range tpl.Spec.Containers {
			if q, ok := c.Resources.Requests[gpuSchedulingResult.GpuResourceName]; ok && q.Value() > 0 {
				tpl.Spec.Tolerations = append(tpl.Spec.Tolerations, corev1.Toleration{
					Key:      config.Nodes.DefaultGpuTaintKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				})
				return
			}
		}
	}
}

// WithResourceRequirements updates the pod template spec with the appropriate resource requirements based
// on the given requirements
func WithResourceRequirements(requirements *corev1.ResourceRequirements) PodTemplateSpecOption {
	resourceRequirements := corev1.ResourceRequirements{}
	if requirements != nil {
		resourceRequirements = *requirements.DeepCopy()
	}
	return func(tpl *corev1.PodTemplateSpec) {
		for i := range tpl.Spec.Containers {
			updateResourceListLimitsAndRequests(&resourceRequirements, corev1.ResourceCPU, DefaultCPU, false)
			updateResourceListLimitsAndRequests(&resourceRequirements, corev1.ResourceMemory, DefaultMemory, false)
			fillContainerResources(&tpl.Spec.Containers[i], resourceRequirements, false)
		}
	}
}

func updateResourceListLimitsAndRequests(resourceRequirements *corev1.ResourceRequirements, resourceName corev1.ResourceName, quantity resource.Quantity, override bool) {
	if resourceRequirements.Requests == nil {
		resourceRequirements.Requests = corev1.ResourceList{}
	}
	if resourceRequirements.Limits == nil {
		resourceRequirements.Limits = corev1.ResourceList{}
	}
	if _, exists := resourceRequirements.Requests[resourceName]; !exists || override {
		resourceRequirements.Requests[resourceName] = quantity
	}
	if _, exists := resourceRequirements.Limits[resourceName]; !exists || override {
		resourceRequirements.Limits[resourceName] = quantity
	}
}

// WithGpuResourceRequirements updates the pod template spec with the appropriate
// resource requirements, including GPU values, if any have been requested
func WithGpuResourceRequirements(requirements *corev1.ResourceRequirements, gpuSchedulingResult *GpuSchedulingResult) PodTemplateSpecOption {
	resourceRequirements := corev1.ResourceRequirements{}
	if requirements != nil {
		resourceRequirements = *requirements.DeepCopy()
	}
	gpuCount := 0
	if gpuSchedulingResult.GpusRequested() {
		gpuCount = gpuSchedulingResult.GpuCountPerReplica
		quantity := resource.MustParse(fmt.Sprintf("%d", gpuCount))

		updateResourceListLimitsAndRequests(&resourceRequirements, gpuSchedulingResult.GpuResourceName, quantity, true)
		updateResourceListLimitsAndRequests(&resourceRequirements, corev1.ResourceCPU, resource.MustParse(fmt.Sprintf("%d", gpuCount*4)), false)
		updateResourceListLimitsAndRequests(&resourceRequirements, corev1.ResourceMemory, resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32)), false)
	}

	return WithResourceRequirements(&resourceRequirements)
}

// WithRayHeadPodResourceRequirements updates the Ray head pod's memory requirements
func WithRayHeadPodResourceRequirements(headPodMemory resource.Quantity) PodTemplateSpecOption {
	return func(tpl *corev1.PodTemplateSpec) {
		if headPodMemory.Value() > 0 {
			for i := range tpl.Spec.Containers {
				updateResourceListLimitsAndRequests(&tpl.Spec.Containers[i].Resources, corev1.ResourceMemory, headPodMemory, true)
			}
		}
	}
}
