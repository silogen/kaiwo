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

package kueue

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	visibilityv1beta1 "sigs.k8s.io/kueue/apis/visibility/v1beta1"
)

const (
	// ExpiredLabel is the label used to mark expired workloads
	ExpiredLabel = "kaiwo.silogen.ai/expired"
	// EvictedForAnnotation tracks which pending workload caused the eviction
	EvictedForAnnotation = "kaiwo.silogen.ai/evicted-for"
	// EvictedAtAnnotation tracks when the eviction happened
	EvictedAtAnnotation = "kaiwo.silogen.ai/evicted-at"
)

// ReclaimerLogic contains the core logic for workload reclamation
type ReclaimerLogic struct {
	client client.Client
}

// NewReclaimerLogic creates a new ReclaimerLogic instance
func NewReclaimerLogic(c client.Client) *ReclaimerLogic {
	return &ReclaimerLogic{client: c}
}

// Reclaim attempts to free resources for the given ClusterQueue by evicting expired workloads
func (r *ReclaimerLogic) Reclaim(ctx context.Context, clusterQueueName string) error {
	logger := log.FromContext(ctx).WithName("ReclaimerLogic").WithValues("clusterQueue", clusterQueueName)

	// Get head-of-line pending workload
	pendingWorkload, err := r.getHeadOfLinePendingWorkload(ctx, clusterQueueName)
	if err != nil {
		return fmt.Errorf("failed to get pending workloads: %w", err)
	}
	if pendingWorkload == nil {
		// No pending workloads, nothing to do
		logger.V(1).Info("No pending workloads found")
		return nil
	}

	logger.Info("Found head-of-line pending workload", "workload", pendingWorkload.Name)

	// Calculate resource requirements for the pending workload
	requiredResources, err := r.calculateResourceRequirements(pendingWorkload)
	if err != nil {
		return fmt.Errorf("failed to calculate resource requirements: %w", err)
	}

	if len(requiredResources) == 0 {
		logger.V(1).Info("Pending workload has no resource requirements")
		return nil
	}

	logger.Info("Calculated resource requirements", "resources", requiredResources)

	// Find expired and admitted workloads in the same ClusterQueue
	expiredWorkloads, err := r.getExpiredAdmittedWorkloads(ctx, clusterQueueName)
	if err != nil {
		return fmt.Errorf("failed to get expired workloads: %w", err)
	}

	if len(expiredWorkloads) == 0 {
		logger.V(1).Info("No expired workloads found")
		return nil
	}

	logger.Info("Found expired workloads", "count", len(expiredWorkloads))

	// Select minimal set of expired workloads to cover the resource need
	victimsToEvict := r.selectVictims(expiredWorkloads, requiredResources)

	if len(victimsToEvict) == 0 {
		logger.V(1).Info("No suitable victims found for eviction")
		return nil
	}

	logger.Info("Selected victims for eviction", "count", len(victimsToEvict))

	// Evict the selected workloads
	for _, victim := range victimsToEvict {
		if err := r.evictWorkload(ctx, victim, pendingWorkload.Name); err != nil {
			logger.Error(err, "failed to evict workload", "victim", victim.Name)
			// Continue with other victims even if one fails
		} else {
			logger.Info("Successfully evicted workload", "victim", victim.Name)
		}
	}

	return nil
}

// getHeadOfLinePendingWorkload retrieves the first pending workload using Kueue's Visibility API
func (r *ReclaimerLogic) getHeadOfLinePendingWorkload(ctx context.Context, clusterQueueName string) (*kueuev1beta1.Workload, error) {
	// Try to get pending workloads via the Visibility API subresource
	var pendingWorkloads visibilityv1beta1.PendingWorkloadsSummary
	if err := r.getPendingWorkloadsFromVisibilityAPI(ctx, clusterQueueName, &pendingWorkloads); err != nil {
		// If Visibility API is not available, fall back to listing workloads directly
		return r.getHeadOfLinePendingWorkloadFallback(ctx, clusterQueueName)
	}

	if len(pendingWorkloads.Items) == 0 {
		return nil, nil
	}

	// Find the workload with position 0 (head-of-line)
	for _, item := range pendingWorkloads.Items {
		if item.PositionInClusterQueue == 0 {
			// Get the full workload object
			var workload kueuev1beta1.Workload
			key := types.NamespacedName{
				Name:      item.Name,
				Namespace: item.Namespace,
			}
			if err := r.client.Get(ctx, key, &workload); err != nil {
				return nil, fmt.Errorf("failed to get workload %s: %w", key, err)
			}
			return &workload, nil
		}
	}

	return nil, nil
}

// getHeadOfLinePendingWorkloadFallback is a fallback when Visibility API is not available
func (r *ReclaimerLogic) getHeadOfLinePendingWorkloadFallback(ctx context.Context, clusterQueueName string) (*kueuev1beta1.Workload, error) {
	var workloadList kueuev1beta1.WorkloadList
	listOptions := []client.ListOption{
		client.MatchingLabels(map[string]string{
			"kueue.x-k8s.io/queue-name": clusterQueueName,
		}),
	}

	if err := r.client.List(ctx, &workloadList, listOptions...); err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	// Filter for pending workloads and sort by creation time
	var pendingWorkloads []kueuev1beta1.Workload
	for _, wl := range workloadList.Items {
		if !isWorkloadAdmitted(&wl) {
			pendingWorkloads = append(pendingWorkloads, wl)
		}
	}

	if len(pendingWorkloads) == 0 {
		return nil, nil
	}

	// Sort by creation timestamp (FIFO)
	sort.Slice(pendingWorkloads, func(i, j int) bool {
		return pendingWorkloads[i].CreationTimestamp.Before(&pendingWorkloads[j].CreationTimestamp)
	})

	return &pendingWorkloads[0], nil
}

// getPendingWorkloadsFromVisibilityAPI calls the Kueue Visibility API subresource
func (r *ReclaimerLogic) getPendingWorkloadsFromVisibilityAPI(ctx context.Context, clusterQueueName string, result *visibilityv1beta1.PendingWorkloadsSummary) error {
	// Use the SubResource method to access the visibility API
	return r.client.SubResource("pendingworkloads").Get(ctx, &kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{Name: clusterQueueName},
	}, result)
}

// calculateResourceRequirements extracts resource requirements from a workload
// Prefers status.resourceRequests (normalized by Kueue) over spec calculation
func (r *ReclaimerLogic) calculateResourceRequirements(workload *kueuev1beta1.Workload) (map[corev1.ResourceName]resource.Quantity, error) {
	// Prefer normalized resource requests from status (matches Kueue's math)
	if len(workload.Status.ResourceRequests) > 0 {
		return r.calculateFromStatusResourceRequests(workload), nil
	}

	// Fallback to spec-based calculation if status is not available
	return r.calculateFromSpecPodSets(workload), nil
}

// calculateFromStatusResourceRequests uses Kueue's normalized resource requests
func (r *ReclaimerLogic) calculateFromStatusResourceRequests(workload *kueuev1beta1.Workload) map[corev1.ResourceName]resource.Quantity {
	resources := make(map[corev1.ResourceName]resource.Quantity)

	for _, resourceRequest := range workload.Status.ResourceRequests {
		for resourceName, quantity := range resourceRequest.Resources {
			if existing, exists := resources[resourceName]; exists {
				existing.Add(quantity)
				resources[resourceName] = existing
			} else {
				resources[resourceName] = quantity.DeepCopy()
			}
		}
	}

	return resources
}

// calculateFromSpecPodSets calculates resources from spec as fallback
func (r *ReclaimerLogic) calculateFromSpecPodSets(workload *kueuev1beta1.Workload) map[corev1.ResourceName]resource.Quantity {
	resources := make(map[corev1.ResourceName]resource.Quantity)

	for _, podSet := range workload.Spec.PodSets {
		count := int64(podSet.Count)

		for _, container := range podSet.Template.Spec.Containers {
			for resourceName, quantity := range container.Resources.Requests {
				// Multiply by pod count and add to total
				totalQuantity := quantity.DeepCopy()
				if count > 1 {
					totalQuantity.SetMilli(totalQuantity.MilliValue() * count)
				}

				if existing, exists := resources[resourceName]; exists {
					existing.Add(totalQuantity)
					resources[resourceName] = existing
				} else {
					resources[resourceName] = totalQuantity
				}
			}
		}
	}

	return resources
}

// getExpiredAdmittedWorkloads gets all expired and admitted workloads in the cluster queue
func (r *ReclaimerLogic) getExpiredAdmittedWorkloads(ctx context.Context, clusterQueueName string) ([]kueuev1beta1.Workload, error) {
	var workloadList kueuev1beta1.WorkloadList
	listOptions := []client.ListOption{
		client.MatchingLabels(map[string]string{
			"kueue.x-k8s.io/queue-name": clusterQueueName,
			ExpiredLabel:                "true",
		}),
	}

	if err := r.client.List(ctx, &workloadList, listOptions...); err != nil {
		return nil, fmt.Errorf("failed to list expired workloads: %w", err)
	}

	// Filter for admitted workloads only
	var expiredAdmitted []kueuev1beta1.Workload
	for _, wl := range workloadList.Items {
		if isWorkloadAdmitted(&wl) && isWorkloadActive(&wl) {
			expiredAdmitted = append(expiredAdmitted, wl)
		}
	}

	return expiredAdmitted, nil
}

// selectVictims selects the minimal set of expired workloads to satisfy resource requirements
func (r *ReclaimerLogic) selectVictims(expiredWorkloads []kueuev1beta1.Workload, requiredResources map[corev1.ResourceName]resource.Quantity) []kueuev1beta1.Workload {
	// Sort expired workloads by creation time (oldest first)
	sort.Slice(expiredWorkloads, func(i, j int) bool {
		return expiredWorkloads[i].CreationTimestamp.Before(&expiredWorkloads[j].CreationTimestamp)
	})

	var victims []kueuev1beta1.Workload
	remainingNeeds := copyResourceMap(requiredResources)

	for _, workload := range expiredWorkloads {
		if len(remainingNeeds) == 0 {
			break
		}

		workloadResources, err := r.calculateResourceRequirements(&workload)
		if err != nil {
			continue
		}

		// Check if this workload can contribute to satisfying any remaining needs
		canContribute := false
		for resourceName := range remainingNeeds {
			if available, exists := workloadResources[resourceName]; exists && available.Cmp(resource.Quantity{}) > 0 {
				canContribute = true
				break
			}
		}

		if canContribute {
			victims = append(victims, workload)

			// Subtract this workload's resources from remaining needs
			for resourceName, available := range workloadResources {
				if needed, exists := remainingNeeds[resourceName]; exists {
					if needed.Cmp(available) <= 0 {
						// This fully satisfies this resource need
						delete(remainingNeeds, resourceName)
					} else {
						// Partially satisfies this resource need
						needed.Sub(available)
						remainingNeeds[resourceName] = needed
					}
				}
			}
		}
	}

	return victims
}

// evictWorkload evicts a workload by setting spec.active=false with optimistic concurrency
func (r *ReclaimerLogic) evictWorkload(ctx context.Context, workload kueuev1beta1.Workload, pendingWorkloadName string) error {
	// Re-read the workload to ensure we have the latest version
	var current kueuev1beta1.Workload
	key := client.ObjectKeyFromObject(&workload)
	if err := r.client.Get(ctx, key, &current); err != nil {
		return fmt.Errorf("failed to re-read workload: %w", err)
	}

	// Check if still admitted and active
	if !isWorkloadAdmitted(&current) || !isWorkloadActive(&current) {
		return fmt.Errorf("workload is no longer admitted or active")
	}

	// Create patch from current state
	patch := client.MergeFrom(current.DeepCopy())

	// Set spec.active=false
	active := false
	current.Spec.Active = &active

	// Add eviction annotations
	if current.Annotations == nil {
		current.Annotations = make(map[string]string)
	}
	current.Annotations[EvictedForAnnotation] = pendingWorkloadName
	current.Annotations[EvictedAtAnnotation] = time.Now().Format(time.RFC3339)

	// Use optimistic concurrency with patch to avoid conflicts
	if err := r.client.Patch(ctx, &current, patch); err != nil {
		return fmt.Errorf("failed to evict workload: %w", err)
	}

	return nil
}

// Helper functions

func isWorkloadAdmitted(workload *kueuev1beta1.Workload) bool {
	for _, condition := range workload.Status.Conditions {
		if condition.Type == kueuev1beta1.WorkloadAdmitted && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func isWorkloadActive(workload *kueuev1beta1.Workload) bool {
	return workload.Spec.Active == nil || *workload.Spec.Active
}

func copyResourceMap(original map[corev1.ResourceName]resource.Quantity) map[corev1.ResourceName]resource.Quantity {
	copy := make(map[corev1.ResourceName]resource.Quantity)
	for k, v := range original {
		copy[k] = v.DeepCopy()
	}
	return copy
}