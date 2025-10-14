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
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconcile implements the observe → plan → apply → status pattern.
//
// High-level flow:
// 1. Ensure/Remove Finalizer
// 2. If deleting → finalize → remove finalizer → patch status → exit
// 3. Observe (read-only)
// 4. Short-circuit on observation failure → project error status → return
// 5. Plan (pure function)
// 6. Apply (SSA with deterministic ordering)
// 7. Project Status (from observation + errors)
// 8. Patch Status once
// 9. Return with appropriate requeue policy
func Reconcile[T ObjectWithStatus[S], S any](ctx context.Context, spec ReconcileSpec[T, S]) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := spec.Object

	var errs ReconcileErrors

	// Step 1: Ensure finalizer (if specified and not deleting)
	if spec.FinalizerName != "" {
		if obj.GetDeletionTimestamp().IsZero() {
			if !HasFinalizer(obj, spec.FinalizerName) {
				AddFinalizer(obj, spec.FinalizerName)
				if err := spec.Client.Update(ctx, obj); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
				}
				logger.Info("Added finalizer", "finalizer", spec.FinalizerName)
				// Requeue to continue reconciliation
				return ctrl.Result{RequeueAfter: time.Millisecond}, nil
			}
		}
	}

	// Step 2: Handle deletion
	if !obj.GetDeletionTimestamp().IsZero() {
		return handleDeletion(ctx, spec, &errs)
	}

	// Step 3: Observe current state
	obs, observeErr := spec.ObserveFn(ctx)
	errs.ObserveErr = observeErr

	// Step 4: Short-circuit on observation failure
	if observeErr != nil {
		logger.Error(observeErr, "Observation failed")
		originalStatus := cloneStatus(obj)
		if projErr := spec.ProjectFn(ctx, obs, errs); projErr != nil {
			logger.Error(projErr, "Status projection failed after observe error")
			// Still try to patch with error info
		}
		if err := PatchStatus(ctx, spec.Client, obj, originalStatus); err != nil {
			logger.Error(err, "Failed to patch status after observe error")
		}
		return ctrl.Result{}, observeErr
	}

	// Step 5: Plan desired state
	desired, planErr := spec.PlanFn(ctx, obs)
	errs.PlanErr = planErr

	if planErr != nil {
		logger.Error(planErr, "Planning failed")
		// Continue to status projection with error
	}

	// Step 6: Apply desired state (only if plan succeeded)
	var applyErr error
	if planErr == nil && len(desired) > 0 {
		applyConfig := ApplyConfig{
			FieldOwner:      spec.FieldOwner,
			EnablePruning:   false, // TODO: add pruning support later
			InventoryLabels: nil,
		}
		applyErr = ApplyDesiredState(ctx, spec.Client, spec.Scheme, desired, applyConfig)
		errs.ApplyErr = applyErr

		if applyErr != nil {
			logger.Error(applyErr, "Apply failed")
		}
	}

	// Step 7: Project status from observation + errors
	originalStatus := cloneStatus(obj)
	if projErr := spec.ProjectFn(ctx, obs, errs); projErr != nil {
		logger.Error(projErr, "Status projection failed")
		return ctrl.Result{}, projErr
	}

	// Step 8: Patch status once
	if err := PatchStatus(ctx, spec.Client, obj, originalStatus); err != nil {
		logger.Error(err, "Failed to patch status")
		return ctrl.Result{}, err
	}

	// Step 9: Return with requeue policy
	if errs.HasError() {
		// Let controller-runtime backoff handle retries
		if applyErr != nil {
			return ctrl.Result{}, applyErr
		}
		if planErr != nil {
			return ctrl.Result{}, planErr
		}
	}

	return ctrl.Result{}, nil
}

// handleDeletion processes deletion path: finalize → remove finalizer → patch status
func handleDeletion[T ObjectWithStatus[S], S any](ctx context.Context, spec ReconcileSpec[T, S], errs *ReconcileErrors) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := spec.Object

	// Observe for finalization context
	obs, observeErr := spec.ObserveFn(ctx)
	errs.ObserveErr = observeErr

	// Run finalizer if provided
	if spec.FinalizeFn != nil {
		finalizeErr := spec.FinalizeFn(ctx, obs)
		errs.FinalizeErr = finalizeErr

		if finalizeErr != nil {
			logger.Error(finalizeErr, "Finalization failed")
			// Project status with error
			originalStatus := cloneStatus(obj)
			if projErr := spec.ProjectFn(ctx, obs, *errs); projErr != nil {
				logger.Error(projErr, "Status projection failed during finalization")
			}
			if err := PatchStatus(ctx, spec.Client, obj, originalStatus); err != nil {
				logger.Error(err, "Failed to patch status during finalization")
			}
			return ctrl.Result{}, finalizeErr
		}
	}

	// Remove finalizer
	if spec.FinalizerName != "" && HasFinalizer(obj, spec.FinalizerName) {
		RemoveFinalizer(obj, spec.FinalizerName)
		if err := spec.Client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
		logger.Info("Removed finalizer", "finalizer", spec.FinalizerName)
	}

	// Project final status
	originalStatus := cloneStatus(obj)
	if projErr := spec.ProjectFn(ctx, obs, *errs); projErr != nil {
		logger.Error(projErr, "Status projection failed after finalization")
	}
	if err := PatchStatus(ctx, spec.Client, obj, originalStatus); err != nil {
		logger.Error(err, "Failed to patch final status")
	}

	return ctrl.Result{}, nil
}

// ReconcileWithoutStatus implements the observe → plan → apply pattern for
// controllers that maintain derived resources but don't need status updates.
//
// High-level flow:
// 1. Observe (read-only)
// 2. Short-circuit on observation failure
// 3. Plan (pure function)
// 4. Apply (SSA with deterministic ordering)
// 5. Return with appropriate requeue policy
func ReconcileWithoutStatus(ctx context.Context, spec ReconcileWithoutStatusSpec) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Observe current state
	obs, observeErr := spec.ObserveFn(ctx)
	if observeErr != nil {
		logger.Error(observeErr, "Observation failed")
		return ctrl.Result{}, observeErr
	}

	// Step 2: Plan desired state
	desired, planErr := spec.PlanFn(ctx, obs)
	if planErr != nil {
		logger.Error(planErr, "Planning failed")
		return ctrl.Result{}, planErr
	}

	// Step 3: Apply desired state
	if len(desired) > 0 {
		applyConfig := ApplyConfig{
			FieldOwner:      spec.FieldOwner,
			EnablePruning:   false,
			InventoryLabels: nil,
		}
		if applyErr := ApplyDesiredState(ctx, spec.Client, spec.Scheme, desired, applyConfig); applyErr != nil {
			logger.Error(applyErr, "Apply failed")
			return ctrl.Result{}, applyErr
		}
	}

	return ctrl.Result{}, nil
}
