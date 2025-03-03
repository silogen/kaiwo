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

package workloadcommon

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

const (
	DefaultLocalQueueName = "kaiwo"
	Finalizer             = "kaiwo.silogen.ai/finalizer"
)

type LocalQueueReconciler struct {
	ResourceReconcilerBase[*kueuev1beta1.LocalQueue]
}

func NewLocalQueueReconciler(objectKey client.ObjectKey) *LocalQueueReconciler {
	reconciler := &LocalQueueReconciler{
		ResourceReconcilerBase: ResourceReconcilerBase[*kueuev1beta1.LocalQueue]{
			ObjectKey: objectKey,
		},
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *LocalQueueReconciler) Build(_ context.Context, _ client.Client) (*kueuev1beta1.LocalQueue, error) {
	objectKey := r.ObjectKey
	return &kueuev1beta1.LocalQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
		Spec: kueuev1beta1.LocalQueueSpec{
			ClusterQueue: kueuev1beta1.ClusterQueueReference(r.ObjectKey.Name),
		},
	}, nil
}

func (r *LocalQueueReconciler) GetEmptyObject() *kueuev1beta1.LocalQueue {
	return &kueuev1beta1.LocalQueue{}
}
