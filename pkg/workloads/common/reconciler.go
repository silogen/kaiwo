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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcilerBase[T client.Object] struct {
	ObjectKey client.ObjectKey
	Object    T
	Self      Reconciler[T]
}

// Reconciler manages the reconciliation of a Kaiwo resource (job or service)
type Reconciler[T client.Object] interface {
	// Reconcile runs through the reconciliation loop
	Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, recorder record.EventRecorder) (ctrl.Result, error)
}

type ResourceReconcilerBase[T client.Object] struct {
	ObjectKey client.ObjectKey
	Self      ResourceReconciler[T]
	Desired   T
}

func (d *ResourceReconcilerBase[T]) Create(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, desired T, owner client.Object, recorder record.EventRecorder) error {
	if owner != nil {
		if err := ctrl.SetControllerReference(owner, desired, scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	}
	gvk, err := apiutil.GVKForObject(desired, scheme)
	if err != nil {
		// fallback if scheme lookup fails
		gvk = desired.GetObjectKind().GroupVersionKind()
	}
	key := client.ObjectKeyFromObject(desired).String()

	if err := k8sClient.Create(ctx, desired); err != nil {
		// The main reconciler function catches this error and emits an event, so no extra event is emitted here
		return err
	}
	recorder.Event(
		owner,
		corev1.EventTypeNormal,
		"CreateSucceeded",
		fmt.Sprintf("Created %s %s", gvk.Kind, key),
	)
	return nil
}

// Update updates the object. By default, nothing is done, and desired is set to actual
func (d *ResourceReconcilerBase[T]) Update(_ context.Context, _ client.Client, desired *T, actual T, recorder record.EventRecorder) error {
	*desired = actual
	return nil
}

// Reconcile ensures that the resource exists and is in the desired state.
// The object is first built based on the reconciler object (kaiwo job or service), and the remote object is fetched.
// If it exists, it is updated, otherwise it is created.
func (d *ResourceReconcilerBase[T]) Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, owner client.Object, recorder record.EventRecorder) (actual T, result *ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	var empty T // nil or default value
	desired, err := d.Self.Build(ctx, k8sClient)
	if err != nil {
		return empty, nil, fmt.Errorf("failed to build object: %w", err)
	}

	d.Desired = desired

	gvk, err := baseutils.GetGVK(*scheme, desired)
	if err != nil {
		return empty, nil, fmt.Errorf("failed to get GVK: %w", err)
	}
	baseutils.Debug(logger, fmt.Sprintf("Reconciling %s (%s/%s)", gvk.String(), desired.GetNamespace(), desired.GetName()))

	actual, err = d.Self.Get(ctx, k8sClient)

	// Check if the reconciliation should be aborted at this point (based on remote object state)
	if intermediateResult, err := d.Self.ValidateBeforeCreateOrUpdate(ctx, actual); err != nil {
		return empty, nil, fmt.Errorf("failed to validate object before create or update: %w", err)
	} else if intermediateResult != nil {
		return empty, intermediateResult, nil
	}

	if err == nil {
		switch any(actual).(type) {
		case *batchv1.Job, *rayv1.RayJob, *rayv1.RayService, *appwrapperv1beta2.AppWrapper, *appsv1.Deployment:
			if !metav1.IsControlledBy(actual, owner) {
				logger.Info("Updating object owner for BatchJob or RayJob", "Object", actual.GetObjectKind().GroupVersionKind().Kind)

				if err := ctrl.SetControllerReference(owner, actual, scheme); err != nil {
					logger.Error(err, "Failed to set controller reference on existing object")
					return empty, nil, err
				}

				// Apply the update to the object in Kubernetes

				retryAttempts := 3
				for i := 0; i < retryAttempts; i++ {
					if err := k8sClient.Update(ctx, actual); err != nil {
						if errors.IsConflict(err) {
							continue
						}
						logger.Error(err, "Failed to update existing Job with correct owner reference")
						return empty, nil, err
					}
				}
			}
		default:
			baseutils.Debug(logger, "Skipping owner reference update since object is neither a BatchJob nor a RayJob", "ObjectType", fmt.Sprintf("%T", actual))
		}

		err = d.Update(ctx, k8sClient, &desired, actual, recorder)
		if err != nil {
			return empty, nil, fmt.Errorf("failed to update object: %w", err)
		}
		actual = desired
	} else if errors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("creating %s (%s/%s)", gvk.String(), desired.GetNamespace(), desired.GetName()))
		// Object doesn't exist
		err = d.Create(ctx, k8sClient, scheme, desired, owner, recorder)
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

func (d *ResourceReconcilerBase[T]) ValidateBeforeCreateOrUpdate(ctx context.Context, actual T) (*ctrl.Result, error) {
	return nil, nil
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
	Create(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, desired T, owner client.Object, recorder record.EventRecorder) error

	// Update will update the client object
	Update(ctx context.Context, k8sClient client.Client, desired *T, actual T, recorder record.EventRecorder) error

	// Reconcile will build and then create
	Reconcile(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, owner client.Object, recorder record.EventRecorder) (actual T, result *ctrl.Result, err error)

	// ShouldContinue returns a reconciliation result if there is an intermediate result to return
	ShouldContinue(ctx context.Context, actual T) *ctrl.Result

	GetEmptyObject() T

	// ValidateBeforeCreateOrUpdate provides a way to abort the reconciliation based on the currently existing object's state
	ValidateBeforeCreateOrUpdate(ctx context.Context, actual T) (*ctrl.Result, error)
}
