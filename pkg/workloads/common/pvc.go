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

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PvcReconciler struct {
	ObjectKey        client.ObjectKey
	Amount           resource.Quantity
	StorageClassName string
	AccessMode       corev1.PersistentVolumeAccessMode
}

func NewPvcReconciler(objectKey client.ObjectKey, amount string, storageClassName string, accessMode corev1.PersistentVolumeAccessMode) *PvcReconciler {
	return &PvcReconciler{
		ObjectKey:        objectKey,
		Amount:           resource.MustParse(amount),
		StorageClassName: storageClassName,
		AccessMode:       accessMode,
	}
}

func (r *PvcReconciler) GetInitializedObject() client.Object {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
		},
	}
}

func (r *PvcReconciler) BuildDesired(ctx context.Context, clusterCtx ClusterContext) (client.Object, error) {
	obj := r.GetInitializedObject().(*corev1.PersistentVolumeClaim)
	obj.Spec = corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			r.AccessMode,
		},
		StorageClassName: &r.StorageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: r.Amount,
			},
		},
	}
	return obj, nil
}

func (r *PvcReconciler) MutateActual(ctx context.Context, clusterCtx ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

const PvcReadyConditionType = "PvcReady"

func (r *PvcReconciler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousWorkloadStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	pvc := obj.(*corev1.PersistentVolumeClaim)
	var workloadStatus kaiwo.WorkloadStatus
	var conditions []metav1.Condition
	var conditionStatus metav1.ConditionStatus

	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		workloadStatus = kaiwo.WorkloadStatusComplete
		conditionStatus = metav1.ConditionTrue
	case corev1.ClaimPending:
		workloadStatus = kaiwo.WorkloadStatusPending
		conditionStatus = metav1.ConditionFalse
	case corev1.ClaimLost:
		workloadStatus = kaiwo.WorkloadStatusFailed
		conditionStatus = metav1.ConditionFalse
	}
	conditions = append(conditions, metav1.Condition{
		Type:   PvcReadyConditionType,
		Status: conditionStatus,
		Reason: string(pvc.Status.Phase),
	})

	return &workloadStatus, conditions, nil
}
