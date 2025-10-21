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
	corev1 "k8s.io/api/core/v1"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
)

// PlanObservation holds the observed state for a PartitioningPlan.
type PlanObservation struct {
	// MatchingNodes are the nodes selected by the plan's selectors.
	MatchingNodes []corev1.Node

	// ExistingChildren are the NodePartitioning resources owned by this plan.
	ExistingChildren []infrastructurev1alpha1.NodePartitioning

	// RuleMatches tracks which rule indexes match each node.
	RuleMatches map[string][]int

	// ConflictingNodePartitionings are NodePartitioning resources that target the same node
	// but are associated with other plans.
	ConflictingNodePartitionings map[string][]infrastructurev1alpha1.NodePartitioning
}

// NodePartitioningObservation holds the observed state for a NodePartitioning.
type NodePartitioningObservation struct {
	// Node is the target node, if it exists.
	Node *corev1.Node

	// DCMConfigMap is the AMD GPU Operator DCM ConfigMap.
	DCMConfigMap *corev1.ConfigMap

	// DevicePluginReady indicates whether the AMD device plugin is running on the node.
	DevicePluginReady bool
}
