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

package storage

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/silogen/kaiwo/pkg/api"

	"github.com/silogen/kaiwo/pkg/observe"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (r *PvcReconciler) BuildDesired(ctx context.Context, clusterCtx api.ClusterContext) (client.Object, error) {
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

func (r *PvcReconciler) MutateActual(ctx context.Context, clusterCtx api.ClusterContext, actual client.Object) error {
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

// PVCObserver observes PVC status
type PVCObserver struct {
	Identified observe.Identified
}

func NewPVCObserver(nn types.NamespacedName, group observe.UnitGroup) *PVCObserver {
	return &PVCObserver{
		Identified: observe.Identified{
			NamespacedName: nn,
			Group:          group,
		},
	}
}

func (o *PVCObserver) Kind() string {
	return "PersistentVolumeClaim"
}

func (o *PVCObserver) Observe(ctx context.Context, c client.Client) (observe.UnitStatus, error) {
	var pvc corev1.PersistentVolumeClaim
	if err := c.Get(ctx, o.Identified.GetNamespacedName(), &pvc); apierrors.IsNotFound(err) {
		return observe.UnitStatus{Phase: observe.UnitPending}, nil
	} else if err != nil {
		return observe.UnitStatus{
			Phase:   observe.UnitUnknown,
			Reason:  observe.ReasonGetError,
			Message: err.Error(),
		}, nil
	}

	// Deleting
	if pvc.DeletionTimestamp != nil {
		return observe.UnitStatus{
			Phase:  observe.UnitProgressing,
			Reason: observe.ReasonDeleting,
		}, nil
	}

	conditions := convertPvcConditions(pvc.Status.Conditions)

	fsResizePending := meta.IsStatusConditionTrue(conditions, string(corev1.PersistentVolumeClaimFileSystemResizePending)) &&
		(pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode == corev1.PersistentVolumeFilesystem)

	// Compare requested vs actual capacity (values, not pointers)
	reqQty, haveReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	capQty, haveCap := pvc.Status.Capacity[corev1.ResourceStorage]
	capBelowRequest := haveReq && haveCap && capQty.Cmp(reqQty) < 0

	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		// Bound but not fully “done” yet (resize in progress)
		if capBelowRequest || fsResizePending {
			var msg string
			switch {
			case capBelowRequest && fsResizePending:
				msg = "PVC capacity below requested and filesystem resize pending"
			case capBelowRequest:
				msg = "PVC capacity below requested size"
			case fsResizePending:
				msg = "PVC filesystem resize pending"
			}
			return observe.UnitStatus{
				Phase:      observe.UnitProgressing,
				Reason:     observe.ReasonPvcResizing,
				Message:    msg,
				Conditions: conditions,
			}, nil
		}
		// Fully usable
		return observe.UnitStatus{
			Phase:      observe.UnitReady,
			Ready:      true,
			Conditions: conditions,
		}, nil

	case corev1.ClaimPending:
		reason := observe.ReasonPVCPending
		msg := "PVC pending; waiting for volume binding/provisioning"
		if pvc.Spec.VolumeName != "" {
			msg = "PVC pending"
		}
		return observe.UnitStatus{
			Phase:      observe.UnitPending,
			Reason:     reason,
			Message:    msg,
			Conditions: conditions,
		}, nil

	case corev1.ClaimLost:
		return observe.UnitStatus{
			Phase:      observe.UnitFailed,
			Reason:     observe.ReasonPVCLost,
			Message:    "PVC is lost",
			Conditions: conditions,
		}, nil

	default:
		return observe.UnitStatus{
			Phase:      observe.UnitUnknown,
			Reason:     observe.ReasonUnknownPhase,
			Message:    fmt.Sprintf("Unknown PVC phase: %s", pvc.Status.Phase),
			Conditions: conditions,
		}, nil
	}
}

func convertPvcConditions(conditions []corev1.PersistentVolumeClaimCondition) []metav1.Condition {
	var result []metav1.Condition
	for _, c := range conditions {
		result = append(result, metav1.Condition{
			Type:               string(c.Type),
			Status:             metav1.ConditionStatus(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			LastTransitionTime: c.LastTransitionTime,
		})
	}
	return result
}
