# Controller Reconciliation Framework

This document explains the reconciliation framework used by AIM controllers in `internal/controller/aim/framework/`. This framework provides a structured, testable approach to implementing Kubernetes controllers using the observe-plan-apply pattern with explicit status projection.

**Target Audience**: Developers experienced with Kubernetes controllers and controller-runtime who want to understand this framework's conventions and implement new controllers following the same patterns.

## Relationship to controller-runtime

This framework builds on top of `sigs.k8s.io/controller-runtime` rather than replacing it. Standard controller-runtime concepts like the `Reconcile()` method, manager setup, watches, and predicates remain unchanged. What this framework provides is a structured implementation of the `Reconcile()` method itself.

Without this framework, a typical controller-runtime reconciler implements `Reconcile(ctx, req)` as a monolithic function that reads state, makes decisions, applies changes, updates status, and handles errors all in one flow. This works but has testability and maintainability challenges: testing requires mocking the entire Kubernetes API, business logic mixes with infrastructure concerns, and error handling becomes complex with multiple failure paths.

This framework decomposes reconciliation into four distinct phases implemented as separate functions: **observe** (read cluster state), **plan** (compute desired state), **apply** (use Server-Side Apply), and **project** (compute status updates). These phases are provided to the framework as closures that capture controller-specific types and logic. The framework handles the orchestration, error accumulation, finalizer management, status patching with retry logic, and logging.

The key benefit is separation of concerns: your controller implements pure functions for planning and status projection while the framework handles Kubernetes API interactions, SSA mechanics, and error handling patterns. This makes controllers easier to test (mock simple data structures instead of Kubernetes clients), easier to understand (each function has a single responsibility), and more consistent (all controllers follow the same flow).

## Design Philosophy & Rationale

### Why Closures Instead of Interfaces

The framework uses closures (`func(ctx context.Context) (any, error)`) rather than interface methods for observe/plan/project functions. This decision reflects several practical considerations.

Closures allow each controller to define its own strongly-typed observation structure without the framework needing to know about it. The `observe` function returns `any` to the framework but is actually `*clusterTemplateObservation` or `*namespaceTemplateObservation` internally. The controller then type-asserts back to the concrete type in its `plan` and `project` closures. This approach provides type safety within the controller without forcing the framework to be generic over every possible observation type.

Interface-based approaches would require either losing type safety (interface{} everywhere) or complex generics that make the framework harder to understand and maintain. The closure approach is idiomatic Go: small functions that capture their context rather than large objects implementing interfaces.

### Separate Observe, Plan, and Project Functions

Reconciliation is conceptually three separate concerns, each with different characteristics:

**Observation** is I/O-bound, read-only, and gathers facts about current cluster state. It should never modify anything or make decisions. Separating observation makes it clear what data the controller depends on and allows you to test planning logic by constructing observation structures directly without a cluster.

**Planning** is pure computation that transforms spec + observation into desired state. It should not perform I/O, read from the API server, or have side effects. This purity makes planning trivially testable (call the function with different observations and verify the output) and easy to reason about (same inputs always produce same outputs). Planning can return empty desired state when prerequisites aren't met—the framework handles this gracefully.

**Status projection** computes what status should be based on observation and any errors that occurred during reconciliation. Separating this from the reconciliation flow ensures status accurately reflects what was observed and attempted, not what we hoped would happen. Status projection sees the reconciliation errors and can set appropriate conditions like `Failure` when apply failed or `Progressing` when discovery is running.

### Error Accumulation Pattern

The framework accumulates errors from each phase into a `ReconcileErrors` structure rather than short-circuiting on the first error. This approach ensures status updates reflect what actually happened.

If the observe phase fails, the framework still calls project with the (possibly nil) observation and the observe error. The status projection can then set a `Failure` condition explaining what went wrong. If observation succeeds but planning fails, we project status showing plan failure. This means the resource's status always updates even during failures, giving users visibility into what's wrong.

The alternative—returning immediately on first error—would leave status stale. Users wouldn't see updated conditions explaining current failures. The error accumulation pattern ensures every reconciliation attempts to update status with current information.

### Server-Side Apply (SSA) for Declarative Management

The framework applies desired state using Kubernetes Server-Side Apply rather than create/update logic. SSA is declarative: you tell Kubernetes what fields you own and what their values should be. Kubernetes handles conflicts, partial updates, and field ownership tracking.

This eliminates complex diff logic, handles concurrent modifications gracefully, and makes controllers idempotent by default. The framework sets a consistent field owner for all applies, ensuring the controller owns its declared fields. Future framework enhancements will add pruning support to delete previously-owned resources no longer in the desired set.

## Core Concepts

### The Reconcile Spec

Controllers call `framework.Reconcile(ctx, spec)` where `spec` is a `ReconcileSpec` structure containing:

- **Client, Scheme**: Standard Kubernetes client and scheme for API operations
- **Object**: The resource being reconciled (already fetched by controller)
- **Recorder**: Event recorder for emitting Kubernetes events (optional)
- **FinalizerName**: Finalizer to add/remove (optional, leave empty if no cleanup needed)
- **FieldOwner**: Field manager name for Server-Side Apply (required, identifies this controller)
- **ObserveFn**: Closure that gathers current cluster state
- **PlanFn**: Closure that computes desired state from spec and observation
- **ProjectFn**: Closure that computes status updates from observation and errors
- **FinalizeFn**: Closure for external cleanup during deletion (optional)

### Reconciliation Flow

The framework executes this sequence for every reconciliation:

1. **Ensure Finalizer**: If `FinalizerName` is set and object is not being deleted, add the finalizer (if not already present) and requeue
2. **Handle Deletion**: If object is being deleted, jump to deletion path (observe → finalize → remove finalizer → project status → exit)
3. **Observe**: Call `ObserveFn` to gather current cluster state
4. **Short-circuit on Observation Failure**: If observe fails, skip to status projection with the error
5. **Plan**: Call `PlanFn` with observation to compute desired state
6. **Apply**: If planning succeeded and desired state is non-empty, apply via SSA
7. **Project Status**: Call `ProjectFn` with observation and accumulated errors
8. **Patch Status**: Update status subresource with retry on conflicts
9. **Return**: Return appropriate requeue policy based on errors

This flow ensures every reconciliation attempts to update status, even when earlier phases fail. Errors are accumulated and passed to status projection, ensuring status reflects actual state and any failures encountered.

### Deletion Path

When an object has a deletion timestamp, the framework follows the deletion path:

1. **Observe**: Gather current state for finalization context
2. **Finalize**: If `FinalizeFn` is provided, call it to perform external cleanup
3. **Remove Finalizer**: Remove the finalizer so Kubernetes can delete the object
4. **Project Status**: Compute final status (often including deletion conditions)
5. **Patch Status**: Update status one last time before object is deleted

The finalization function should only interact with external systems (delete cloud resources, clean up webhooks, etc.). It should not delete owned child resources—those are automatically garbage-collected by Kubernetes through owner references.

## Status Management

Status updates deserve special attention due to their importance for user visibility and their potential to cause reconciliation loops.

### Status Update Flow

The framework handles status updates in `PatchStatus()` with this flow:

1. **Retry on Conflicts**: Wraps the update in `retry.RetryOnConflict()` with exponential backoff
2. **Fetch Latest**: On each retry attempt, fetches the latest version of the object to avoid stale reads
3. **Reflect Changes**: Uses reflection to access the `Status` field and apply updates
4. **Merge Conditions**: Uses `meta.SetStatusCondition()` which properly handles `LastTransitionTime` (only updates when condition status changes)
5. **Set ObservedGeneration**: Automatically sets `ObservedGeneration` to match `metadata.generation`
6. **Detect Changes**: Compares original and updated status with `DeepEqual` to skip unnecessary updates
7. **Update Subresource**: If changes detected, calls `client.Status().Update()`

This design handles concurrent modifications (conflict errors), avoids unnecessary API calls (skip update if nothing changed), and prevents reconciliation loops (only update `LastTransitionTime` on actual state transitions).

### Status Update Patterns

When implementing `ProjectFn`, follow these patterns:

**Handle nil observations**: If observe failed, the observation may be nil or partially populated. Status projection must handle this gracefully:

```go
func (r *MyReconciler) projectStatus(ctx context.Context, resource *myv1.Resource, obs *myObservation, errs framework.ReconcileErrors) (framework.StatusUpdate, error) {
    if errs.ObserveErr != nil {
        return framework.StatusUpdate{
            Conditions: []metav1.Condition{{
                Type:    "Ready",
                Status:  metav1.ConditionFalse,
                Reason:  "ObservationFailed",
                Message: errs.ObserveErr.Error(),
            }},
            StatusField: "Status",
            StatusValue: myv1.StatusFailed,
        }, nil
    }

    // Now safe to use obs fields
    if obs.SomeField == nil {
        // Handle missing data
    }
}
```

**Return early for unchanged status**: If the current status already reflects the desired state (especially for `Available` resources), return an empty update:

```go
if currentStatus == aimv1alpha1.AIMTemplateStatusAvailable {
    // Template already available, no status changes needed
    return framework.StatusUpdate{}, nil
}
```

**Use additional fields for complex status**: Beyond conditions and the status enum, you can update arbitrary status fields:

```go
return framework.StatusUpdate{
    Conditions: conditions,
    StatusField: "Status",
    StatusValue: status,
    AdditionalFields: map[string]any{
        "ModelSources": modelSources,
        "Profile":      profile,
    },
}
```

## Error Handling

The framework accumulates errors from each phase into `ReconcileErrors` and passes them to status projection. This ensures status reflects what actually happened during reconciliation.

### Error Propagation

Errors are captured at each phase:

- **ObserveErr**: Error from `ObserveFn`, indicates failure to read cluster state
- **PlanErr**: Error from `PlanFn`, indicates failure to compute desired state
- **ApplyErr**: Error from applying desired state via SSA
- **FinalizeErr**: Error from `FinalizeFn` during deletion

When observe fails, the framework short-circuits to status projection without calling plan or apply. When plan fails, apply is skipped. In all cases, status projection sees the accumulated errors and can set appropriate conditions.

### Status Projection with Errors

Your `ProjectFn` should inspect `errs` and set conditions accordingly:

```go
func (r *MyReconciler) projectStatus(ctx context.Context, resource *myv1.Resource, obs *myObservation, errs framework.ReconcileErrors) (framework.StatusUpdate, error) {
    var conditions []metav1.Condition
    var status myv1.ResourceStatus

    // Check for errors first
    if errs.HasError() {
        status = myv1.StatusFailed

        if errs.ObserveErr != nil {
            conditions = append(conditions, metav1.Condition{
                Type:    "Failure",
                Status:  metav1.ConditionTrue,
                Reason:  "ObservationFailed",
                Message: fmt.Sprintf("Failed to observe: %v", errs.ObserveErr),
            })
        }

        if errs.ApplyErr != nil {
            conditions = append(conditions, metav1.Condition{
                Type:    "Failure",
                Status:  metav1.ConditionTrue,
                Reason:  "ApplyFailed",
                Message: fmt.Sprintf("Failed to apply: %v", errs.ApplyErr),
            })
        }

        return framework.StatusUpdate{
            Conditions:  conditions,
            StatusField: "Status",
            StatusValue: status,
        }, nil
    }

    // No errors, proceed with normal status projection based on observation
    // ...
}
```

This pattern ensures users see meaningful error information in status conditions rather than generic failure messages.

### When to Return Errors vs Set Status

Generally, return errors from `ProjectFn` only for truly exceptional cases where status projection itself failed (not when reconciliation failed). Most reconciliation failures should be reflected in status conditions, not returned as errors.

Returning an error from observe, plan, or project causes the framework to return that error from `Reconcile()`, which triggers controller-runtime's exponential backoff retry. This is appropriate for transient failures like API server timeouts. Persistent failures (missing resources, invalid configuration) should be reflected in status, not returned as errors.

## Deletion and Finalization

Finalizers allow controllers to perform cleanup when resources are deleted. The framework handles finalizer management automatically when you provide a `FinalizerName`.

### Finalizer Lifecycle

When `FinalizerName` is set:

1. **On create/update**: Framework adds the finalizer if not present, then requeues
2. **During normal reconciliation**: Finalizer remains present
3. **On deletion request**: Kubernetes sets `DeletionTimestamp` but doesn't delete the object (blocked by finalizer)
4. **During deletion reconciliation**: Framework calls `FinalizeFn`, then removes finalizer
5. **After finalizer removal**: Kubernetes deletes the object

### What to Finalize

The `FinalizeFn` should perform external cleanup only: calling external APIs, deleting cloud resources, revoking webhooks, cleaning up non-Kubernetes state. Do **not** delete owned child resources (resources with owner references to your resource). Kubernetes automatically garbage-collects those when the owner is deleted.

Example appropriate finalization:

```go
func (r *MyReconciler) finalize(ctx context.Context, resource *myv1.Resource, obs *myObservation) error {
    // Good: call external service to clean up
    if err := externalService.DeleteResource(resource.Spec.ExternalID); err != nil {
        return fmt.Errorf("failed to delete external resource: %w", err)
    }
    return nil
}
```

Example **inappropriate** finalization:

```go
func (r *MyReconciler) finalize(ctx context.Context, resource *myv1.Resource, obs *myObservation) error {
    // Bad: don't manually delete owned Jobs - Kubernetes does this automatically
    if obs.Job != nil {
        if err := r.Delete(ctx, obs.Job); err != nil {
            return err
        }
    }
    return nil
}
```

### Finalization Failures

If `FinalizeFn` returns an error, the framework:

1. Projects status including the finalization error
2. Patches status to reflect the error
3. Returns the error to trigger retry with exponential backoff

The finalizer remains in place until finalization succeeds. This prevents Kubernetes from deleting the object while external cleanup is incomplete.

## Anti-Patterns and Gotchas

### Anti-Pattern: Modifying Observation Data in Plan

The observation structure should be treated as read-only by the plan function. Modifying observation data can lead to subtle bugs where status projection sees different data than planning saw.

**Bad**:
```go
func (r *MyReconciler) plan(ctx context.Context, resource *myv1.Resource, obs *myObservation) ([]client.Object, error) {
    obs.Job = nil  // Bad: modifying observation
    return buildDesiredState(resource, obs), nil
}
```

**Good**:
```go
func (r *MyReconciler) plan(ctx context.Context, resource *myv1.Resource, obs *myObservation) ([]client.Object, error) {
    // Make decisions based on observation but don't modify it
    if obs.Job != nil && jobFailed(obs.Job) {
        // Plan for new job without modifying obs
        return buildDesiredState(resource, obs), nil
    }
    return nil, nil
}
```

### Anti-Pattern: Performing I/O in Plan Function

The plan function should be pure computation. Performing API calls, reading files, or other I/O violates this contract and makes the function hard to test.

**Bad**:
```go
func (r *MyReconciler) plan(ctx context.Context, resource *myv1.Resource, obs *myObservation) ([]client.Object, error) {
    // Bad: calling API during planning
    var secret corev1.Secret
    if err := r.Get(ctx, client.ObjectKey{Name: "my-secret"}, &secret); err != nil {
        return nil, err
    }
    return buildDesiredState(resource, secret), nil
}
```

**Good**:
```go
func (r *MyReconciler) observe(ctx context.Context, resource *myv1.Resource) (*myObservation, error) {
    obs := &myObservation{}

    // Good: fetch secret during observation
    var secret corev1.Secret
    if err := r.Get(ctx, client.ObjectKey{Name: "my-secret"}, &secret); err != nil {
        return nil, err
    }
    obs.Secret = &secret

    return obs, nil
}

func (r *MyReconciler) plan(ctx context.Context, resource *myv1.Resource, obs *myObservation) ([]client.Object, error) {
    // Good: use observed data
    if obs.Secret == nil {
        return nil, nil
    }
    return buildDesiredState(resource, obs.Secret), nil
}
```

### Anti-Pattern: Not Handling Nil Observations

When observe fails, observation may be nil. Status projection must handle this gracefully.

**Bad**:
```go
func (r *MyReconciler) projectStatus(ctx context.Context, resource *myv1.Resource, obs *myObservation, errs framework.ReconcileErrors) (framework.StatusUpdate, error) {
    // Bad: obs might be nil if observe failed
    if obs.Job == nil {
        // Panic if obs is nil!
    }
}
```

**Good**:
```go
func (r *MyReconciler) projectStatus(ctx context.Context, resource *myv1.Resource, obs *myObservation, errs framework.ReconcileErrors) (framework.StatusUpdate, error) {
    // Good: check for observe errors first
    if errs.ObserveErr != nil {
        return framework.StatusUpdate{
            Conditions: []metav1.Condition{{
                Type:    "Ready",
                Status:  metav1.ConditionFalse,
                Reason:  "ObservationFailed",
                Message: errs.ObserveErr.Error(),
            }},
            StatusField: "Status",
            StatusValue: myv1.StatusFailed,
        }, nil
    }

    // Now safe to access obs fields (though still should check for nil on individual fields)
    if obs.Job == nil {
        // Handle missing job
    }
}
```

### Anti-Pattern: Forgetting Status Checks for Idempotency

Plan functions should check current status to avoid recreating resources that have already completed their lifecycle.

**Bad**:
```go
func (r *TemplateReconciler) plan(ctx context.Context, template *Template, obs *templateObservation) ([]client.Object, error) {
    var desired []client.Object

    // Bad: always creates discovery job, even after discovery completes
    if obs.Job == nil {
        job := buildDiscoveryJob(template)
        desired = append(desired, job)
    }

    return desired, nil
}
```

**Good**:
```go
func (r *TemplateReconciler) plan(ctx context.Context, template *Template, obs *templateObservation) ([]client.Object, error) {
    var desired []client.Object

    // Good: check if discovery already completed
    if template.Status.Status != StatusAvailable {
        if obs.Job == nil || !isJobComplete(obs.Job) {
            job := buildDiscoveryJob(template)
            desired = append(desired, job)
        }
    }

    return desired, nil
}
```

This prevents recreating short-lived resources (like discovery jobs with TTL) after they complete and get garbage-collected.

### Gotcha: Type Assertions Must Match Observation Type

The closures type-assert observation data back to the concrete type. If you change the observation type, update all assertions.

```go
type oldObservation struct { /* ... */ }
type newObservation struct { /* ... */ }  // Changed observation structure

// Don't forget to update all type assertions!
PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
    return r.plan(ctx, &template, obs.(*newObservation))  // Updated
},

ProjectFn: func(ctx context.Context, obs any, errs framework.ReconcileErrors) (framework.StatusUpdate, error) {
    return r.projectStatus(ctx, &template, obs.(*newObservation), errs)  // Updated
},
```

### Gotcha: Empty Desired State Is Valid

Returning an empty slice from plan is perfectly valid and means "nothing to apply". This is appropriate when prerequisites are missing.

```go
func (r *MyReconciler) plan(ctx context.Context, resource *myv1.Resource, obs *myObservation) ([]client.Object, error) {
    var desired []client.Object

    // If prerequisite missing, return empty desired state
    if obs.ConfigMap == nil {
        return desired, nil  // Valid: nothing to create yet
    }

    // Otherwise build desired state
    job := buildJob(resource, obs.ConfigMap)
    desired = append(desired, job)
    return desired, nil
}
```

The status projection should detect missing prerequisites and set appropriate status/conditions.

## Framework Evolution and Roadmap

The reconciliation framework is functional but has areas marked for future enhancement. Understanding the current limitations and planned improvements helps when designing new controllers.

### Current Limitations

**Pruning support is not implemented** (marked as TODO in `apply.go`). The framework currently only creates or updates resources via SSA but doesn't delete resources that were previously owned but are no longer in the desired set. Controllers must implement their own cleanup logic or rely on garbage collection via owner references.

**Event emission is manual**. While the framework provides `EmitEvent()` helper functions, controllers must explicitly call them. Future versions may integrate event emission into the reconciliation flow, automatically emitting events on phase transitions or errors.

**Status conflict retry is basic**. The current implementation uses `retry.DefaultRetry` with standard exponential backoff. Highly contested resources might benefit from more sophisticated retry strategies or optimistic locking patterns.

**No built-in caching layer**. Controllers fetch observations on every reconciliation. For expensive lookups (listing many resources, calling external APIs), controllers should implement their own caching strategies in the observe function.

## Summary

The reconciliation framework provides a structured approach to controller implementation through the observe-plan-apply-status pattern:

- **Observe** gathers current state through read-only API calls
- **Plan** computes desired state as a pure function of spec and observation
- **Apply** uses Server-Side Apply to declaratively manage resources
- **Project** computes status updates from observation and accumulated errors

This separation of concerns makes controllers testable, maintainable, and consistent. Status updates include retry logic and change detection to prevent reconciliation loops. Error accumulation ensures status reflects actual reconciliation results even when phases fail.

When implementing new controllers, follow the example pattern: define a typed observation structure, implement pure planning logic, handle nil observations in status projection, and check current status for idempotency. Avoid I/O in planning, don't modify observations, and use owner references for automatic garbage collection of child resources.

The framework handles the mechanical aspects of reconciliation (SSA, finalizers, status patching, error handling) so you can focus on your controller's business logic.
