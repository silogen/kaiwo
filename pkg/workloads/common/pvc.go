/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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
