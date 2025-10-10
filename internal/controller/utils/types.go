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

package controllerutils

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectWithStatus is a generic constraint for objects that have a typed status accessor.
// This allows type-safe status access without reflection (except for ObservedGeneration).
type ObjectWithStatus[S any] interface {
	client.Object
	GetStatus() *S
}

// ReconcileSpec defines the callback-based reconciliation specification.
// Controllers provide closures that capture their specific types and logic.
//
// Type parameters:
//   - T: The object type being reconciled (must implement ObjectWithStatus[S])
//   - S: The status type of the object
type ReconcileSpec[T ObjectWithStatus[S], S any] struct {
	// Client is the Kubernetes client for API operations
	Client client.Client

	// Scheme is the runtime scheme for GVK resolution
	Scheme *runtime.Scheme

	// Object is the resource being reconciled (must be fetched and passed in)
	Object T

	// Recorder is the event recorder for emitting Kubernetes events (optional)
	Recorder record.EventRecorder

	// FinalizerName is the finalizer to add/remove (optional)
	FinalizerName string

	// FieldOwner is the field manager name for SSA (required)
	FieldOwner string

	// ObserveFn gathers current cluster state (read-only).
	// Returns typed observation data and any errors encountered.
	ObserveFn func(ctx context.Context) (observation any, err error)

	// PlanFn computes desired state from spec + observation (pure function).
	// Returns slice of objects to apply via SSA.
	PlanFn func(ctx context.Context, obs any) (desired []client.Object, err error)

	// ProjectFn computes status from observation + reconcile errors.
	// Modifies Object.Status directly. Framework patches if changed.
	ProjectFn func(ctx context.Context, obs any, errs ReconcileErrors) error

	// FinalizeFn performs external cleanup during deletion (optional).
	// Should only interact with external systems, not owned children.
	FinalizeFn func(ctx context.Context, obs any) error
}

// ReconcileErrors captures errors from different reconciliation phases
type ReconcileErrors struct {
	ObserveErr  error
	PlanErr     error
	ApplyErr    error
	FinalizeErr error
}

// HasError returns true if any error is set
func (e ReconcileErrors) HasError() bool {
	return e.ObserveErr != nil || e.PlanErr != nil || e.ApplyErr != nil || e.FinalizeErr != nil
}

// ApplyConfig configures the SSA apply operation
type ApplyConfig struct {
	// FieldOwner for SSA
	FieldOwner string

	// EnablePruning enables deletion of previously-owned objects not in desired set
	EnablePruning bool

	// InventoryLabels are labels used to track owned objects for pruning
	InventoryLabels map[string]string
}

// ReconcileResult encapsulates the result of a reconciliation
type ReconcileResult struct {
	Result ctrl.Result
	Error  error
}

// ReconcileWithoutStatusSpec defines the reconciliation specification for
// controllers that don't need status updates (e.g., derived resource controllers).
type ReconcileWithoutStatusSpec struct {
	// Client is the Kubernetes client for API operations
	Client client.Client

	// Scheme is the runtime scheme for GVK resolution
	Scheme *runtime.Scheme

	// FieldOwner is the field manager name for SSA (required)
	FieldOwner string

	// ObserveFn gathers current cluster state (read-only).
	// Returns typed observation data and any errors encountered.
	ObserveFn func(ctx context.Context) (observation any, err error)

	// PlanFn computes desired state from observation (pure function).
	// Returns slice of objects to apply via SSA.
	PlanFn func(ctx context.Context, obs any) (desired []client.Object, err error)
}
