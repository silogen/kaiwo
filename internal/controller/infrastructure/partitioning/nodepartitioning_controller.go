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

package partitioning

import (
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	nodePartitioningFieldOwner = "node-partitioning-controller"
)

// NodePartitioningReconciler reconciles a NodePartitioning object.
type NodePartitioningReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=nodepartitionings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=nodepartitionings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=nodepartitionings/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *NodePartitioningReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NodePartitioning
	var np infrastructurev1alpha1.NodePartitioning
	if err := r.Get(ctx, req.NamespacedName, &np); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling NodePartitioning", "name", np.Name, "node", np.Spec.NodeName, "phase", np.Status.Phase)

	// Use framework orchestrator with closures
	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*infrastructurev1alpha1.NodePartitioning, infrastructurev1alpha1.NodePartitioningStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &np,
		Recorder:   r.Recorder,
		FieldOwner: nodePartitioningFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &np)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			// NodePartitioning controller doesn't create child resources
			// It performs direct operations on nodes
			return nil, nil
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *NodePartitioningObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*NodePartitioningObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &np, o, errs)
		},

		FinalizeFn: nil, // Phase 1: No cleanup needed
	})
}

// observe gathers current cluster state (read-only).
func (r *NodePartitioningReconciler) observe(ctx context.Context, np *infrastructurev1alpha1.NodePartitioning) (*NodePartitioningObservation, error) {
	logger := log.FromContext(ctx)
	obs := &NodePartitioningObservation{}

	// Get the target node
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: np.Spec.NodeName}, &node); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Target node not found", "node", np.Spec.NodeName)
			return obs, nil // Node doesn't exist
		}
		return nil, fmt.Errorf("failed to get node %s: %w", np.Spec.NodeName, err)
	}
	obs.Node = &node

	// Get DCM ConfigMap
	dcmConfigMap, err := GetDCMConfigMap(ctx, r.Client)
	if err != nil {
		logger.Info("DCM ConfigMap not available", "error", err)
		// Don't fail observation, just log
	} else {
		obs.DCMConfigMap = dcmConfigMap
	}

	// Check DCM profile application state from node label
	obs.DCMProfileState = obs.Node.Labels[DCMNodeStateLabelKey]

	return obs, nil
}

// projectStatus executes the state machine and updates status.
func (r *NodePartitioningReconciler) projectStatus(
	ctx context.Context,
	np *infrastructurev1alpha1.NodePartitioning,
	obs *NodePartitioningObservation,
	errs controllerutils.ReconcileErrors,
) error {
	logger := log.FromContext(ctx)

	// Initialize status
	np.Status.ObservedGeneration = np.Generation

	// Handle observation errors
	if errs.ObserveErr != nil {
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		controllerutils.EmitWarningEvent(r.Recorder, np, "ObservationFailed", fmt.Sprintf("Failed to observe cluster state: %v", errs.ObserveErr))
		return nil
	}

	if obs == nil {
		return nil
	}

	// Check if node exists
	if obs.Node == nil {
		controllerutils.EmitWarningEvent(r.Recorder, np, "NodeNotFound", fmt.Sprintf("Target node %s does not exist", np.Spec.NodeName))
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.NodePartitioningConditionNodeCordoned,
			Status:             metav1.ConditionFalse,
			Reason:             "NodeNotFound",
			Message:            "Target node does not exist",
			ObservedGeneration: np.Generation,
		})
		return nil
	}

	// Execute state machine
	if err := r.executeStateMachine(ctx, np, obs); err != nil {
		controllerutils.EmitWarningEvent(r.Recorder, np, "StateMachineFailed", fmt.Sprintf("State machine execution failed: %v", err))
		logger.Error(err, "State machine execution failed")
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		return nil // Don't fail reconciliation, status is updated
	}

	return nil
}

// executeStateMachine executes the node partitioning state machine.
func (r *NodePartitioningReconciler) executeStateMachine(
	ctx context.Context,
	np *infrastructurev1alpha1.NodePartitioning,
	obs *NodePartitioningObservation,
) error {
	logger := log.FromContext(ctx)

	// Handle dry-run mode
	if np.Spec.DryRun {
		// Only skip if not already in a terminal state
		if np.Status.Phase == "" || np.Status.Phase == infrastructurev1alpha1.NodePartitioningPhasePending {
			logger.Info("Dry-run mode enabled, skipping actual operations", "node", np.Spec.NodeName)
			r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseSkipped)

			// Set DryRun condition
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               "DryRun",
				Status:             metav1.ConditionTrue,
				Reason:             "DryRunEnabled",
				Message:            "Dry-run mode: no actual changes will be made",
				ObservedGeneration: np.Generation,
			})

			// Set all operational conditions to reflect they were skipped
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionNodeCordoned,
				Status:             metav1.ConditionFalse,
				Reason:             "DryRunSkipped",
				Message:            "Dry-run mode: node cordon skipped",
				ObservedGeneration: np.Generation,
			})

			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionNodeTainted,
				Status:             metav1.ConditionFalse,
				Reason:             "DryRunSkipped",
				Message:            "Dry-run mode: node taint skipped",
				ObservedGeneration: np.Generation,
			})

			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionDrainCompleted,
				Status:             metav1.ConditionFalse,
				Reason:             "DryRunSkipped",
				Message:            "Dry-run mode: node drain skipped",
				ObservedGeneration: np.Generation,
			})

			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionProfileApplied,
				Status:             metav1.ConditionFalse,
				Reason:             "DryRunSkipped",
				Message:            fmt.Sprintf("Dry-run mode: DCM profile %s application skipped", np.Spec.Profile.DcmProfileName),
				ObservedGeneration: np.Generation,
			})

			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
				Status:             metav1.ConditionFalse,
				Reason:             "DryRunSkipped",
				Message:            "Dry-run mode: operator ready check skipped",
				ObservedGeneration: np.Generation,
			})

			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
				Status:             metav1.ConditionFalse,
				Reason:             "DryRunSkipped",
				Message:            "Dry-run mode: verification skipped",
				ObservedGeneration: np.Generation,
			})

			controllerutils.EmitNormalEvent(r.Recorder, np, "DryRunSkipped",
				fmt.Sprintf("Dry-run: Would partition node %s with profile %s",
					np.Spec.NodeName, np.Spec.Profile.DcmProfileName))
		}
		return nil
	}

	// Check if already at desired state
	if np.Status.Phase == infrastructurev1alpha1.NodePartitioningPhaseSucceeded &&
		np.Status.CurrentHash == np.Spec.DesiredHash {
		// Already done
		return nil
	}

	// State machine transitions
	switch np.Status.Phase {
	case "": // Uninitialized
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhasePending)
		return nil

	case infrastructurev1alpha1.NodePartitioningPhasePending:
		// Check if node resources already match expected - if so, skip drain/apply cycle
		// Use Capacity (not Allocatable) because it reflects hardware configuration
		// and isn't affected by device plugin pod not running
		if len(np.Spec.Profile.ExpectedResources) > 0 {
			resourcesMatch := true
			var mismatches []string

			for resourceName, expectedQty := range np.Spec.Profile.ExpectedResources {
				actualQty, exists := obs.Node.Status.Capacity[corev1.ResourceName(resourceName)]
				if !exists {
					resourcesMatch = false
					mismatches = append(mismatches, fmt.Sprintf("%s: not found", resourceName))
					continue
				}
				if !actualQty.Equal(expectedQty) {
					resourcesMatch = false
					mismatches = append(mismatches, fmt.Sprintf("%s: got %s (expected %s)",
						resourceName, actualQty.String(), expectedQty.String()))
				}
			}

			if resourcesMatch {
				// Node is already in desired state - skip directly to succeeded
				logger.Info("Node resources already match expected, skipping drain/apply cycle",
					"node", np.Spec.NodeName)

				// Set all conditions to reflect current state
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionNodeCordoned,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node already has desired configuration",
					ObservedGeneration: np.Generation,
				})
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionNodeTainted,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node already has desired configuration",
					ObservedGeneration: np.Generation,
				})
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionDrainCompleted,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node already has desired configuration",
					ObservedGeneration: np.Generation,
				})
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionProfileApplied,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node resources match expected values",
					ObservedGeneration: np.Generation,
				})
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node resources match expected values",
					ObservedGeneration: np.Generation,
				})
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node resources match expected configuration",
					ObservedGeneration: np.Generation,
				})

				// Update current hash and move to succeeded
				np.Status.CurrentHash = np.Spec.DesiredHash
				r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseSucceeded)
				controllerutils.EmitNormalEvent(r.Recorder, np, "AlreadyConfigured",
					fmt.Sprintf("Node %s already in desired state, skipped drain/apply", np.Spec.NodeName))
				return nil
			}

			// Resources don't match - proceed with partitioning
			logger.Info("Node resources don't match expected, will partition",
				"node", np.Spec.NodeName, "mismatches", mismatches)
		}

		// Transition to draining phase
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseDraining)
		controllerutils.EmitNormalEvent(r.Recorder, np, "DrainStarted", fmt.Sprintf("Started draining node %s", np.Spec.NodeName))
		// Return to allow status update and requeue
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseDraining:
		// Check if already cordoned
		cordonedCond := meta.FindStatusCondition(np.Status.Conditions, infrastructurev1alpha1.NodePartitioningConditionNodeCordoned)
		if cordonedCond == nil || cordonedCond.Status != metav1.ConditionTrue {
			// Cordon node
			if err := CordonNode(ctx, r.Client, np.Spec.NodeName); err != nil {
				return fmt.Errorf("failed to cordon node: %w", err)
			}
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionNodeCordoned,
				Status:             metav1.ConditionTrue,
				Reason:             "CordonSucceeded",
				Message:            "Node cordoned successfully",
				ObservedGeneration: np.Generation,
			})
			// Requeue to continue in next reconcile
			return nil
		}

		// Check if already tainted
		taintedCond := meta.FindStatusCondition(np.Status.Conditions, infrastructurev1alpha1.NodePartitioningConditionNodeTainted)
		if taintedCond == nil || taintedCond.Status != metav1.ConditionTrue {
			// Apply taint
			if err := TaintNode(ctx, r.Client, np.Spec.NodeName); err != nil {
				return fmt.Errorf("failed to taint node: %w", err)
			}
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionNodeTainted,
				Status:             metav1.ConditionTrue,
				Reason:             "TaintApplied",
				Message:            fmt.Sprintf("Taint %s=%s:NoExecute applied", TaintKey, TaintValue),
				ObservedGeneration: np.Generation,
			})
			// Requeue to continue in next reconcile
			return nil
		}

		// Check if drain is already completed
		drainedCond := meta.FindStatusCondition(np.Status.Conditions, infrastructurev1alpha1.NodePartitioningConditionDrainCompleted)
		if drainedCond == nil || drainedCond.Status != metav1.ConditionTrue {
			// Drain node (this is now idempotent and fast)
			err := DrainNode(ctx, r.Client, r.Clientset, np.Spec.NodeName)
			if err != nil {
				// Check if this is a "drain in progress" error or an actual failure
				if goerrors.Is(err, ErrDrainInProgress) {
					// Drain is still in progress - set condition to false and wait
					meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
						Type:               infrastructurev1alpha1.NodePartitioningConditionDrainCompleted,
						Status:             metav1.ConditionFalse,
						Reason:             "DrainInProgress",
						Message:            err.Error(),
						ObservedGeneration: np.Generation,
					})
					// Return nil to requeue and wait for pods to be evicted
					return nil
				}
				// Actual error - return it to trigger failure
				return fmt.Errorf("failed to drain node: %w", err)
			}

			// Drain succeeded
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionDrainCompleted,
				Status:             metav1.ConditionTrue,
				Reason:             "DrainSucceeded",
				Message:            "All non-tolerated pods evicted successfully",
				ObservedGeneration: np.Generation,
			})
			controllerutils.EmitNormalEvent(r.Recorder, np, "DrainCompleted", fmt.Sprintf("Node %s drained successfully", np.Spec.NodeName))
			// Requeue to continue in next reconcile
			return nil
		}

		// All drain steps complete, move to applying phase
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseApplying)
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseApplying:
		// Check if profile already applied
		profileAppliedCond := meta.FindStatusCondition(np.Status.Conditions, infrastructurev1alpha1.NodePartitioningConditionProfileApplied)
		if profileAppliedCond == nil || profileAppliedCond.Status != metav1.ConditionTrue {
			// Ensure profile is in DCM ConfigMap
			profileName, err := EnsureDCMProfileInConfigMap(ctx, r.Client, &np.Spec.Profile)
			if err != nil {
				return fmt.Errorf("failed to ensure DCM profile in ConfigMap: %w", err)
			}

			// Apply profile to node (label)
			if err := ApplyProfileToNode(ctx, r.Client, np.Spec.NodeName, profileName); err != nil {
				return fmt.Errorf("failed to apply profile to node: %w", err)
			}

			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionProfileApplied,
				Status:             metav1.ConditionTrue,
				Reason:             "DCMConfigUpdated",
				Message:            fmt.Sprintf("DCM profile %s applied to node", profileName),
				ObservedGeneration: np.Generation,
			})

			controllerutils.EmitNormalEvent(r.Recorder, np, "ProfileApplied", fmt.Sprintf("Applied profile %s to node %s", profileName, np.Spec.NodeName))
			// Requeue to continue in next reconcile
			return nil
		}

		// Profile applied, move to waiting for operator
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseWaitingOperator)
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseWaitingOperator:
		// Check if resources already match expected - if so, skip directly to succeeded
		// Use Capacity (not Allocatable) because it reflects hardware configuration
		if len(np.Spec.Profile.ExpectedResources) > 0 {
			resourcesMatch := true
			var mismatches []string

			for resourceName, expectedQty := range np.Spec.Profile.ExpectedResources {
				actualQty, exists := obs.Node.Status.Capacity[corev1.ResourceName(resourceName)]
				if !exists {
					resourcesMatch = false
					mismatches = append(mismatches, fmt.Sprintf("%s: not found", resourceName))
					continue
				}
				if !actualQty.Equal(expectedQty) {
					resourcesMatch = false
					mismatches = append(mismatches, fmt.Sprintf("%s: got %s (expected %s)",
						resourceName, actualQty.String(), expectedQty.String()))
				}
			}

			if resourcesMatch {
				// Resources already match - skip to succeeded
				logger.Info("Node resources already match expected while waiting for operator, skipping to succeeded",
					"node", np.Spec.NodeName)

				// Set operator ready condition
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node resources match expected values",
					ObservedGeneration: np.Generation,
				})
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
					Status:             metav1.ConditionTrue,
					Reason:             "AlreadyInDesiredState",
					Message:            "Node resources match expected configuration",
					ObservedGeneration: np.Generation,
				})

				// Untaint and uncordon node
				if err := UntaintNode(ctx, r.Client, np.Spec.NodeName); err != nil {
					return fmt.Errorf("failed to untaint node: %w", err)
				}

				if err := UncordonNode(ctx, r.Client, np.Spec.NodeName); err != nil {
					return fmt.Errorf("failed to uncordon node: %w", err)
				}

				// Update current hash and move to succeeded
				np.Status.CurrentHash = np.Spec.DesiredHash
				r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseSucceeded)
				controllerutils.EmitNormalEvent(r.Recorder, np, "AlreadyConfigured",
					fmt.Sprintf("Node %s already in desired state", np.Spec.NodeName))
				return nil
			}

			// Resources don't match yet - wait for DCM
			logger.V(1).Info("Resources don't match yet, waiting for DCM",
				"node", np.Spec.NodeName, "mismatches", mismatches)
		}

		// Check DCM profile application state from node label
		switch obs.DCMProfileState {
		case "", "processing":
			// DCM is still processing the profile
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
				Status:             metav1.ConditionFalse,
				Reason:             "DCMProcessing",
				Message:            "Waiting for DCM to apply GPU partitioning profile",
				ObservedGeneration: np.Generation,
			})
			// Requeue - will be triggered by DCM state label changes
			return nil

		case "success":
			// DCM successfully applied the profile
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
				Status:             metav1.ConditionTrue,
				Reason:             "DCMSucceeded",
				Message:            "DCM successfully applied GPU partitioning profile",
				ObservedGeneration: np.Generation,
			})

			// Untaint and uncordon node before verification
			if err := UntaintNode(ctx, r.Client, np.Spec.NodeName); err != nil {
				return fmt.Errorf("failed to untaint node: %w", err)
			}

			if err := UncordonNode(ctx, r.Client, np.Spec.NodeName); err != nil {
				return fmt.Errorf("failed to uncordon node: %w", err)
			}

			// Move to verifying
			r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseVerifying)
			return nil

		case "failed":
			// DCM failed to apply the profile
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
				Status:             metav1.ConditionFalse,
				Reason:             "DCMFailed",
				Message:            "DCM failed to apply GPU partitioning profile",
				ObservedGeneration: np.Generation,
			})

			// Untaint and uncordon node to restore normal operation
			if err := UntaintNode(ctx, r.Client, np.Spec.NodeName); err != nil {
				logger.Error(err, "Failed to untaint node after DCM failure", "node", np.Spec.NodeName)
			}

			if err := UncordonNode(ctx, r.Client, np.Spec.NodeName); err != nil {
				logger.Error(err, "Failed to uncordon node after DCM failure", "node", np.Spec.NodeName)
			}

			// Transition to Failed phase
			controllerutils.EmitWarningEvent(r.Recorder, np, "DCMFailed",
				fmt.Sprintf("DCM failed to apply profile to node %s", np.Spec.NodeName))
			r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
			return nil

		default:
			// Unknown state
			logger.Info("Unknown DCM profile state", "state", obs.DCMProfileState, "node", np.Spec.NodeName)
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
				Status:             metav1.ConditionFalse,
				Reason:             "UnknownDCMState",
				Message:            fmt.Sprintf("Unknown DCM profile state: %s", obs.DCMProfileState),
				ObservedGeneration: np.Generation,
			})
			return nil
		}

	case infrastructurev1alpha1.NodePartitioningPhaseVerifying:
		// Verify that the DCM label is correctly applied
		expectedLabel := np.Spec.Profile.DcmProfileName
		if obs.Node.Labels[DCMNodeLabelKey] != expectedLabel {
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
				Status:             metav1.ConditionFalse,
				Reason:             "DCMLabelMissing",
				Message:            fmt.Sprintf("DCM profile label %q not found on node", expectedLabel),
				ObservedGeneration: np.Generation,
			})
			// Requeue to retry
			return nil
		}

		// Verify that allocatable GPU resources match expected resources
		if len(np.Spec.Profile.ExpectedResources) > 0 {
			// Check each expected resource
			var mismatches []string
			for resourceName, expectedQty := range np.Spec.Profile.ExpectedResources {
				actualQty, exists := obs.Node.Status.Allocatable[corev1.ResourceName(resourceName)]
				if !exists {
					mismatches = append(mismatches, fmt.Sprintf("%s: not found (expected %s)", resourceName, expectedQty.String()))
					continue
				}
				if !actualQty.Equal(expectedQty) {
					mismatches = append(mismatches, fmt.Sprintf("%s: got %s (expected %s)", resourceName, actualQty.String(), expectedQty.String()))
				}
			}

			if len(mismatches) > 0 {
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
					Status:             metav1.ConditionFalse,
					Reason:             "ResourceMismatch",
					Message:            fmt.Sprintf("Resources don't match expected: %v", mismatches),
					ObservedGeneration: np.Generation,
				})
				// Requeue to retry - will be triggered by node allocatable changes
				return nil
			}
		} else {
			// If no expected resources specified, just check that GPU resources exist
			hasGPUResources := false
			for key := range obs.Node.Status.Allocatable {
				if key.String() == "amd.com/gpu" {
					hasGPUResources = true
					break
				}
			}

			if !hasGPUResources {
				meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
					Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
					Status:             metav1.ConditionFalse,
					Reason:             "GPUResourcesNotFound",
					Message:            "GPU resources not yet available on node",
					ObservedGeneration: np.Generation,
				})
				// Requeue to retry - will be triggered by node allocatable changes
				return nil
			}
		}

		// Verification passed
		meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
			Status:             metav1.ConditionTrue,
			Reason:             "VerificationSucceeded",
			Message:            "Node verification passed - all resources match expected values",
			ObservedGeneration: np.Generation,
		})

		controllerutils.EmitNormalEvent(r.Recorder, np, "VerificationSucceeded", fmt.Sprintf("Node %s verified successfully", np.Spec.NodeName))

		// Untaint and uncordon
		if err := UntaintNode(ctx, r.Client, np.Spec.NodeName); err != nil {
			return fmt.Errorf("failed to untaint node: %w", err)
		}

		if err := UncordonNode(ctx, r.Client, np.Spec.NodeName); err != nil {
			return fmt.Errorf("failed to uncordon node: %w", err)
		}

		// Update current hash to match desired
		np.Status.CurrentHash = np.Spec.DesiredHash

		// Move to succeeded
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseSucceeded)
		controllerutils.EmitNormalEvent(r.Recorder, np, "NodeSucceeded", fmt.Sprintf("Node %s partitioning completed successfully", np.Spec.NodeName))
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseFailed:
		// Phase 1: Stay in failed state, no retry logic
		logger.Info("NodePartitioning is in failed state", "node", np.Spec.NodeName)
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseSkipped:
		// Stay in skipped state (dry-run mode)
		// If dryRun is disabled, transition to pending to actually execute
		if !np.Spec.DryRun {
			logger.Info("DryRun disabled, transitioning to pending to execute", "node", np.Spec.NodeName)
			r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhasePending)
		}
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseSucceeded:
		// Check if desired hash changed (need to re-partition)
		if np.Status.CurrentHash != np.Spec.DesiredHash {
			logger.Info("Desired state changed, re-partitioning", "node", np.Spec.NodeName)

			// Clear all conditions so we start fresh
			np.Status.Conditions = nil

			r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhasePending)
		}
		return nil

	default:
		return fmt.Errorf("unknown phase: %s", np.Status.Phase)
	}
}

// setPhase updates the phase and adds a history entry.
func (r *NodePartitioningReconciler) setPhase(_ context.Context, np *infrastructurev1alpha1.NodePartitioning, phase infrastructurev1alpha1.NodePartitioningPhase) {
	np.Status.Phase = phase
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePartitioningReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create predicate to watch for DCM label changes and allocatable resource changes
	nodeChangePredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Don't reconcile on node creation
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Don't reconcile on node deletion (NodePartitioning will handle missing node)
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, ok := e.ObjectOld.(*corev1.Node)
			if !ok {
				return false
			}
			newNode, ok := e.ObjectNew.(*corev1.Node)
			if !ok {
				return false
			}

			// Check if DCM profile label changed
			oldLabel := oldNode.Labels[DCMNodeLabelKey]
			newLabel := newNode.Labels[DCMNodeLabelKey]
			if oldLabel != newLabel {
				return true
			}

			// Check if DCM state label changed
			oldState := oldNode.Labels[DCMNodeStateLabelKey]
			newState := newNode.Labels[DCMNodeStateLabelKey]
			if oldState != newState {
				return true
			}

			// Check if allocatable resources changed (specifically GPU resources)
			oldAllocatable := oldNode.Status.Allocatable
			newAllocatable := newNode.Status.Allocatable

			// Check for any GPU-related resource changes (amd.com/*, nvidia.com/*)
			for key := range newAllocatable {
				keyStr := key.String()
				if strings.HasPrefix(keyStr, "amd.com/") || strings.HasPrefix(keyStr, "nvidia.com/") {
					if !oldAllocatable[key].Equal(newAllocatable[key]) {
						return true
					}
				}
			}

			// Also check for resources that disappeared
			for key := range oldAllocatable {
				keyStr := key.String()
				if strings.HasPrefix(keyStr, "amd.com/") || strings.HasPrefix(keyStr, "nvidia.com/") {
					if _, exists := newAllocatable[key]; !exists {
						return true
					}
				}
			}

			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// Map Node events to NodePartitioning reconcile requests
	nodeToNodePartitioning := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		node, ok := obj.(*corev1.Node)
		if !ok {
			return nil
		}

		// List all NodePartitioning resources for this node
		var npList infrastructurev1alpha1.NodePartitioningList
		if err := r.List(ctx, &npList, client.MatchingFields{
			".spec.nodeName": node.Name,
		}); err != nil {
			return nil
		}

		var requests []reconcile.Request
		for _, np := range npList.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: np.Name,
				},
			})
		}

		return requests
	})

	// Map Pod deletions to NodePartitioning reconcile requests
	// This ensures we detect when drain is complete
	podToNodePartitioning := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}

		// Only care about pods on actual nodes
		if pod.Spec.NodeName == "" {
			return nil
		}

		// List all NodePartitioning resources for this node
		var npList infrastructurev1alpha1.NodePartitioningList
		if err := r.List(ctx, &npList, client.MatchingFields{
			".spec.nodeName": pod.Spec.NodeName,
		}); err != nil {
			return nil
		}

		var requests []reconcile.Request
		for _, np := range npList.Items {
			// Only reconcile if NodePartitioning is in Draining phase
			if np.Status.Phase == infrastructurev1alpha1.NodePartitioningPhaseDraining {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: np.Name,
					},
				})
			}
		}

		return requests
	})

	// Predicate to only watch for Pod deletions (not creates/updates)
	podDeletionPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Only care if pod is being deleted
			return e.ObjectNew.GetDeletionTimestamp() != nil
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.NodePartitioning{}).
		Watches(&corev1.Node{}, nodeToNodePartitioning, builder.WithPredicates(nodeChangePredicate)).
		Watches(&corev1.Pod{}, podToNodePartitioning, builder.WithPredicates(podDeletionPredicate)).
		Named("node-partitioning").
		Complete(r)
}
