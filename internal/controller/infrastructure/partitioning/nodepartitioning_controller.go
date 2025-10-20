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
	"fmt"

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
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=partitioningprofiles,verbs=get;list;watch
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

	// Get the PartitioningProfile
	var profile infrastructurev1alpha1.PartitioningProfile
	if err := r.Get(ctx, types.NamespacedName{Name: np.Spec.ProfileRef.Name}, &profile); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PartitioningProfile not found", "profile", np.Spec.ProfileRef.Name)
			return obs, nil
		}
		return nil, fmt.Errorf("failed to get PartitioningProfile %s: %w", np.Spec.ProfileRef.Name, err)
	}
	obs.Profile = &profile

	// Get DCM ConfigMap
	dcmConfigMap, err := GetDCMConfigMap(ctx, r.Client)
	if err != nil {
		logger.Info("DCM ConfigMap not available", "error", err)
		// Don't fail observation, just log
	} else {
		obs.DCMConfigMap = dcmConfigMap
	}

	// Check if device plugin is ready (simplified check)
	obs.DevicePluginReady = r.isDevicePluginReady(ctx, np.Spec.NodeName)

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

	// Check if profile exists
	if obs.Profile == nil {
		controllerutils.EmitWarningEvent(r.Recorder, np, "ProfileNotFound", fmt.Sprintf("PartitioningProfile %s does not exist", np.Spec.ProfileRef.Name))
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
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
			if err := DrainNode(ctx, r.Client, r.Clientset, np.Spec.NodeName); err != nil {
				return fmt.Errorf("failed to drain node: %w", err)
			}
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
			profileName, err := EnsureDCMProfileInConfigMap(ctx, r.Client, obs.Profile)
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
		// Check if device plugin is ready on this node
		if !obs.DevicePluginReady {
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
				Status:             metav1.ConditionFalse,
				Reason:             "DevicePluginNotReady",
				Message:            "Waiting for AMD GPU device plugin to be ready",
				ObservedGeneration: np.Generation,
			})
			// Requeue - will be triggered by node status changes
			return nil
		}

		// Device plugin is ready
		meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
			Status:             metav1.ConditionTrue,
			Reason:             "OperatorReady",
			Message:            "AMD GPU operator is ready",
			ObservedGeneration: np.Generation,
		})

		// Move to verifying
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseVerifying)
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseVerifying:
		// Verify that the DCM label is correctly applied
		if obs.Node.Labels[DCMNodeLabelKey] == "" {
			meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
				Status:             metav1.ConditionFalse,
				Reason:             "DCMLabelMissing",
				Message:            "DCM profile label not found on node",
				ObservedGeneration: np.Generation,
			})
			// Requeue to retry
			return nil
		}

		// Verify that allocatable GPU resources exist
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

		// Verification passed
		meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.NodePartitioningConditionVerified,
			Status:             metav1.ConditionTrue,
			Reason:             "VerificationSucceeded",
			Message:            "Node verification passed",
			ObservedGeneration: np.Generation,
		})

		controllerutils.EmitNormalEvent(r.Recorder, np, "VerificationSucceeded", fmt.Sprintf("Node %s verified successfully", np.Spec.NodeName))

		// Untaint and uncordon
		if err := UntaintNode(ctx, r.Client, np.Spec.NodeName); err != nil {
			logger.Error(err, "Failed to untaint node", "node", np.Spec.NodeName)
			// Don't fail, continue
		}

		if err := UncordonNode(ctx, r.Client, np.Spec.NodeName); err != nil {
			logger.Error(err, "Failed to uncordon node", "node", np.Spec.NodeName)
			// Don't fail, continue
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

	case infrastructurev1alpha1.NodePartitioningPhaseSucceeded:
		// Check if desired hash changed (need to re-partition)
		if np.Status.CurrentHash != np.Spec.DesiredHash {
			logger.Info("Desired state changed, re-partitioning", "node", np.Spec.NodeName)
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

// isDevicePluginReady checks if the AMD device plugin is ready on a node.
func (r *NodePartitioningReconciler) isDevicePluginReady(ctx context.Context, nodeName string) bool {
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace("kube-amd-gpu")); err != nil {
		return false
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName && isPodReady(&pod) {
			return true
		}
	}

	return false
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

			// Check if DCM label changed
			oldLabel := oldNode.Labels[DCMNodeLabelKey]
			newLabel := newNode.Labels[DCMNodeLabelKey]
			if oldLabel != newLabel {
				return true
			}

			// Check if allocatable resources changed (specifically GPU resources)
			oldAllocatable := oldNode.Status.Allocatable
			newAllocatable := newNode.Status.Allocatable

			// Check for any GPU-related resource changes
			for key := range newAllocatable {
				if key.String() == "amd.com/gpu" || key.String() == "nvidia.com/gpu" {
					if !oldAllocatable[key].Equal(newAllocatable[key]) {
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.NodePartitioning{}).
		Watches(&corev1.Node{}, nodeToNodePartitioning, builder.WithPredicates(nodeChangePredicate)).
		Named("node-partitioning").
		Complete(r)
}
