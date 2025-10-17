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

package infrastructure

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	now := metav1.Now()
	np.Status.ObservedGeneration = np.Generation
	np.Status.LastUpdateTime = &now

	// Handle observation errors
	if errs.ObserveErr != nil {
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		np.Status.LastError = fmt.Sprintf("Observation failed: %v", errs.ObserveErr)
		np.Status.LastErrorClass = infrastructurev1alpha1.ErrorClassNodeNotFound
		return nil
	}

	if obs == nil {
		return nil
	}

	// Check if node exists
	if obs.Node == nil {
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		np.Status.LastError = fmt.Sprintf("Node %s not found", np.Spec.NodeName)
		np.Status.LastErrorClass = infrastructurev1alpha1.ErrorClassNodeNotFound
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
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		np.Status.LastError = fmt.Sprintf("PartitioningProfile %s not found", np.Spec.ProfileRef.Name)
		np.Status.LastErrorClass = infrastructurev1alpha1.ErrorClassApplyFailed
		return nil
	}

	// Execute state machine
	if err := r.executeStateMachine(ctx, np, obs); err != nil {
		logger.Error(err, "State machine execution failed")
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseFailed)
		np.Status.LastError = err.Error()
		// Classify error
		np.Status.LastErrorClass = classifyError(err)
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
		// Start draining
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseDraining)
		r.addHistoryEntry(np, "Started cordoning and draining node")
		controllerutils.EmitNormalEvent(r.Recorder, np, "DrainStarted", fmt.Sprintf("Started draining node %s", np.Spec.NodeName))
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseDraining:
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

		// Drain node
		if err := DrainNode(ctx, r.Client, r.Clientset, np.Spec.NodeName, np.Spec.DrainPolicy); err != nil {
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

		// Move to applying
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseApplying)
		r.addHistoryEntry(np, "Drain completed, applying DCM profile")
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseApplying:
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

		// Move to waiting for operator
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseWaitingOperator)
		r.addHistoryEntry(np, "Waiting for AMD GPU operator to reconcile")
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseWaitingOperator:
		// Wait for operator to be ready
		if err := WaitForOperatorReady(ctx, r.Client, np.Spec.NodeName, np.Spec.Verification.TimeoutSeconds); err != nil {
			// Not ready yet, will retry
			logger.V(1).Info("Operator not ready yet", "node", np.Spec.NodeName, "error", err)
			return nil // Don't fail, just wait
		}

		meta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.NodePartitioningConditionOperatorReady,
			Status:             metav1.ConditionTrue,
			Reason:             "OperatorReady",
			Message:            "AMD GPU operator is ready",
			ObservedGeneration: np.Generation,
		})

		// Move to verifying
		r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhaseVerifying)
		r.addHistoryEntry(np, "Verifying partition configuration")
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseVerifying:
		// Verify partitioning succeeded
		if err := VerifyPartitioning(ctx, r.Client, np.Spec.NodeName, obs.Profile, np.Spec.Verification); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

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
		r.addHistoryEntry(np, "Partitioning completed successfully")
		controllerutils.EmitNormalEvent(r.Recorder, np, "NodeSucceeded", fmt.Sprintf("Node %s partitioning completed successfully", np.Spec.NodeName))
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseFailed:
		// Phase 1: Stay in failed state, no retry logic
		logger.Info("NodePartitioning is in failed state", "node", np.Spec.NodeName, "error", np.Status.LastError)
		return nil

	case infrastructurev1alpha1.NodePartitioningPhaseSucceeded:
		// Check if desired hash changed (need to re-partition)
		if np.Status.CurrentHash != np.Spec.DesiredHash {
			logger.Info("Desired state changed, re-partitioning", "node", np.Spec.NodeName)
			r.setPhase(ctx, np, infrastructurev1alpha1.NodePartitioningPhasePending)
			r.addHistoryEntry(np, "Desired state changed, starting re-partition")
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

// addHistoryEntry adds an entry to the history ring buffer.
func (r *NodePartitioningReconciler) addHistoryEntry(np *infrastructurev1alpha1.NodePartitioning, message string) {
	entry := infrastructurev1alpha1.NodePartitioningHistoryEntry{
		At:      metav1.Now(),
		Phase:   np.Status.Phase,
		Message: message,
	}

	np.Status.History = append(np.Status.History, entry)

	// Keep only last 20 entries
	if len(np.Status.History) > 20 {
		np.Status.History = np.Status.History[len(np.Status.History)-20:]
	}
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

// classifyError classifies an error into an ErrorClass.
func classifyError(err error) infrastructurev1alpha1.ErrorClass {
	errStr := err.Error()

	if contains(errStr, "not found") {
		return infrastructurev1alpha1.ErrorClassNodeNotFound
	}
	if contains(errStr, "drain timeout") || contains(errStr, "timeout") {
		return infrastructurev1alpha1.ErrorClassDrainTimeout
	}
	if contains(errStr, "operator") {
		return infrastructurev1alpha1.ErrorClassOperatorUnavailable
	}
	if contains(errStr, "apply") || contains(errStr, "profile") {
		return infrastructurev1alpha1.ErrorClassApplyFailed
	}
	if contains(errStr, "verif") {
		return infrastructurev1alpha1.ErrorClassVerifyFailed
	}

	return infrastructurev1alpha1.ErrorClassApplyFailed // Default
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePartitioningReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.NodePartitioning{}).
		Named("node-partitioning").
		Complete(r)
}
