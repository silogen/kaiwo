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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DataStoragePostfix = "data"
	HfStoragePostfix   = "hf"
)

type StorageReconciler struct {
	ResourceReconcilerBase[*corev1.PersistentVolumeClaim]
	Amount           string
	StorageClassName string
	AccessMode       corev1.PersistentVolumeAccessMode
}

func NewStorageReconciler(objectKey client.ObjectKey,
	accessMode corev1.PersistentVolumeAccessMode,
	storageClassName string,
	amount string,
) *StorageReconciler {
	reconciler := &StorageReconciler{
		ResourceReconcilerBase: ResourceReconcilerBase[*corev1.PersistentVolumeClaim]{
			ObjectKey: objectKey,
		},
		Amount:           amount,
		StorageClassName: storageClassName,
		AccessMode:       accessMode,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *StorageReconciler) Build(ctx context.Context, _ client.Client) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				r.AccessMode,
			},
			StorageClassName: &r.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(r.Amount),
				},
			},
		},
	}
	return pvc, nil
}

func (r *StorageReconciler) GetEmptyObject() *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{}
}
