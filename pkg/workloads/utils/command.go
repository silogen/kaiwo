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

package workloadutils

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CommandBase[T any] struct {
	// State holds a shared state struct that each command can use
	State *T

	// Self holds a reference to the concrete command implementing Command.
	Self Command

	OwnerName string
	Namespace string
}

func (cb *CommandBase[T]) Get(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	obj := cb.Self.GetEmptyObject()
	if err := k8sClient.Get(ctx, cb.Self.GetObjectKey(), obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (cb *CommandBase[T]) Create(ctx context.Context, k8sClient client.Client, obj client.Object) error {
	log.FromContext(ctx).Info("Creating object")
	if err := k8sClient.Create(ctx, obj); err != nil {
		return err
	}
	return nil
}

func (cb *CommandBase[T]) Update(ctx context.Context, _ client.Client, object client.Object) error {
	// logger := log.FromContext(ctx)
	// logger.Info("Object exists, skipping update")
	return nil
}

func (cb *CommandBase[T]) GetCurrentReconcileResult(context.Context, client.Client) (*ctrl.Result, error) {
	return nil, nil
}

func (cb *CommandBase[T]) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: cb.Namespace,
		Name:      cb.Self.GetName(),
	}
}

type Command interface {
	// Build creates the Kubernetes manifest that this command would create. If nil is returned, assuming nothing would be created
	Build(ctx context.Context, k8sClient client.Client) (client.Object, error)

	// Create creates the object inside the Kubernetes cluster. Must be run after Build().
	Create(ctx context.Context, k8sClient client.Client, obj client.Object) error

	// Update updates the object inside the Kubernetes cluster. Must be run after Build().
	Update(ctx context.Context, k8sClient client.Client, obj client.Object) error

	// GetCurrentReconcileResult returns an optional reconciliation result, which can be used to force a reconciliation
	// request in the middle of invoking a list of commands, in which case the rest of the commands do not get invoked.
	GetCurrentReconcileResult(ctx context.Context, k8sClient client.Client) (*ctrl.Result, error)

	GetObjectKey() client.ObjectKey

	GetEmptyObject() client.Object

	GetName() string
}
