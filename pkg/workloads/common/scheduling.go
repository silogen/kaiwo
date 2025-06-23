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

package common

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

var (
	ErrNoMatchingVramOptionsInCluster = errors.New("no matching vram options in cluster")
	ErrNoMatchingResourcesInCluster   = errors.New("no matching resources in cluster")
)

// GpuSchedulingResult encapsulates the outcome of the GPU scheduling calculation.
type GpuSchedulingResult struct {
	// The calculated number of GPUs per replica
	GpuCountPerReplica int

	// The GPU resource name to use
	GpuResourceName corev1.ResourceName

	// The node affinity labels to use to ensure the workload is scheduled correctly
	NodeSelectorTerms corev1.NodeSelectorTerm

	Replicas *int
}

func (r *GpuSchedulingResult) GetTotalGpus() int {
	return r.GpuCountPerReplica * *r.Replicas
}

func (r *GpuSchedulingResult) GpusRequested() bool {
	return r != nil && r.GpuCountPerReplica > 0
}

// CreateResourceRequirements creates the Kubernetes resource requirements from the GPU scheduling result
func (r *GpuSchedulingResult) CreateResourceRequirements(defaults *corev1.ResourceRequirements) corev1.ResourceRequirements {
	resourceRequirements := corev1.ResourceRequirements{}
	if defaults != nil {
		resourceRequirements = *defaults.DeepCopy()
	}

	if resourceRequirements.Requests == nil {
		resourceRequirements.Requests = corev1.ResourceList{}
	}
	if resourceRequirements.Limits == nil {
		resourceRequirements.Limits = corev1.ResourceList{}
	}

	gpuCount := 0
	if r.GpusRequested() {
		gpuCount = r.GpuCountPerReplica
		quantity := resource.MustParse(fmt.Sprintf("%d", gpuCount))
		resourceRequirements.Requests[r.GpuResourceName] = quantity
		resourceRequirements.Limits[r.GpuResourceName] = quantity
	}

	updateResourceList := func(resourceList corev1.ResourceList) {
		if _, exists := resourceList[corev1.ResourceCPU]; !exists {
			if r.GpusRequested() {
				resourceList[corev1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%d", gpuCount*4))
			} else {
				resourceList[corev1.ResourceCPU] = DefaultCPU
			}
		}
		if _, exists := resourceList[corev1.ResourceMemory]; !exists {
			if r.GpusRequested() {
				resourceList[corev1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32))
			} else {
				resourceList[corev1.ResourceMemory] = DefaultMemory
			}
		}
	}

	updateResourceList(resourceRequirements.Limits)
	updateResourceList(resourceRequirements.Requests)

	return resourceRequirements
}

// CompetesWith checks if two GpuSchedulingResult objects compete with the same GPU resources
func (r *GpuSchedulingResult) CompetesWith(other *GpuSchedulingResult) bool {
	if !r.GpusRequested() || !other.GpusRequested() {
		return false
	}
	if r.GpuResourceName != other.GpuResourceName {
		return false
	}

	return matchExpressionsOverlap(r.NodeSelectorTerms.MatchExpressions, other.NodeSelectorTerms.MatchExpressions)
}

// matchExpressionsOverlap returns true iff for every key that appears
// in both expr lists, the two In-value sets intersect non-empty.
func matchExpressionsOverlap(aExpressions, bExpressions []corev1.NodeSelectorRequirement) bool {
	aMap := make(map[string][]string, len(aExpressions))
	for _, req := range aExpressions {
		if req.Operator == corev1.NodeSelectorOpIn {
			aMap[req.Key] = req.Values
		}
	}
	bMap := make(map[string][]string, len(bExpressions))
	for _, req := range bExpressions {
		if req.Operator == corev1.NodeSelectorOpIn {
			bMap[req.Key] = req.Values
		}
	}

	// For every key both sides constrain, check that their value-lists intersect.
	for key, aVals := range aMap {
		if bVals, ok := bMap[key]; ok {
			if len(intersect(aVals, bVals)) == 0 {
				return false
			}
		}
	}
	return true
}

// simple slice intersection
func intersect(a, b []string) []string {
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	var out []string
	for _, v := range b {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

// CalculateGpuRequirements calculates the GPU requirements for the workload. It ensures that the cluster has sufficient GPU resources to
// run the workload, and if GPU count is not explicitly set, determines the most suitable GPU node type that minimizes the wasted vRAM resources
func CalculateGpuRequirements(ctx context.Context, clusterCtx ClusterContext, gpuResourceRequirements *v1alpha1.GpuResourceRequirements, replicas *int) (*GpuSchedulingResult, error) {
	result := GpuSchedulingResult{
		Replicas: replicas,
	}
	if gpuResourceRequirements == nil {
		return &result, nil
	}
	result.GpuResourceName = VendorToResourceName(gpuResourceRequirements.Vendor)
	if replicas == nil {
		result.Replicas = baseutils.Pointer(1)
	}

	var candidateNodes []NodeInfo

	for _, node := range clusterCtx.Nodes {
		if nodeMatchesRequirements(node, *gpuResourceRequirements) {
			candidateNodes = append(candidateNodes, node)
		}
	}
	if len(candidateNodes) == 0 {
		return nil, ErrNoMatchingResourcesInCluster
	}

	// Add node selectors
	if len(gpuResourceRequirements.Models) > 0 {
		result.NodeSelectorTerms.MatchExpressions = append(result.NodeSelectorTerms.MatchExpressions,
			corev1.NodeSelectorRequirement{
				Key:      NodeGpuModelLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   gpuResourceRequirements.Models,
			})
	}

	result.NodeSelectorTerms.MatchExpressions = append(result.NodeSelectorTerms.MatchExpressions,
		corev1.NodeSelectorRequirement{
			Key:      NodeGpusPartitionedLabelKey,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{strconv.FormatBool(gpuResourceRequirements.Partitioned)},
		})

	if gpuResourceRequirements.TotalVram != nil {
		bestMatchingVram := bestMatchingVramTier(candidateNodes, *gpuResourceRequirements, *result.Replicas)
		if bestMatchingVram == nil {
			return nil, ErrNoMatchingVramOptionsInCluster
		}
		result.NodeSelectorTerms.MatchExpressions = append(result.NodeSelectorTerms.MatchExpressions,
			corev1.NodeSelectorRequirement{
				Key:      NodeGpuLogicalVramLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{baseutils.QuantityToGi(*bestMatchingVram)},
			})
		result.GpuCountPerReplica = int(math.Ceil(float64(gpuResourceRequirements.TotalVram.Value()) / float64(bestMatchingVram.Value())))
	} else {
		result.GpuCountPerReplica = *gpuResourceRequirements.Count
	}

	return &result, nil
}

// nodeMatchesRequirements checks if the given node matches the workload's requirements
func nodeMatchesRequirements(nodeInfo NodeInfo, gpuResourceRequirements v1alpha1.GpuResourceRequirements) bool {
	if nodeInfo.IsUnschedulable() {
		return false
	}

	if !gpuResourceRequirements.IsGpuRequested() {
		return true
	}

	if nodeInfo.IsCpuOnlyNode() {
		return false
	}

	gpuInfo := nodeInfo.GpuInfo

	// If GPU models are defined and the node does not have a matching model, skip
	if len(gpuResourceRequirements.Models) > 0 {
		match := false
		for _, model := range gpuResourceRequirements.Models {
			if model == gpuInfo.Model {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// If the node partitioning is different, skip
	if gpuInfo.IsPartitioned() != gpuResourceRequirements.Partitioned {
		return false
	}

	// If the node GPU vendor is different, skip
	if gpuInfo.Vendor != gpuResourceRequirements.Vendor {
		return false
	}

	// If the GPU does not have the requested amount of GPUs, skip
	if count := gpuResourceRequirements.Count; count != nil && *count > gpuInfo.LogicalCount {
		return false
	}

	if gpuResourceRequirements.TotalVram != nil {
		if count := gpuResourceRequirements.Count; count != nil {
			// If the requested number of GPUs cannot satisfy the requested vRAM, skip node
			nodeSelectedGpusTotalVram := int64(*count) * gpuInfo.LogicalVramPerGpu.Value()
			if nodeSelectedGpusTotalVram < gpuResourceRequirements.TotalVram.Value() {
				return false
			}
		} else {
			// If all the GPUs in the node cannot satisfy the requested vRAM, skip node
			return gpuResourceRequirements.TotalVram.Value() <= (gpuInfo.LogicalVramPerGpu.Value() * int64(gpuInfo.LogicalCount))
		}
	}
	return true
}

// candidateTierInfo stores information about a potential vRAM tier choice during evaluation.
type candidateTierInfo struct {
	vramPerGpu      resource.Quantity // The vRAM per GPU for this tier
	gpusPerReplica  int               // Calculated GPUs needed per replica for this tier
	wastePerReplica resource.Quantity // Calculated waste (TotalVramProvided - TotalVramRequested)
}

// bestMatchingNode selects the vRAM tier which minimizes wasted vRAM but guarantees that there are adequate resources available
func bestMatchingVramTier(nodes []NodeInfo, gpuResourceRequirements v1alpha1.GpuResourceRequirements, replicas int) *resource.Quantity {
	if gpuResourceRequirements.TotalVram == nil || gpuResourceRequirements.TotalVram.IsZero() {
		// Cannot determine best tier without a target TotalVram
		return nil
	}
	if replicas <= 0 {
		// No replicas needed, so no tier selection is strictly necessary in this context.
		// Or, one might pick the smallest available if any GPU is still desired for 0 replicas (unlikely).
		return nil
	}
	requestedTotalVramPerReplica := gpuResourceRequirements.TotalVram

	// Step 1: Group nodes by their LogicalVramPerGpu and collect LogicalGpus counts for each tier.
	// The key is the vRAM per GPU (as a Quantity), value is a list of GPU counts on nodes of that tier.
	tiersNodeGpuCapacities := make(map[string]struct {
		vram resource.Quantity
		gpus []int // List of logical GPU counts on nodes of this tier
	})

	for _, node := range nodes {
		gpuInfo := node.GpuInfo
		key := gpuInfo.LogicalVramPerGpu.String()
		entry, exists := tiersNodeGpuCapacities[key]
		if !exists {
			entry.vram = gpuInfo.LogicalVramPerGpu
			entry.gpus = make([]int, 0)
		}
		entry.gpus = append(entry.gpus, gpuInfo.LogicalCount)
		tiersNodeGpuCapacities[key] = entry
	}

	var candidates []candidateTierInfo

	// Step 2: Evaluate each tier
	for _, tierData := range tiersNodeGpuCapacities {
		vramPerGpuOfTier := tierData.vram
		nodeGpuCountsInTier := tierData.gpus

		if vramPerGpuOfTier.IsZero() {
			continue // Skip tiers with zero vRAM per GPU (should not happen with valid data)
		}

		// Calculate GPUs needed per replica for this tier to meet TotalVram
		var gpusPerReplica int
		if gpuResourceRequirements.Count != nil {
			// If count is given, ensure that the requested number of GPUs can provide the requested amount of vRAM
			gpusPerReplica = *gpuResourceRequirements.Count
			totalVram := vramPerGpuOfTier.Value() * int64(gpusPerReplica)
			if totalVram < requestedTotalVramPerReplica.Value() {
				continue
			}
		} else {
			needed := float64(requestedTotalVramPerReplica.Value()) / float64(vramPerGpuOfTier.Value())
			gpusPerReplica = int(math.Ceil(needed))
		}

		// Check cluster capacity for this tier and calculated gpusPerReplica
		maxReplicasPlaceableInTier := 0
		for _, nodeGpuCapacity := range nodeGpuCountsInTier {
			if nodeGpuCapacity >= gpusPerReplica {
				maxReplicasPlaceableInTier += nodeGpuCapacity / gpusPerReplica // Integer division
			}
		}

		if maxReplicasPlaceableInTier < replicas {
			continue // This tier cannot support the required number of replicas
		}

		// Calculate waste for this configuration (per replica)
		provided := vramPerGpuOfTier.Value() * int64(gpusPerReplica)
		waste := provided - requestedTotalVramPerReplica.Value()
		wasteQty := resource.NewQuantity(waste, requestedTotalVramPerReplica.Format)

		candidates = append(candidates, candidateTierInfo{
			vramPerGpu:      vramPerGpuOfTier,
			gpusPerReplica:  gpusPerReplica,
			wastePerReplica: *wasteQty,
		})
	}

	// Step 3: Select the best candidate
	if len(candidates) == 0 {
		return nil // No tier could satisfy the requirements
	}

	sort.Slice(candidates, func(i, j int) bool {
		// Primary sort: minimize waste
		if comp := candidates[i].wastePerReplica.Cmp(candidates[j].wastePerReplica); comp != 0 {
			return comp < 0 // Less waste is better
		}
		// Secondary sort (tie-breaker): fewer GPUs per replica is better
		return candidates[i].gpusPerReplica < candidates[j].gpusPerReplica
	})

	bestTierVram := candidates[0].vramPerGpu.DeepCopy()
	return &bestTierVram
}

type SchedulingReason string

const (
	SchedulableType = "Schedulable"

	Schedulable SchedulingReason = "Schedulable"

	ResourceMismatch              SchedulingReason = "ResourceMismatch"
	UnschedulableNoGPUs           SchedulingReason = "NoGPUs"
	UnschedulableInsufficientGPUs SchedulingReason = "InsufficientGPUs"

	UnschedulableWrongQueueNamespace  SchedulingReason = "WrongQueueNamespace"
	UnschedulableClusterQueueNotFound SchedulingReason = "ClusterQueueNotFound"
)

func GetSchedulableCondition(ctx context.Context, clusterCtx ClusterContext, workload KaiwoWorkload) metav1.Condition {
	queueConfigCondition := getQueueConfigCondition(ctx, clusterCtx, workload)

	if queueConfigCondition.Status == metav1.ConditionFalse {
		return queueConfigCondition
	}
	gpuSchedulableCondition := getGpuSchedulableCondition(ctx, clusterCtx, workload)

	// TODO add another reason that indicates the workload can be scheduled, but not immediately?
	// TODO add checks for other cluster resources?

	return gpuSchedulableCondition
}

func getQueueConfigCondition(ctx context.Context, clusterCtx ClusterContext, workload KaiwoWorkload) metav1.Condition {
	kaiwoQueueConfig := clusterCtx.KaiwoQueueConfig
	clusterQueueName := GetClusterQueueName(ctx, workload)
	workloadNamespace := workload.GetKaiwoWorkloadObject().GetNamespace()
	for _, clusterQueue := range kaiwoQueueConfig.Spec.ClusterQueues {
		if clusterQueue.Name == clusterQueueName {
			if len(clusterQueue.Namespaces) == 0 {
				return metav1.Condition{
					Type:    SchedulableType,
					Status:  metav1.ConditionTrue,
					Reason:  string(Schedulable),
					Message: "Cluster queue has no namespace restrictions",
				}
			}
			for _, namespace := range clusterQueue.Namespaces {
				if namespace == workloadNamespace {
					return metav1.Condition{
						Type:    SchedulableType,
						Status:  metav1.ConditionTrue,
						Reason:  string(Schedulable),
						Message: "Cluster queue namespace matches",
					}
				}
			}
			return metav1.Condition{
				Type:    SchedulableType,
				Status:  metav1.ConditionFalse,
				Reason:  string(UnschedulableWrongQueueNamespace),
				Message: fmt.Sprintf("Cluster queue '%s' defines one or more namespaces, but workload namespace '%s' is not one of them", clusterQueueName, workloadNamespace),
			}
		}
	}
	return metav1.Condition{
		Type:    SchedulableType,
		Status:  metav1.ConditionFalse,
		Reason:  string(UnschedulableClusterQueueNotFound),
		Message: fmt.Sprintf("Cluster queue '%s' is not defined in KaiwoQueueConfig '%s'", clusterQueueName, kaiwoQueueConfig.Name),
	}
}

func getGpuSchedulableCondition(ctx context.Context, clusterCtx ClusterContext, workload KaiwoWorkload) metav1.Condition {
	spec := workload.GetCommonSpec()
	if !spec.GpuResources.IsGpuRequested() {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionTrue,
			Reason:  string(Schedulable),
			Message: "Workload does not require GPU resources",
		}
	}
	_, err := CalculateGpuRequirements(ctx, clusterCtx, spec.GpuResources, spec.Replicas)
	if err != nil {
		return metav1.Condition{
			Type:    SchedulableType,
			Status:  metav1.ConditionFalse,
			Reason:  string(ResourceMismatch),
			Message: err.Error(),
		}
	}

	return metav1.Condition{
		Type:    SchedulableType,
		Status:  metav1.ConditionTrue,
		Reason:  string(Schedulable),
		Message: "Cluster meets GPU resource requests",
	}
}
