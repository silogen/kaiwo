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
	stderrors "errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
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

		CleanupFn: func(ctx context.Context, obs any, desired []client.Object) error {
			if obs == nil {
				return nil
			}
			o, ok := obs.(*PlanObservation)
			if !ok {
				return fmt.Errorf("unexpected observation type %T", obs)
			}
			return r.cleanupStaleNodePartitionings(ctx, &plan, o)
		},

		FinalizeFn: nil, // Phase 1: No external cleanup needed
	})
}

// observe gathers cluster state for a plan by
// (1) walking every node and tracking which rules match this node,
// (2) cataloging NodePartitioning CRs owned by this plan along with any conflicting CRs owned elsewhere, and
// (3) resolving all referenced partitioning profiles into a cache for later steps.
func (r *PartitioningPlanReconciler) observe(ctx context.Context, plan *infrastructurev1alpha1.PartitioningPlan) (*PlanObservation, error) {
	logger := log.FromContext(ctx)
	obs := &PlanObservation{}

	// 1. Get all matching nodes
	var allNodes corev1.NodeList
	if err := r.List(ctx, &allNodes); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Build set of matching nodes across all partitioning rules
	matchingNodesMap := make(map[string]corev1.Node)
	obs.RuleMatches = make(map[string][]int)
	for i := range plan.Spec.Rules {
		rule := &plan.Spec.Rules[i]
		selector, err := metav1.LabelSelectorAsSelector(&rule.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector in rule %q: %w", rule.Description, err)
		}

		for _, node := range allNodes.Items {
			// Skip control plane nodes if excludeControlPlane is enabled
			if plan.ShouldExcludeControlPlane() && isControlPlaneNode(&node) {
				logger.V(1).Info("Excluding control plane node", "node", node.Name)
				continue
			}

			if selector.Matches(labels.Set(node.Labels)) {
				matchingNodesMap[node.Name] = node
				obs.RuleMatches[node.Name] = append(obs.RuleMatches[node.Name], i)
			}
		}
	}

	// Convert map to slice
	for _, node := range matchingNodesMap {
		obs.MatchingNodes = append(obs.MatchingNodes, node)
	}

	baseutils.Debug(logger, "Found matching nodes", "count", len(obs.MatchingNodes))

	// 2. Get existing NodePartitioning children and detect conflicts
	obs.ConflictingNodePartitionings = make(map[string][]infrastructurev1alpha1.NodePartitioning)

	var nodePartitionings infrastructurev1alpha1.NodePartitioningList
	if err := r.List(ctx, &nodePartitionings); err != nil {
		return nil, fmt.Errorf("failed to list NodePartitioning resources: %w", err)
	}

	for _, child := range nodePartitionings.Items {
		nodeName := child.Spec.NodeName
		if nodeName == "" {
			continue
		}

		if child.Spec.PlanRef.Name == plan.Name {
			obs.ExistingChildren = append(obs.ExistingChildren, child)
			continue
		}

		obs.ConflictingNodePartitionings[nodeName] = append(obs.ConflictingNodePartitionings[nodeName], child)
	}

	baseutils.Debug(logger, "Found existing children", "count", len(obs.ExistingChildren))
	baseutils.Debug(logger, "Found NodePartitionings from other plans", "count", len(obs.ConflictingNodePartitionings))

	return obs, nil
}

// plan turns the observation into desired NodePartitioning specs by
// (1) validating each node is owned by exactly one rule and no other plan
// (2) projecting the chosen profile and dry-run flag onto the spec, and
// (3) diffing against existing children to determine which CRs need to be created or updated.
func (r *PartitioningPlanReconciler) plan(ctx context.Context, plan *infrastructurev1alpha1.PartitioningPlan, obs *PlanObservation) ([]client.Object, error) {
	logger := log.FromContext(ctx)

	if obs == nil {
		return nil, fmt.Errorf("observation is nil")
	}

	var desired []client.Object

	// Build a map of desired NodePartitioning resources
	desiredChildren := make(map[string]*infrastructurev1alpha1.NodePartitioning)

	for _, node := range obs.MatchingNodes {
		matchIndexes := obs.RuleMatches[node.Name]
		if len(matchIndexes) == 0 {
			logger.V(1).Info("No matching rules found for node during planning", "node", node.Name)
			continue
		}
		if len(matchIndexes) > 1 {
			var ruleNames []string
			for _, idx := range matchIndexes {
				rule := plan.Spec.Rules[idx]
				ruleLabel := rule.Description
				if ruleLabel == "" {
					ruleLabel = fmt.Sprintf("rule-%d (%s)", idx, rule.Profile.DcmProfileName)
				}
				ruleNames = append(ruleNames, ruleLabel)
			}
			return nil, fmt.Errorf("multiple partitioning rules match node %s: %s", node.Name, strings.Join(ruleNames, ", "))
		}

		if conflicts := obs.ConflictingNodePartitionings[node.Name]; len(conflicts) > 0 {
			var owners []string
			for _, conflict := range conflicts {
				owner := conflict.Spec.PlanRef.Name
				if owner == "" {
					owner = "<unknown>"
				}
				owners = append(owners, fmt.Sprintf("%s/%s", owner, conflict.Name))
			}
			return nil, fmt.Errorf("node %s is already targeted by other partitioning plan(s): %s", node.Name, strings.Join(owners, ", "))
		}

		ruleIndex := matchIndexes[0]
		matchedRule := &plan.Spec.Rules[ruleIndex]

		// Compute desired hash
		profileCopy := matchedRule.Profile
		desiredHash, err := computeDesiredHash(&profileCopy)
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
				DryRun:      plan.Spec.DryRun,
				NodeName:    node.Name,
				DesiredHash: desiredHash,
				Profile:     matchedRule.Profile,
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
				!apiequality.Semantic.DeepEqual(existing.Spec.Profile, desiredChild.Spec.Profile) ||
				existing.Spec.DryRun != desiredChild.Spec.DryRun {
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

// projectStatus folds observation plus any reconciliation errors into status by
// (1) stamping summary counts from child NodePartitionings
// (2) deriving the overall Phase, and
// (3) managing user-facing conditions so UIs and alerting capture the current rollout state.
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

		phase := infrastructurev1alpha1.NodePartitioningPhase(child.Status.Phase)
		if phase == "" {
			phase = infrastructurev1alpha1.NodePartitioningPhasePending
		}

		switch phase {
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
			Phase:       phase,
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

// cleanupStaleNodePartitionings removes NodePartitioning children whose nodes are no longer targeted by the plan.
func (r *PartitioningPlanReconciler) cleanupStaleNodePartitionings(
	ctx context.Context,
	plan *infrastructurev1alpha1.PartitioningPlan,
	obs *PlanObservation,
) error {
	logger := log.FromContext(ctx)

	keepNodes := make(map[string]struct{}, len(obs.RuleMatches))
	for nodeName, matches := range obs.RuleMatches {
		if len(matches) == 1 {
			keepNodes[nodeName] = struct{}{}
		}
	}

	var (
		errs     []error
		retained []infrastructurev1alpha1.NodePartitioning
	)
	for i := range obs.ExistingChildren {
		child := obs.ExistingChildren[i]
		if child.Spec.NodeName == "" {
			retained = append(retained, child)
			continue
		}

		if _, keep := keepNodes[child.Spec.NodeName]; keep {
			retained = append(retained, child)
			continue
		}

		if err := r.Delete(ctx, &child); err != nil {
			if errors.IsNotFound(err) {
				// Already gone, nothing to retain
				continue
			}
			errs = append(errs, fmt.Errorf("failed to delete NodePartitioning %s: %w", child.Name, err))
			retained = append(retained, child)
			continue
		}

		logger.Info("Deleted NodePartitioning for node no longer targeted by plan",
			"node", child.Spec.NodeName,
			"nodePartitioning", child.Name)

		controllerutils.EmitNormalEvent(
			r.Recorder,
			plan,
			"NodePartitioningDeleted",
			fmt.Sprintf("Removed NodePartitioning %s for node %s", child.Name, child.Spec.NodeName),
		)
	}

	obs.ExistingChildren = retained

	if len(errs) > 0 {
		return stderrors.Join(errs...)
	}

	return nil
}

// computeDesiredHash computes a deterministic hash of the desired partition state.
func computeDesiredHash(profile *infrastructurev1alpha1.PartitioningProfileSpec) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("profile is nil")
	}

	data, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash), nil
}

// isControlPlaneNode checks if a node has control plane labels.
func isControlPlaneNode(node *corev1.Node) bool {
	if node.Labels == nil {
		return false
	}

	// Check for the newer control-plane label
	if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
		return true
	}

	// Check for the older master label (deprecated but still in use)
	if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
		return true
	}

	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *PartitioningPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.PartitioningPlan{}).
		Owns(&infrastructurev1alpha1.NodePartitioning{}).
		Named("partitioning-plan").
		Complete(r)
}
