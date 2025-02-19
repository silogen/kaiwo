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

package workloadshared

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	DataStoragePostfix = "data"
	HfStoragePostfix   = "hf"
)

type StorageCommand[T any] struct {
	workloadutils.CommandBase[T]
	PvcNamePostfix   string
	Amount           string
	StorageClassName string
	AccessMode       corev1.PersistentVolumeAccessMode
	StoreCallback    func(base *T, claim *corev1.PersistentVolumeClaim, target string)
}

func NewStorageCommand[T any](
	base workloadutils.CommandBase[T],
	pvcNamePostfix string,
	accessMode corev1.PersistentVolumeAccessMode,
	storageClassName string,
	amount string,
	storeCallback func(base *T, claim *corev1.PersistentVolumeClaim, target string),
) *StorageCommand[T] {
	cmd := &StorageCommand[T]{
		PvcNamePostfix:   pvcNamePostfix,
		CommandBase:      base,
		AccessMode:       accessMode,
		StorageClassName: storageClassName,
		Amount:           amount,
		StoreCallback:    storeCallback,
	}
	cmd.Self = cmd
	return cmd
}

func (cmd *StorageCommand[T]) Build(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	// logger := log.FromContext(ctx)

	// logger.Info(fmt.Sprintf("Building with amount: %s, access mode: %s", cmd.Amount, cmd.AccessMode))

	objectKey := cmd.GetObjectKey()

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				cmd.AccessMode,
			},
			StorageClassName: &cmd.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(cmd.Amount),
				},
			},
		},
	}

	cmd.StoreCallback(cmd.State, pvc, cmd.PvcNamePostfix)
	return pvc, nil
}

func (cmd *StorageCommand[T]) GetEmptyObject() client.Object {
	return &corev1.PersistentVolumeClaim{}
}

func (cmd *StorageCommand[T]) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: cmd.Owner.GetNamespace(),
		Name:      baseutils.FormatNameWithPostfix(cmd.Owner.GetName(), cmd.PvcNamePostfix),
	}
}

func SetStorage(state *workloadutils.CommandStateBase, pvc *corev1.PersistentVolumeClaim, target string) {
	if target == DataStoragePostfix {
		state.DataPvc = pvc
	} else if target == HfStoragePostfix {
		state.HfPvc = pvc
	} else {
		panic(fmt.Sprintf("unknown target %s", target))
	}
}
