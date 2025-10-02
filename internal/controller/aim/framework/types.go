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

package framework

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileSpec defines the callback-based reconciliation specification.
// Controllers provide closures that capture their specific types and logic.
type ReconcileSpec struct {
	// Client is the Kubernetes client for API operations
	Client client.Client

	// Scheme is the runtime scheme for GVK resolution
	Scheme *runtime.Scheme

	// Object is the resource being reconciled (must be fetched and passed in)
	Object client.Object

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

	// ProjectFn computes status from observation + reconcile errors (read-only).
	// Returns status update to patch.
	ProjectFn func(ctx context.Context, obs any, errs ReconcileErrors) (StatusUpdate, error)

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

// StatusUpdate defines what to patch to .status subresource
type StatusUpdate struct {
	// Conditions to set (will be merged with existing)
	Conditions []metav1.Condition

	// StatusField is the high-level status enum field name
	StatusField string

	// StatusValue is the value to set for the status field
	StatusValue any

	// AdditionalFields are extra fields to update on status
	AdditionalFields map[string]any
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
