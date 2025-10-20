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
	"crypto/sha256"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	partitioningPlanFieldOwner = "partitioning-plan-controller"
	partitioningPlanFinalizer  = "infrastructure.silogen.ai/partitioning-plan"
)

// PartitioningPlanReconciler reconciles a PartitioningPlan object.
type PartitioningPlanReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=partitioningplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=partitioningplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=partitioningplans/finalizers,verbs=update
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=partitioningprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.silogen.ai,resources=nodepartitionings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *PartitioningPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the plan
	var plan infrastructurev1alpha1.PartitioningPlan
	if err := r.Get(ctx, req.NamespacedName, &plan); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	baseutils.Debug(logger, "Reconciling PartitioningPlan", "name", plan.Name)

	// Use framework orchestrator with closures
	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*infrastructurev1alpha1.PartitioningPlan, infrastructurev1alpha1.PartitioningPlanStatus]{
		Client:     r.Client,
		Scheme:     r.Scheme,
		Object:     &plan,
		Recorder:   r.Recorder,
		FieldOwner: partitioningPlanFieldOwner,

		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &plan)
		},

		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var o *PlanObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*PlanObservation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &plan, o)
		},

		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *PlanObservation
			if obs != nil {
				var ok bool
				o, ok = obs.(*PlanObservation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &plan, o, errs)
		},

		FinalizeFn: nil, // Phase 1: No external cleanup needed
	})
}

// observe gathers current cluster state (read-only).
func (r *PartitioningPlanReconciler) observe(ctx context.Context, plan *infrastructurev1alpha1.PartitioningPlan) (*PlanObservation, error) {
	logger := log.FromContext(ctx)
	obs := &PlanObservation{
		Profiles: make(map[string]*infrastructurev1alpha1.PartitioningProfile),
	}

	// 1. Get all matching nodes
	var allNodes corev1.NodeList
	if err := r.List(ctx, &allNodes); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Build set of matching nodes across all partitioning rules
	matchingNodesMap := make(map[string]corev1.Node)
	for _, rule := range plan.Spec.Rules {
		selector, err := metav1.LabelSelectorAsSelector(&rule.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector in rule %q: %w", rule.Name, err)
		}

		var excludeSelector labels.Selector

		for _, node := range allNodes.Items {
			if selector.Matches(labels.Set(node.Labels)) {
				// Check exclude
				if excludeSelector.Matches(labels.Set(node.Labels)) {
					continue
				}
				matchingNodesMap[node.Name] = node
			}
		}
	}

	// Convert map to slice
	for _, node := range matchingNodesMap {
		obs.MatchingNodes = append(obs.MatchingNodes, node)
	}

	baseutils.Debug(logger, "Found matching nodes", "count", len(obs.MatchingNodes))

	// 2. Get existing NodePartitioning children
	var children infrastructurev1alpha1.NodePartitioningList
	if err := r.List(ctx, &children, client.MatchingFields{
		".metadata.ownerReferences.uid": string(plan.UID),
	}); err != nil {
		logger.Info("Failed to list NodePartitioning children (index may not be set up yet)", "error", err)
		// Try listing all and filtering manually
		var allChildren infrastructurev1alpha1.NodePartitioningList
		if err := r.List(ctx, &allChildren); err != nil {
			return nil, fmt.Errorf("failed to list NodePartitioning resources: %w", err)
		}

		for _, child := range allChildren.Items {
			if child.Spec.PlanRef.Name == plan.Name {
				children.Items = append(children.Items, child)
			}
		}
	}

	obs.ExistingChildren = children.Items
	baseutils.Debug(logger, "Found existing children", "count", len(obs.ExistingChildren))

	// 3. Resolve referenced profiles
	profileNames := make(map[string]bool)
	for _, rule := range plan.Spec.Rules {
		profileNames[rule.ProfileRef.Name] = true
	}

	for profileName := range profileNames {
		var profile infrastructurev1alpha1.PartitioningProfile
		if err := r.Get(ctx, types.NamespacedName{Name: profileName}, &profile); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("PartitioningProfile not found", "profile", profileName)
				continue
			}
			return nil, fmt.Errorf("failed to get PartitioningProfile %s: %w", profileName, err)
		}
		obs.Profiles[profileName] = &profile
	}

	return obs, nil
}

// plan computes desired state (pure function).
func (r *PartitioningPlanReconciler) plan(ctx context.Context, plan *infrastructurev1alpha1.PartitioningPlan, obs *PlanObservation) ([]client.Object, error) {
	logger := log.FromContext(ctx)

	if obs == nil {
		return nil, fmt.Errorf("observation is nil")
	}

	var desired []client.Object

	// Build a map of desired NodePartitioning resources
	desiredChildren := make(map[string]*infrastructurev1alpha1.NodePartitioning)

	for _, node := range obs.MatchingNodes {
		// Find which rule matches this node (use first match)
		var matchedRule *infrastructurev1alpha1.PartitioningRule
		for i := range plan.Spec.Rules {
			rule := &plan.Spec.Rules[i]
			selector, err := metav1.LabelSelectorAsSelector(&rule.Selector)
			if err != nil {
				continue
			}

			if !selector.Matches(labels.Set(node.Labels)) {
				continue
			}

			matchedRule = rule
			break
		}

		if matchedRule == nil {
			continue // No rule matches, skip
		}

		// Get profile
		profile, exists := obs.Profiles[matchedRule.ProfileRef.Name]
		if !exists {
			logger.Info("Profile not found for node", "node", node.Name, "profile", matchedRule.ProfileRef.Name)
			continue
		}

		// Compute desired hash
		desiredHash, err := computeDesiredHash(profile)
		if err != nil {
			logger.Error(err, "Failed to compute desired hash", "node", node.Name)
			continue
		}

		// Create NodePartitioning spec
		npName := fmt.Sprintf("%s-%s", plan.Name, node.Name)
		np := &infrastructurev1alpha1.NodePartitioning{
			ObjectMeta: metav1.ObjectMeta{
				Name: npName,
			},
			Spec: infrastructurev1alpha1.NodePartitioningSpec{
				PlanRef: infrastructurev1alpha1.PlanReference{
					Name: plan.Name,
					UID:  string(plan.UID),
				},
				NodeName:    node.Name,
				DesiredHash: desiredHash,
				ProfileRef:  matchedRule.ProfileRef,
			},
		}

		// Set owner reference
		if err := controllerutil.SetControllerReference(plan, np, r.Scheme); err != nil {
			return nil, fmt.Errorf("failed to set controller reference: %w", err)
		}

		desiredChildren[node.Name] = np
	}

	// Merge with existing children
	existingMap := make(map[string]*infrastructurev1alpha1.NodePartitioning)
	for i := range obs.ExistingChildren {
		child := &obs.ExistingChildren[i]
		existingMap[child.Spec.NodeName] = child
	}

	// Add or update children
	for nodeName, desiredChild := range desiredChildren {
		existing, exists := existingMap[nodeName]
		if exists {
			// Update if needed
			if existing.Spec.DesiredHash != desiredChild.Spec.DesiredHash ||
				existing.Spec.ProfileRef.Name != desiredChild.Spec.ProfileRef.Name {
				// Need to update
				updated := existing.DeepCopy()
				updated.Spec = desiredChild.Spec
				desired = append(desired, updated)
			}
		} else {
			// Create new
			desired = append(desired, desiredChild)
		}
	}

	// Phase 1: Don't delete children for nodes that no longer match
	// This will be added in Phase 2 with proper cleanup

	baseutils.Debug(logger, "Planned NodePartitioning resources", "count", len(desired))
	return desired, nil
}

// projectStatus computes status from observation + errors.
func (r *PartitioningPlanReconciler) projectStatus(
	ctx context.Context,
	plan *infrastructurev1alpha1.PartitioningPlan,
	obs *PlanObservation,
	errs controllerutils.ReconcileErrors,
) error {
	logger := log.FromContext(ctx)

	// Initialize status
	now := metav1.Now()
	plan.Status.ObservedGeneration = plan.Generation
	plan.Status.LastSyncTime = &now

	// Handle errors
	if errs.HasError() {
		if errs.ObserveErr != nil {
			meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.PartitioningPlanConditionPlanReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ObservationFailed",
				Message:            fmt.Sprintf("Failed to observe cluster state: %v", errs.ObserveErr),
				ObservedGeneration: plan.Generation,
			})
			plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhaseDegraded
			return nil
		}

		if errs.PlanErr != nil {
			meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
				Type:               infrastructurev1alpha1.PartitioningPlanConditionPlanReady,
				Status:             metav1.ConditionFalse,
				Reason:             "PlanningFailed",
				Message:            fmt.Sprintf("Failed to compute desired state: %v", errs.PlanErr),
				ObservedGeneration: plan.Generation,
			})
			plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhaseDegraded
			return nil
		}

		if errs.ApplyErr != nil {
			logger.Error(errs.ApplyErr, "Failed to apply desired state")
			// Don't fail the reconciliation, just log and continue to status projection
		}
	}

	if obs == nil {
		return nil
	}

	// Aggregate status from children
	summary := infrastructurev1alpha1.PlanSummary{
		MatchingNodes: int32(len(obs.MatchingNodes)),
	}

	var nodeStatuses []infrastructurev1alpha1.NodeStatusSummary
	for _, child := range obs.ExistingChildren {
		summary.TotalNodes++

		switch child.Status.Phase {
		case infrastructurev1alpha1.NodePartitioningPhasePending:
			summary.Pending++
		case infrastructurev1alpha1.NodePartitioningPhaseDraining,
			infrastructurev1alpha1.NodePartitioningPhaseApplying,
			infrastructurev1alpha1.NodePartitioningPhaseWaitingOperator:
			summary.Applying++
		case infrastructurev1alpha1.NodePartitioningPhaseVerifying:
			summary.Verifying++
		case infrastructurev1alpha1.NodePartitioningPhaseSucceeded:
			summary.Succeeded++
		case infrastructurev1alpha1.NodePartitioningPhaseFailed:
			summary.Failed++
		case infrastructurev1alpha1.NodePartitioningPhaseSkipped:
			summary.Skipped++
		}

		// Add to node statuses
		nodeStatuses = append(nodeStatuses, infrastructurev1alpha1.NodeStatusSummary{
			NodeName:    child.Spec.NodeName,
			DesiredHash: child.Spec.DesiredHash,
			CurrentHash: child.Status.CurrentHash,
			Phase:       child.Status.Phase,
		})
	}

	plan.Status.Summary = summary
	plan.Status.NodeStatuses = nodeStatuses

	// Update phase
	if plan.Spec.Paused {
		plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhasePaused
	} else if summary.Failed > 0 {
		plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhaseDegraded
	} else if summary.Applying > 0 || summary.Verifying > 0 || summary.Pending > 0 {
		plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhaseProgressing
	} else if summary.Succeeded == summary.MatchingNodes {
		plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhaseCompleted
	} else {
		plan.Status.Phase = infrastructurev1alpha1.PartitioningPlanPhasePending
	}

	// Update conditions
	meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
		Type:               infrastructurev1alpha1.PartitioningPlanConditionPlanReady,
		Status:             metav1.ConditionTrue,
		Reason:             "PlanValid",
		Message:            fmt.Sprintf("Plan is valid, targeting %d nodes", summary.MatchingNodes),
		ObservedGeneration: plan.Generation,
	})

	switch plan.Status.Phase {
	case infrastructurev1alpha1.PartitioningPlanPhaseProgressing:
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionRolloutProgressing,
			Status:             metav1.ConditionTrue,
			Reason:             "NodeOperationsInProgress",
			Message:            fmt.Sprintf("%d nodes in progress", summary.Applying+summary.Verifying+summary.Pending),
			ObservedGeneration: plan.Generation,
		})
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionRolloutCompleted,
			Status:             metav1.ConditionFalse,
			Reason:             "RolloutInProgress",
			Message:            "Rollout is still in progress",
			ObservedGeneration: plan.Generation,
		})
	case infrastructurev1alpha1.PartitioningPlanPhaseCompleted:
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionRolloutProgressing,
			Status:             metav1.ConditionFalse,
			Reason:             "RolloutComplete",
			Message:            "All nodes have been successfully partitioned",
			ObservedGeneration: plan.Generation,
		})
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionRolloutCompleted,
			Status:             metav1.ConditionTrue,
			Reason:             "AllNodesSucceeded",
			Message:            fmt.Sprintf("All %d nodes succeeded", summary.Succeeded),
			ObservedGeneration: plan.Generation,
		})
	}

	if summary.Failed > 0 {
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionRolloutDegraded,
			Status:             metav1.ConditionTrue,
			Reason:             "NodeFailures",
			Message:            fmt.Sprintf("%d nodes failed", summary.Failed),
			ObservedGeneration: plan.Generation,
		})
	} else {
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionRolloutDegraded,
			Status:             metav1.ConditionFalse,
			Reason:             "NoFailures",
			Message:            "No node failures",
			ObservedGeneration: plan.Generation,
		})
	}

	if plan.Spec.Paused {
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionPaused,
			Status:             metav1.ConditionTrue,
			Reason:             "PlanPaused",
			Message:            "Plan is paused by user",
			ObservedGeneration: plan.Generation,
		})
	} else {
		meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
			Type:               infrastructurev1alpha1.PartitioningPlanConditionPaused,
			Status:             metav1.ConditionFalse,
			Reason:             "PlanActive",
			Message:            "Plan is active",
			ObservedGeneration: plan.Generation,
		})
	}

	return nil
}

// computeDesiredHash computes a deterministic hash of the desired partition state.
func computeDesiredHash(profile *infrastructurev1alpha1.PartitioningProfile) (string, error) {
	// Include relevant fields in the hash
	hashInput := map[string]interface{}{
		"profileName":       profile.Name,
		"profileGeneration": profile.Generation,
		"profileSpec":       profile.Spec,
	}

	data, err := json.Marshal(hashInput)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PartitioningPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.PartitioningPlan{}).
		Owns(&infrastructurev1alpha1.NodePartitioning{}).
		Named("partitioning-plan").
		Complete(r)
}
