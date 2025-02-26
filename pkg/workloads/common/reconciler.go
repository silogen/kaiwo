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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcilerBase[T client.Object] struct {
	ObjectKey client.ObjectKey
	Object    T
	Self      Reconciler[T]
}

func (r *ReconcilerBase[T]) GetManifests(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme) ([]client.Object, error) {
	_, manifests, err := r.Self.Reconcile(ctx, k8sClient, scheme, true)
	if err != nil {
		return nil, err
	}
	return manifests, nil
}

// Reconciler manages the reconciliation of a Kaiwo resource (job or service)
type Reconciler[T client.Object] interface {
	GetManifests(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme) ([]client.Object, error)

	// Reconcile runs through the reconciliation loop
	Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, dryRun bool) (ctrl.Result, []client.Object, error)
}

type ResourceReconcilerBase[T client.Object] struct {
	ObjectKey client.ObjectKey
	Self      ResourceReconciler[T]
}

func (d *ResourceReconcilerBase[T]) Create(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, desired T, owner client.Object) error {
	if owner != nil {
		if err := ctrl.SetControllerReference(owner, desired, scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	}
	return k8sClient.Create(ctx, desired)
}

// Update updates the object. By default, nothing is done, and desired is set to actual
func (d *ResourceReconcilerBase[T]) Update(_ context.Context, _ client.Client, desired *T, actual T) error {
	*desired = actual
	return nil
}

// Reconcile ensures that the resource exists and is in the desired state.
// The object is first built based on the reconciler object (kaiwo job or service), and the remote object is fetched.
// If it exists, it is updated, otherwise it is created.
func (d *ResourceReconcilerBase[T]) Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, owner client.Object, dryRun bool) (actual T, result *ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	var empty T // nil or default value
	desired, err := d.Self.Build(ctx, k8sClient)
	if err != nil {
		return empty, nil, fmt.Errorf("failed to build object: %w", err)
	}

	if dryRun {
		return desired, nil, nil
	}

	gvk, err := baseutils.GetGVK(*scheme, desired)
	if err != nil {
		return empty, nil, fmt.Errorf("failed to get GVK: %w", err)
	}
	baseutils.Debug(logger, fmt.Sprintf("Reconciling %s (%s/%s)", gvk.String(), desired.GetNamespace(), desired.GetName()))

	actual, err = d.Self.Get(ctx, k8sClient)
	if err == nil {
		// Object exists

		if !metav1.IsControlledBy(actual, owner) {
			// Ensure the ownership is up-to-date
			logger.Info("Updating object owner")
			if err := ctrl.SetControllerReference(owner, actual, scheme); err != nil {
				logger.Error(err, "Failed to set controller reference on existing object")
				return empty, nil, err
			}

			if err := k8sClient.Update(ctx, actual); err != nil {
				logger.Error(err, "Failed to update existing Job with correct owner reference")
				return empty, nil, err
			}
		}

		err = d.Update(ctx, k8sClient, &desired, actual)
		if err != nil {
			return empty, nil, fmt.Errorf("failed to update object: %w", err)
		}
		actual = desired
	} else if errors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("creating %s (%s/%s)", gvk.String(), desired.GetNamespace(), desired.GetName()))
		// Object doesn't exist
		err = d.Create(ctx, k8sClient, scheme, desired, owner)
		actual = desired
		if err != nil {
			return empty, nil, fmt.Errorf("failed to create object: %w", err)
		}
	} else {
		return empty, nil, fmt.Errorf("failed to get object: %w", err)
	}
	shouldContinueResult := d.Self.ShouldContinue(ctx, actual)
	if shouldContinueResult != nil {
		baseutils.Debug(logger, "Reconciliation was interrupted with an intermediate response")
	}
	return actual, shouldContinueResult, nil
}

func (d *ResourceReconcilerBase[T]) Get(ctx context.Context, k8sClient client.Client) (actual T, err error) {
	var zero T // Zero value of T

	obj := d.Self.GetEmptyObject()

	// Pass obj to k8sClient.Get method
	if err := k8sClient.Get(ctx, d.ObjectKey, obj); err != nil {
		return zero, err
	}

	return obj, nil
}

func (d *ResourceReconcilerBase[T]) ShouldContinue(ctx context.Context, actual T) *ctrl.Result {
	return nil
}

// ResourceReconciler ensures that a single dependent resource is reconciled to a desired state
type ResourceReconciler[T client.Object] interface {
	// Build will build the client object without creating it
	Build(ctx context.Context, k8sClient client.Client) (desired T, err error)

	// Get will fetch the client object
	Get(ctx context.Context, k8sClient client.Client) (actual T, err error)

	// Create will create the client object
	Create(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, desired T, owner client.Object) error

	// Update will update the client object
	Update(ctx context.Context, k8sClient client.Client, desired *T, actual T) error

	// Reconcile will build and then create
	Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, owner client.Object, dryRun bool) (actual T, result *ctrl.Result, err error)

	// ShouldContinue returns a reconciliation result if there is an intermediate result to return
	ShouldContinue(ctx context.Context, actual T) *ctrl.Result

	GetEmptyObject() T
}
