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
	"os"

	"k8s.io/apimachinery/pkg/runtime"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

const (
	dataStoragePostfix = "data"
	hfStoragePostfix   = "hf"
)

// ReconcileStorage ensures that any requested PVCs are available and linked to the owner
func ReconcileStorage(r client.Client, s *runtime.Scheme, ctx context.Context, owner client.Object, spec *v1alpha1.StorageSpec) error {
	if !spec.StorageEnabled {
		return nil
	}

	var storageclass string

	if spec.StorageClassName == "" {
		storageclass = os.Getenv("STORAGECLASS")
		if storageclass == "" {
			return fmt.Errorf("storage class name is empty")
		}
	} else {
		storageclass = spec.StorageClassName
	}

	logger := log.FromContext(ctx)

	createStorage := func(amount string, postfix string) error {
		pvcName := baseutils.FormatNameWithPostfix(owner.GetName(), postfix)
		logger.Info(fmt.Sprintf("Creating PVC %s", pvcName))
		storageQuantity := resource.MustParse(amount)

		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: owner.GetNamespace()}, pvc); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			logger.Info(fmt.Sprintf("PVC %s already exists", pvcName))
			return nil
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: owner.GetNamespace(),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					spec.AccessMode,
				},
				StorageClassName: &storageclass,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: storageQuantity,
					},
				},
			},
		}

		// Ensure PVC is attached to the parent object
		if err := ctrl.SetControllerReference(owner, pvc, s); err != nil {
			logger.Error(err, "Failed to set PVC controller reference")
			return err
		}

		if err := r.Create(ctx, pvc); err != nil {
			return err
		}

		return nil
	}

	// Data storage
	if spec.Data.IsRequested() {
		if err := createStorage(spec.Data.StorageSize, dataStoragePostfix); err != nil {
			return err
		}
	}

	// HF storage
	if spec.HuggingFace.IsRequested() {
		if err := createStorage(spec.HuggingFace.StorageSize, hfStoragePostfix); err != nil {
			return err
		}
	}

	return nil
}

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
		addStorageVolume(dataStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, dataStoragePostfix))
	}

	if storageSpec.HuggingFace.IsRequested() {
		addStorageVolume(hfStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, hfStoragePostfix))
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
			addVolumeMount(container, dataStoragePostfix, storageSpec.Data.MountPath)
		}
		if storageSpec.HuggingFace.IsRequested() {
			addVolumeMount(container, hfStoragePostfix, storageSpec.HuggingFace.MountPath)
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
