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

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	workloadshared "github.com/silogen/kaiwo/pkg/workloads2/shared"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

func UpdatePodSpecStorage(ctx context.Context, podSpec *corev1.PodSpec, storageSpec v1alpha1.StorageSpec, ownerName string) error {
	logger := log.FromContext(ctx)

	if !storageSpec.StorageEnabled {
		return nil
	}

	addStorageVolume := func(name string, claimName string) {
		logger.Info(fmt.Sprintf("Adding %s volume", name))
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
		addStorageVolume(workloadshared.DataStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, workloadshared.DataStoragePostfix))
	}

	if storageSpec.HuggingFace.IsRequested() {
		addStorageVolume(workloadshared.HfStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, workloadshared.HfStoragePostfix))
	}

	addVolumeMount := func(container *corev1.Container, name string, path string) {
		logger.Info(fmt.Sprintf("Adding %s volume mount to %s", name, container.Name))
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: path,
		})
	}

	addEnvVar := func(container *corev1.Container, name string, value string) {
		logger.Info(fmt.Sprintf("Adding %s env var to %s", name, container.Name))
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	addContainerInfo := func(container *corev1.Container) {
		if storageSpec.Data.IsRequested() {
			addVolumeMount(container, workloadshared.DataStoragePostfix, storageSpec.Data.MountPath)
		}
		if storageSpec.HuggingFace.IsRequested() {
			addVolumeMount(container, workloadshared.HfStoragePostfix, storageSpec.HuggingFace.MountPath)
			addEnvVar(container, "HF_HOME", storageSpec.HuggingFace.MountPath)
		}
	}

	for i := range podSpec.Containers {
		addContainerInfo(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		addContainerInfo(&podSpec.InitContainers[i])
	}

	return nil
}
