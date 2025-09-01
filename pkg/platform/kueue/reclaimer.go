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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	visibilityv1beta1 "sigs.k8s.io/kueue/apis/visibility/v1beta1"
)

// Outcome represents the result of a reclamation attempt
type Outcome int

const (
	Idle Outcome = iota
	PendingInsufficient
	EvictedOne
	Settling
	AdmittedOrChanged
)

const (
	// ExpiredLabel is the label used to mark expired workloads
	ExpiredLabel = "kaiwo.silogen.ai/expired"
	// EvictedForAnnotation tracks which pending workload caused the eviction
	EvictedForAnnotation = "kaiwo.silogen.ai/evicted-for"
	// EvictedAtAnnotation tracks when the eviction happened
	EvictedAtAnnotation = "kaiwo.silogen.ai/evicted-at"
	// AnnLastHolUID annotation on ClusterQueue to track last head-of-line workload UID
	AnnLastHolUID = "kaiwo.silogen.ai/reclaimer-last-hol-uid"
	// AnnLastEvictAt annotation on ClusterQueue to track when last eviction happened
	AnnLastEvictAt = "kaiwo.silogen.ai/reclaimer-last-evict-at"
	// GuardTTL is how long to wait before allowing another eviction for the same HoL
	GuardTTL = 12 * time.Second
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
func (r *ReclaimerLogic) Reclaim(ctx context.Context, clusterQueueName string) (Outcome, error) {
	logger := log.FromContext(ctx).WithName("ReclaimerLogic").WithValues("clusterQueue", clusterQueueName)

	// Get the ClusterQueue to check/update annotations
	var clusterQueue kueuev1beta1.ClusterQueue
	if err := r.client.Get(ctx, types.NamespacedName{Name: clusterQueueName}, &clusterQueue); err != nil {
		return Idle, fmt.Errorf("failed to get ClusterQueue: %w", err)
	}

	// Find Head of Line (HOL) pending workload
	pendingWorkload, err := r.getHeadOfLinePendingWorkload(ctx, clusterQueueName)
	if err != nil {
		return Idle, fmt.Errorf("failed to get pending workloads: %w", err)
	}
	if pendingWorkload == nil {
		return Idle, nil // No pending workloads
	}

	// Re-fetch to ensure it's still pending
	var currentPendingWorkload kueuev1beta1.Workload
	pendingKey := client.ObjectKeyFromObject(pendingWorkload)
	if err := r.client.Get(ctx, pendingKey, &currentPendingWorkload); err != nil {
		return Idle, fmt.Errorf("failed to re-read pending workload: %w", err)
	}

	// 3. If HoL already admitted or HoL UID changed since last action → AdmittedOrChanged
	if isWorkloadAdmitted(&currentPendingWorkload) {
		return AdmittedOrChanged, nil
	}

	pendingWorkload = &currentPendingWorkload

	// Check if HoL UID changed
	lastHolUID := r.getLastHolUID(&clusterQueue)
	if lastHolUID != "" && lastHolUID != string(pendingWorkload.UID) {
		return AdmittedOrChanged, nil
	}

	// 4. Compute HoL demand, filter to reclaimable resources
	requiredResources := r.calculateResourceRequirements(pendingWorkload)
	gpuResources := filterGpuResources(requiredResources)
	if len(gpuResources) == 0 {
		return Idle, nil // No GPU resources needed
	}

	// 5. Collect expired + admitted + active candidates
	expiredWorkloads, err := r.getExpiredAdmittedWorkloads(ctx, clusterQueueName)
	if err != nil {
		return Idle, fmt.Errorf("failed to get expired workloads: %w", err)
	}

	// 6. If sum(candidates) < need → PendingInsufficient
	if !r.canSatisfyResourceNeeds(expiredWorkloads, gpuResources) {
		return PendingInsufficient, nil
	}

	// 7. Fresh-eviction guard: check if same HoL and recent eviction
	if r.shouldSettle(&clusterQueue, pendingWorkload.UID) {
		return Settling, nil
	}

	// 8. Pick one victim (oldest expired who contributes to any needed resource)
	victim := r.selectSingleVictim(expiredWorkloads, gpuResources)
	if victim == nil {
		return PendingInsufficient, nil // No suitable victim found
	}

	// 9. Evict the victim
	if err := r.evictWorkload(ctx, *victim, pendingWorkload.Name); err != nil {
		logger.Info("Workload eviction failed", "victim", victim.Name, "error", err.Error())
		return Settling, nil // Short requeue on conflict
	}

	// 10. Write guard on CQ
	if err := r.markEvicted(ctx, &clusterQueue, pendingWorkload.UID); err != nil {
		logger.Info("Failed to update ClusterQueue guard", "error", err.Error())
		// Continue - eviction succeeded even if guard failed
	}

	logger.Info("Successfully evicted one workload",
		"pendingWorkload", pendingWorkload.Name,
		"victimEvicted", victim.Name)

	return EvictedOne, nil
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

	// Pick the item with the smallest PositionInClusterQueue (HoL)
	minIdx := 0
	minPos := pendingWorkloads.Items[0].PositionInClusterQueue
	for i := 1; i < len(pendingWorkloads.Items); i++ {
		if pendingWorkloads.Items[i].PositionInClusterQueue < minPos {
			minPos = pendingWorkloads.Items[i].PositionInClusterQueue
			minIdx = i
		}
	}

	var workload kueuev1beta1.Workload
	key := types.NamespacedName{
		Name:      pendingWorkloads.Items[minIdx].Name,
		Namespace: pendingWorkloads.Items[minIdx].Namespace,
	}
	if err := r.client.Get(ctx, key, &workload); err != nil {
		return nil, fmt.Errorf("failed to get workload %s: %w", key, err)
	}
	return &workload, nil
}

// getHeadOfLinePendingWorkloadFallback is a fallback when Visibility API is not available
func (r *ReclaimerLogic) getHeadOfLinePendingWorkloadFallback(ctx context.Context, clusterQueueName string) (*kueuev1beta1.Workload, error) {
	// Build a set of LocalQueue names that map to this ClusterQueue
	lqNames, err := GetLocalQueueNamesForCluster(ctx, r.client, clusterQueueName)
	if err != nil {
		return nil, err
	}

	var workloadList kueuev1beta1.WorkloadList
	if err := r.client.List(ctx, &workloadList); err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	// Filter for pending workloads that belong to one of the LocalQueues and sort by creation time
	var pendingWorkloads []kueuev1beta1.Workload
	for _, wl := range workloadList.Items {
		lqName := string(wl.Spec.QueueName)
		if lqName == "" {
			continue
		}
		if _, ok := lqNames[types.NamespacedName{Namespace: wl.Namespace, Name: lqName}]; !ok {
			continue
		}
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
func (r *ReclaimerLogic) calculateResourceRequirements(workload *kueuev1beta1.Workload) map[corev1.ResourceName]resource.Quantity {
	// Prefer normalized resource requests from status (matches Kueue's math)
	if len(workload.Status.ResourceRequests) > 0 {
		return r.calculateFromStatusResourceRequests(workload)
	}

	// Fallback to spec-based calculation if status is not available
	return r.calculateFromSpecPodSets(workload)
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
	// Build a set of LocalQueue names that map to this ClusterQueue
	lqNames, err := GetLocalQueueNamesForCluster(ctx, r.client, clusterQueueName)
	if err != nil {
		return nil, err
	}

	var workloadList kueuev1beta1.WorkloadList
	// Limit initial list by expired label to reduce load
	listOptions := []client.ListOption{client.MatchingLabels(map[string]string{ExpiredLabel: "true"})}
	if err := r.client.List(ctx, &workloadList, listOptions...); err != nil {
		return nil, fmt.Errorf("failed to list expired workloads: %w", err)
	}

	// Filter for admitted & active workloads that belong to one of the LocalQueues
	var expiredAdmitted []kueuev1beta1.Workload
	var expiredWorkloadNames []string
	skippedCount := 0

	for _, wl := range workloadList.Items {
		lqName := string(wl.Spec.QueueName)
		if lqName == "" {
			skippedCount++
			continue
		}
		if _, ok := lqNames[types.NamespacedName{Namespace: wl.Namespace, Name: lqName}]; !ok {
			skippedCount++
			continue
		}

		admitted := isWorkloadAdmitted(&wl)
		active := isWorkloadActive(&wl)

		if admitted && active {
			expiredAdmitted = append(expiredAdmitted, wl)
			expiredWorkloadNames = append(expiredWorkloadNames, wl.Name)
		} else {
			skippedCount++
		}
	}

	if len(expiredAdmitted) > 0 {
		logger := log.FromContext(ctx).WithName("getExpiredAdmittedWorkloads")
		logger.Info("Found evictable expired workloads",
			"clusterQueue", clusterQueueName,
			"evictableCount", len(expiredAdmitted),
			"skippedCount", skippedCount,
			"workloadNames", expiredWorkloadNames)
	}

	return expiredAdmitted, nil
}

// canSatisfyResourceNeeds checks if expired workloads can collectively satisfy resource needs
func (r *ReclaimerLogic) canSatisfyResourceNeeds(expiredWorkloads []kueuev1beta1.Workload, requiredResources map[corev1.ResourceName]resource.Quantity) bool {
	totalAvailable := make(map[corev1.ResourceName]resource.Quantity)

	for _, workload := range expiredWorkloads {
		workloadResources := filterGpuResources(r.calculateResourceRequirements(&workload))
		for resourceName, quantity := range workloadResources {
			if existing, exists := totalAvailable[resourceName]; exists {
				existing.Add(quantity)
				totalAvailable[resourceName] = existing
			} else {
				totalAvailable[resourceName] = quantity.DeepCopy()
			}
		}
	}

	// Check if we have enough of each required resource
	for resourceName, needed := range requiredResources {
		if available, exists := totalAvailable[resourceName]; !exists || available.Cmp(needed) < 0 {
			return false
		}
	}

	return true
}

// selectSingleVictim selects one expired workload that can contribute to resource needs
func (r *ReclaimerLogic) selectSingleVictim(expiredWorkloads []kueuev1beta1.Workload, requiredResources map[corev1.ResourceName]resource.Quantity) *kueuev1beta1.Workload {
	if len(expiredWorkloads) == 0 {
		return nil
	}

	// Sort expired workloads by creation time (oldest first)
	sort.Slice(expiredWorkloads, func(i, j int) bool {
		return expiredWorkloads[i].CreationTimestamp.Before(&expiredWorkloads[j].CreationTimestamp)
	})

	// Pick the first (oldest) workload that can contribute to any needed resource
	for _, workload := range expiredWorkloads {
		workloadResources := filterGpuResources(r.calculateResourceRequirements(&workload))

		// Check if this workload can contribute to satisfying any resource needs
		for resourceName := range requiredResources {
			if available, exists := workloadResources[resourceName]; exists && available.Cmp(resource.Quantity{}) > 0 {
				return &workload
			}
		}
	}

	return nil
}

// getLastHolUID gets the last head-of-line workload UID from ClusterQueue annotations
func (r *ReclaimerLogic) getLastHolUID(cq *kueuev1beta1.ClusterQueue) string {
	if cq.Annotations == nil {
		return ""
	}
	return cq.Annotations[AnnLastHolUID]
}

// shouldSettle checks if we should wait before evicting again for the same HoL workload
func (r *ReclaimerLogic) shouldSettle(cq *kueuev1beta1.ClusterQueue, holUID types.UID) bool {
	if cq.Annotations == nil {
		return false
	}

	// Check if this is the same HoL workload
	if cq.Annotations[AnnLastHolUID] != string(holUID) {
		return false
	}

	// Check if the last eviction was recent
	if timeStr, exists := cq.Annotations[AnnLastEvictAt]; exists {
		if lastEvictTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
			return time.Since(lastEvictTime) < GuardTTL
		}
	}

	return false
}

// markEvicted updates ClusterQueue annotations to record the eviction
func (r *ReclaimerLogic) markEvicted(ctx context.Context, cq *kueuev1beta1.ClusterQueue, holUID types.UID) error {
	patch := client.MergeFrom(cq.DeepCopy())

	if cq.Annotations == nil {
		cq.Annotations = make(map[string]string)
	}
	cq.Annotations[AnnLastHolUID] = string(holUID)
	cq.Annotations[AnnLastEvictAt] = time.Now().Format(time.RFC3339)

	return r.client.Patch(ctx, cq, patch)
}

// filterGpuResources keeps only resource names that look like GPU quantities
func filterGpuResources(resources map[corev1.ResourceName]resource.Quantity) map[corev1.ResourceName]resource.Quantity {
	if len(resources) == 0 {
		return resources
	}
	out := make(map[corev1.ResourceName]resource.Quantity, len(resources))
	for name, qty := range resources {
		s := string(name)
		// naive but practical: match substrings containing "gpu"
		if containsGPU(s) {
			out[name] = qty
		}
	}
	return out
}

func containsGPU(s string) bool { return strings.Contains(strings.ToLower(s), "gpu") }

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
	return meta.IsStatusConditionTrue(workload.Status.Conditions, kueuev1beta1.WorkloadAdmitted)
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
