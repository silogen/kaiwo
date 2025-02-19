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
	"fmt"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CommandBase[T any] struct {
	// Desired is the object which would be created by running this command
	Desired client.Object

	// Actual is the object that exists on in the cluster
	Actual client.Object

	// Owner is object that owns the object that would be created by this command.
	Owner client.Object

	// State holds a shared state struct that each command can use
	State *T

	// Client client.Client

	// Context context.Context
	Scheme *runtime.Scheme

	// Self holds a reference to the concrete command implementing Command.
	Self Command
}

func (cb *CommandBase[T]) GetGVK(scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	var obj client.Object
	if cb.Desired != nil {
		obj = cb.Desired
	} else {
		if cb.Self == nil {
			return schema.GroupVersionKind{}, fmt.Errorf("self is not set in CommandBase")
		}
		obj = cb.Self.GetEmptyObject()
		if obj == nil {
			return schema.GroupVersionKind{}, fmt.Errorf("GetEmptyObject returned nil")
		}
	}

	return baseutils.GetGVK(*scheme, obj)
}

func (cb *CommandBase[T]) Get(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	obj := cb.Self.GetEmptyObject()
	if err := k8sClient.Get(ctx, cb.Self.GetObjectKey(), obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (cb *CommandBase[T]) SetDesired(object client.Object) {
	cb.Desired = object
}

func (cb *CommandBase[T]) SetActual(object client.Object) {
	cb.Actual = object
}

func (cb *CommandBase[T]) Create(ctx context.Context, k8sClient client.Client) error {
	log.FromContext(ctx).Info("Creating object")
	toCreate := cb.Desired.DeepCopyObject().(client.Object)

	if err := k8sClient.Create(ctx, toCreate); err != nil {
		return err
	}
	return nil
}

func (cb *CommandBase[T]) Update(ctx context.Context, _ client.Client) error {
	// logger := log.FromContext(ctx)
	// logger.Info("Object exists, skipping update")
	return nil
}

func (cb *CommandBase[T]) ResourceExists(ctx context.Context, k8sClient client.Client) (bool, error) {
	existingObject := cb.Desired.DeepCopyObject().(client.Object)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cb.Desired), existingObject); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	cb.Actual = existingObject
	return true, nil
}

func (cb *CommandBase[T]) GetOwner() client.Object {
	return cb.Owner
}

func (cb *CommandBase[T]) GetCurrentReconcileResult(context.Context) *ctrl.Result {
	return nil
}

type Command interface {
	// Build creates the Kubernetes manifest that this command would create. If nil is returned, assuming nothing would be created
	Build(ctx context.Context, k8sClient client.Client) (client.Object, error)

	SetDesired(object client.Object)

	SetActual(object client.Object)

	// ResourceExists checks if the resource exists. Must be run after Build().
	// TODO refactor
	ResourceExists(ctx context.Context, k8sClient client.Client) (bool, error)

	// Create creates the object inside the Kubernetes cluster. Must be run after Build().
	Create(ctx context.Context, k8sClient client.Client) error

	// Update updates the object inside the Kubernetes cluster. Must be run after Build().
	Update(ctx context.Context, k8sClient client.Client) error

	// GetOwner returns the object that owns the object that would be created by this command.
	GetOwner() client.Object

	// GetCurrentReconcileResult returns an optional reconciliation result, which can be used to force a reconciliation
	// request in the middle of invoking a list of commands, in which case the rest of the commands do not get invoked.
	GetCurrentReconcileResult(ctx context.Context) *ctrl.Result

	GetObjectKey() client.ObjectKey

	GetEmptyObject() client.Object

	GetGVK(*runtime.Scheme) (schema.GroupVersionKind, error)
}
