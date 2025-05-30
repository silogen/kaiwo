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
	"context"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// StorageHandler handles the storage-related reconcilers (PVCs and download config map / job)
type StorageHandler struct {
	ObjectKey      client.ObjectKey
	CommonMetaSpec v1alpha1.CommonMetaSpec
}

func NewStorageHandler(workloadHandler WorkloadHandler) *StorageHandler {
	commonSpec := workloadHandler.Workload.GetCommonSpec()
	if commonSpec.Storage == nil || !commonSpec.Storage.StorageEnabled {
		return nil
	}
	return &StorageHandler{
		ObjectKey:      client.ObjectKeyFromObject(workloadHandler.Workload.GetKaiwoWorkloadObject()),
		CommonMetaSpec: workloadHandler.Workload.GetCommonSpec(),
	}
}

const (
	DataStoragePostfix = "data"
	HfStoragePostfix   = "hf"
)

func (h StorageHandler) GetResourceReconcilers() []ResourceReconciler {
	var reconcilers []ResourceReconciler
	storageSpec := h.CommonMetaSpec.Storage

	if storageSpec.HasData() {
		reconcilers = append(reconcilers, NewPvcReconciler(
			client.ObjectKey{
				Name:      baseutils.FormatNameWithPostfix(h.ObjectKey.Name, DataStoragePostfix),
				Namespace: h.ObjectKey.Namespace,
			},
			storageSpec.Data.StorageSize,
			storageSpec.StorageClassName,
			storageSpec.AccessMode,
		))
	}
	if storageSpec.HasHfDownloads() {
		reconcilers = append(reconcilers, NewPvcReconciler(
			client.ObjectKey{
				Name:      baseutils.FormatNameWithPostfix(h.ObjectKey.Name, HfStoragePostfix),
				Namespace: h.ObjectKey.Namespace,
			},
			storageSpec.HuggingFace.StorageSize,
			storageSpec.StorageClassName,
			storageSpec.AccessMode,
		))
	}
	if storageSpec.HasDownloads() {
		reconcilers = append(
			reconcilers,
			NewDownloadConfigMapReconciler(
				h.ObjectKey,
				*storageSpec,
			),
			NewDownloadJobReconciler(
				h.ObjectKey,
				*storageSpec,
				h.CommonMetaSpec.Env,
			))
	}

	return reconcilers
}

func (h StorageHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, previousWorkloadStatus v1alpha1.WorkloadStatus) (v1alpha1.WorkloadStatus, []metav1.Condition, error) {
	status, conditions, err := ObserveOverallStatus(ctx, k8sClient, h.GetResourceReconcilers(), previousWorkloadStatus)
	if status == nil {
		return "", conditions, err
	}
	return *status, conditions, err
}

func updatePodSpecStorage(podSpec *corev1.PodSpec, storageSpec v1alpha1.StorageSpec, ownerName string) {
	if !storageSpec.StorageEnabled {
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
