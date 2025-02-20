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

	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"
)

type KaiwoLocalQueueCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	Name      string
	Namespace string
}

func NewKaiwoLocalQueueCommand(base workloadutils.CommandBase[workloadutils.CommandStateBase], name, namespace string) *KaiwoLocalQueueCommand {
	cmd := &KaiwoLocalQueueCommand{
		CommandBase: base,
		Name:        name,
		Namespace:   namespace,
	}
	cmd.Self = cmd
	return cmd
}

func (k *KaiwoLocalQueueCommand) Build(_ context.Context, _ client.Client) (client.Object, error) {
	objectKey := k.GetObjectKey()
	return &kueuev1beta1.LocalQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
		Spec: kueuev1beta1.LocalQueueSpec{
			ClusterQueue: kueuev1beta1.ClusterQueueReference(k.Name),
		},
	}, nil
}

func (k *KaiwoLocalQueueCommand) GetEmptyObject() client.Object {
	return &kueuev1beta1.LocalQueue{}
}

func (k *KaiwoLocalQueueCommand) GetName() string {
	return k.Name
}
