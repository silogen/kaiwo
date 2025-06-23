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

// cluster.go defines the functionality to inspect cluster resources and to
// update node labels in the cluster

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	errors2 "k8s.io/apimachinery/pkg/api/errors"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

var ErrNoSuitableNode = errors.New("no suitable node found")

type NodeType string

const (
	NodeTypeLabelKey             = KaiwoLabelBase + "node.type"
	NodeGpusPartitionedLabelKey  = KaiwoLabelBase + "node.gpu.partitioned"
	NodeGpuVendorLabelKey        = KaiwoLabelBase + "node.gpu.vendor"
	NodeGpuModelLabelKey         = KaiwoLabelBase + "node.gpu.model"
	NodeGpuPhysicalCountLabelKey = KaiwoLabelBase + "node.gpu"
	NodeGpuPhysicalVramLabelKey  = KaiwoLabelBase + "node.gpu.vram"
	NodeGpuLogicalCountLabelKey  = KaiwoLabelBase + "node.gpu.logical"
	NodeGpuLogicalVramLabelKey   = KaiwoLabelBase + "node.gpu.logical.vram"
	NodeGpuResourceNameLabelKey  = KaiwoLabelBase + "node.gpu.resource-name"
	DefaultNodePoolLabelKey      = KaiwoLabelBase + "node.pool"

	NodeTypeCpuOnly NodeType = "cpu-only"
	NodeTypeGpu     NodeType = "gpu"
)

// ClusterContext provides context of the cluster and its resources to help build downstream objects
type ClusterContext struct {
	Nodes            []NodeInfo
	KaiwoQueueConfig *v1alpha1.KaiwoQueueConfig
}

// GetClusterContext gathers the cluster context
func GetClusterContext(ctx context.Context, k8sClient client.Client) (*ClusterContext, error) {
	config := ConfigFromContext(ctx)
	nodeList := &v1.NodeList{}
	if err := k8sClient.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	clusterCtx := ClusterContext{
		Nodes: []NodeInfo{},
	}
	for _, node := range nodeList.Items {
		if config.Nodes.ExcludeMasterNodesFromNodePools && IsControlPlaneNode(node) {
			continue
		}
		info, err := ExtractLabeledNodeInfo(node)
		if err != nil {
			return nil, fmt.Errorf("failed to extract node resources: %w", err)
		}
		clusterCtx.Nodes = append(clusterCtx.Nodes, *info)
	}

	kaiwoQueueConfig := &v1alpha1.KaiwoQueueConfig{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: KaiwoQueueConfigName}, kaiwoQueueConfig); err != nil {
		if !errors2.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get KaiwoQueueConfig: %w", err)
		}
	} else {
		clusterCtx.KaiwoQueueConfig = kaiwoQueueConfig
	}

	return &clusterCtx, nil
}

// NodeInfo aggregates relevant information about a node
type NodeInfo struct {
	Node    v1.Node
	Type    NodeType
	GpuInfo *NodeGpuInfo
}

// ExtractLabeledNodeInfo extracts the NodeInfo from a Kaiwo-labeled node
func ExtractLabeledNodeInfo(node v1.Node) (*NodeInfo, error) {
	info := &NodeInfo{
		Node: node,
		Type: NodeType(node.Labels[NodeTypeLabelKey]),
	}

	if info.Type == "" {
		return nil, fmt.Errorf("no node type label found for node %s, ensure nodes are correctly labeled", node.Name)
	}

	if info.Type == CPUOnly {
		return info, nil
	}

	gpuInfo := NodeGpuInfo{}

	vendor, exists := node.Labels[NodeGpuVendorLabelKey]
	if !exists {
		return nil, fmt.Errorf("no vendor label '%s' for node %s", NodeGpuLogicalVramLabelKey, node.Name)
	}
	gpuInfo.Vendor = v1alpha1.GpuVendor(vendor)

	gpuInfo.Model, exists = node.Labels[NodeGpuModelLabelKey]
	if !exists {
		return nil, fmt.Errorf("no model label '%s' found for node %s", NodeGpuModelLabelKey, node.Name)
	}

	gpuInfo.ResourceName = VendorToResourceName(gpuInfo.Vendor)

	var err error

	gpuInfo.PhysicalCount, err = baseutils.ExtractAndConvertLabel(node.Labels, NodeGpuPhysicalCountLabelKey, strconv.Atoi)
	if err != nil {
		return nil, err
	}
	gpuInfo.LogicalCount, err = baseutils.ExtractAndConvertLabel(node.Labels, NodeGpuLogicalCountLabelKey, strconv.Atoi)
	if err != nil {
		return nil, err
	}

	gpuInfo.PhysicalVramPerGpu, err = baseutils.ExtractAndConvertLabel(node.Labels, NodeGpuPhysicalVramLabelKey, resource.ParseQuantity)
	if err != nil {
		return nil, err
	}
	gpuInfo.LogicalVramPerGpu, err = baseutils.ExtractAndConvertLabel(node.Labels, NodeGpuLogicalVramLabelKey, resource.ParseQuantity)
	if err != nil {
		return nil, err
	}

	info.GpuInfo = &gpuInfo

	return info, nil
}

func (info NodeInfo) IsControlPlane() bool {
	return IsControlPlaneNode(info.Node)
}

func IsControlPlaneNode(node v1.Node) bool {
	if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
		return true
	}
	if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
		return true
	}
	return false
}

func (info NodeInfo) IsUnschedulable() bool {
	return info.Node.Spec.Unschedulable
}

func (info NodeInfo) GetCPU() *resource.Quantity {
	return info.Node.Status.Capacity.Cpu()
}

func (info NodeInfo) GetMemory() *resource.Quantity {
	return info.Node.Status.Capacity.Memory()
}

func (info NodeInfo) GetNominalCPU() *resource.Quantity {
	origMilli := info.GetCPU().MilliValue()
	scaledMilli := (origMilli * 9) / 10 // integer math
	return resource.NewMilliQuantity(scaledMilli, resource.DecimalSI)
}

func (info NodeInfo) GetNominalMemory() *resource.Quantity {
	origBytes := info.GetMemory().Value()
	scaledBytes := (origBytes * 9) / 10 // integer math
	return resource.NewQuantity(scaledBytes, resource.BinarySI)
}

func (info NodeInfo) IsGpuPartitioned() bool {
	return info.GpuInfo != nil && info.GpuInfo.IsPartitioned()
}

// GetFlavorName returns the Kueue flavor name that this node should belong to
func (info NodeInfo) GetFlavorName() string {
	var components []string
	gpuInfo := info.GpuInfo
	if info.IsCpuOnlyNode() {
		components = append(components, "cpu-only")
	} else {
		model := strings.ReplaceAll(gpuInfo.ModelCleaned(), "-", "")
		gpuComponent := fmt.Sprintf("%dgpu.%s.%s", gpuInfo.LogicalCount, string(gpuInfo.Vendor), model)
		if gpuInfo.IsPartitioned() {
			gpuComponent += ".partitioned"
		}
		gpuComponent += "." + strings.ToLower(baseutils.QuantityToGi(gpuInfo.LogicalVramPerGpu))
		components = append(components, gpuComponent)
	}
	components = append(components, strconv.Itoa(int(info.GetNominalCPU().Value()))+"core")
	components = append(components, strings.ToLower(baseutils.QuantityToGi(*info.GetNominalMemory())))
	return strings.Join(components, "-")
}

func (info NodeInfo) GetType() NodeType {
	if info.GpuInfo == nil {
		return NodeTypeCpuOnly
	}
	return NodeTypeGpu
}

// IsCpuOnlyNode returns true, if the node does not have any GPUs
func (info NodeInfo) IsCpuOnlyNode() bool {
	return info.GpuInfo == nil
}

// extractRawNodeResources extracts the node resources from system labels
func extractRawNodeResources(node v1.Node) (*NodeInfo, error) {
	gpuInfo, err := extractNodeGpuInfo(node)
	if err != nil {
		return nil, fmt.Errorf("failed to extract node gpu info: %w", err)
	}
	info := &NodeInfo{
		Node:    node,
		GpuInfo: gpuInfo,
	}
	if gpuInfo == nil {
		info.Type = NodeTypeCpuOnly
	} else {
		info.Type = NodeTypeGpu
	}
	return info, nil
}

type NodeGpuInfo struct {
	// Vendor is the vendor of the GPU (primarily `amd` or `nvidia`)
	Vendor v1alpha1.GpuVendor

	// Model is the model name of the GPU
	Model string

	// ResourceName is the GPU resource name for this GPU
	ResourceName v1.ResourceName

	// PhysicalCount is the number of physical GPUs in the node
	PhysicalCount int

	// PhysicalVramPerGpu is the vRAM that each physical GPU has.
	PhysicalVramPerGpu resource.Quantity

	// LogicalCount is the number of logical GPUs in the node. If the GPUs are not partitioned, this is the same as PhysicalCount
	LogicalCount int

	// LogicalVramPerGpu is the vRAM that each logical GPU sees. If the node's GPUs are not partitioned, this is the
	// total vRAM that each GPU has. If the node's GPUs are partitioned, this is the vRAM that each partition has.
	LogicalVramPerGpu resource.Quantity
}

func (gpuInfo NodeGpuInfo) IsPartitioned() bool {
	return gpuInfo.PhysicalCount != gpuInfo.LogicalCount
}

// ModelCleaned returns the cleaned model name (compliant with RFC1123)
func (gpuInfo NodeGpuInfo) ModelCleaned() string {
	return baseutils.MakeRFC1123Compliant(gpuInfo.Model)
}

const (
	NodePartitionedGpusTaint = KaiwoLabelBase + "partitioned-gpu"
)

// EnsureClusterNodesLabelsAndTaints ensures that each node in the cluster is labeled correctly for Kaiwo usage
func EnsureClusterNodesLabelsAndTaints(ctx context.Context, c client.Client) error {
	nodeList := &v1.NodeList{}
	if err := c.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	for _, node := range nodeList.Items {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := c.Get(ctx, client.ObjectKeyFromObject(&node), &node)
			if err != nil {
				return fmt.Errorf("failed to get node '%s': %w", node.Name, err)
			}
			if err := EnsureNodeLabelsAndTaints(ctx, c, node); err != nil {
				return fmt.Errorf("failed to ensure cluster nodes: %w", err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to ensure labels and taints on node '%s': %w", node.Name, err)
		}
	}
	return nil
}

// EnsureNodeLabelsAndTaints ensures that a node is labeled correctly for Kaiwo usage
func EnsureNodeLabelsAndTaints(ctx context.Context, c client.Client, node v1.Node) error {
	config := ConfigFromContext(ctx)

	nodeInfo, err := extractRawNodeResources(node)
	if err != nil {
		return fmt.Errorf("failed to extract node resource info for node '%s': %w", node.Name, err)
	}

	ensureNodeLabels(*nodeInfo, &node)
	ensureNodeTaints(config, *nodeInfo, &node)

	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to update node labels: %w", err)
	}

	return nil
}

func ensureNodeLabels(nodeResourceInfo NodeInfo, node *v1.Node) {
	labels := node.Labels

	labels[NodeTypeLabelKey] = string(nodeResourceInfo.Type)
	labels[DefaultNodePoolLabelKey] = nodeResourceInfo.GetFlavorName()

	if nodeResourceInfo.Type == CPUOnly {
		return
	}

	gpuInfo := nodeResourceInfo.GpuInfo

	if nodeResourceInfo.IsGpuPartitioned() {
		labels[NodeGpusPartitionedLabelKey] = True
	} else {
		labels[NodeGpusPartitionedLabelKey] = False
	}

	labels[NodeGpuVendorLabelKey] = string(gpuInfo.Vendor)
	labels[NodeGpuModelLabelKey] = gpuInfo.ModelCleaned()

	labels[NodeGpuPhysicalCountLabelKey] = strconv.Itoa(gpuInfo.PhysicalCount)
	labels[NodeGpuLogicalCountLabelKey] = strconv.Itoa(gpuInfo.LogicalCount)

	labels[NodeGpuPhysicalVramLabelKey] = baseutils.QuantityToGi(gpuInfo.PhysicalVramPerGpu)
	labels[NodeGpuLogicalVramLabelKey] = baseutils.QuantityToGi(gpuInfo.LogicalVramPerGpu)

	node.SetLabels(labels)
}

func ensureNodeTaints(config KaiwoConfigContext, nodeResourceInfo NodeInfo, node *v1.Node) {
	var taints []v1.Taint

	if config.Nodes.AddTaintsToGpuNodes && !nodeResourceInfo.IsCpuOnlyNode() {
		gpuTaint := v1.Taint{
			Key:    config.Nodes.DefaultGpuTaintKey,
			Value:  True,
			Effect: v1.TaintEffectNoSchedule,
		}
		taints = append(taints, gpuTaint)

		if nodeResourceInfo.IsGpuPartitioned() {
			taints = append(taints, v1.Taint{
				Key:    NodePartitionedGpusTaint,
				Value:  True,
				Effect: v1.TaintEffectNoSchedule,
			})
		}
	}

	for _, taint := range taints {
		addTaint := true
		for _, t := range node.Spec.Taints {
			if t.MatchTaint(&taint) {
				addTaint = false
				break
			}
		}
		if addTaint {
			node.Spec.Taints = append(node.Spec.Taints, taint)
		}
	}
}

// The code below is for extracting information from the AMD / NVIDIA system labels

const (
	AmdProductNameLabelKey      = "amd.com/gpu.product-name"
	AmdGpuFamilyAiCountLabelKey = "beta.amd.com/gpu.family.AI"
	AmdGpuVramLabelKey          = "amd.com/gpu.vram"

	NvidiaProductNameLabelKey    = "nvidia.com/gpu.product"
	NvidiaGpuCountLabelKey       = "nvidia.com/gpu.count"
	NvidiaGpuTotalMemoryLabelKey = "nvidia.com/gpu.memory.total"
	NvidiaMigEnabledLabelKey     = "nvidia.com/mig.enabled"
	NvidiaMigCountLabelKey       = "nvidia.com/gpu.mig.count"
	NvidiaMigProfileLabelKey     = "nvidia.com/gpu.mig.profile"
)

func extractNodeGpuInfo(node v1.Node) (*NodeGpuInfo, error) {
	if _, exists := node.Labels[AmdProductNameLabelKey]; exists {
		return extractAmdNodeGpuInfo(node)
	} else if _, exists := node.Labels[NvidiaProductNameLabelKey]; exists {
		return extractNvidiaNodeGpuInfo(node)
	}
	return nil, nil
}

func extractAmdNodeGpuInfo(node v1.Node) (*NodeGpuInfo, error) {
	nodeLabels := node.Labels

	productName, exists := nodeLabels[AmdProductNameLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract product name from labels, label %s does not exist", AmdProductNameLabelKey)
	}

	physicalGpusCount, exists := nodeLabels[AmdGpuFamilyAiCountLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract physical GPU count from labels, label %s does not exist", AmdGpuFamilyAiCountLabelKey)
	}
	physicalGpus, err := strconv.Atoi(physicalGpusCount)
	if err != nil {
		return nil, fmt.Errorf("failed to parse physical GPU count: %w", err)
	}

	physicalVramCount, exists := nodeLabels[AmdGpuVramLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract logical GPU vRAM from labels, label %s does not exist", AmdGpuVramLabelKey)
	}
	physicalVram, err := resource.ParseQuantity(physicalVramCount + "i")
	if err != nil {
		return nil, fmt.Errorf("failed to extract logical VRAM from labels: %w", err)
	}

	logicalCount, exists := node.Status.Capacity[AmdGpuResourceName]
	if !exists {
		return nil, fmt.Errorf("failed to extract logical GPU count from capacity, node does not have the resource '%s' listed in its capacity", AmdGpuResourceName)
	}

	logicalVram := physicalVram.DeepCopy()
	partitionFactor := int(logicalCount.Value()) / physicalGpus
	logicalVram.Mul(int64(partitionFactor))

	return &NodeGpuInfo{
		Vendor:             v1alpha1.GpuVendorAmd,
		ResourceName:       AmdGpuResourceName,
		Model:              cleanAMDGPUName(productName),
		PhysicalVramPerGpu: physicalVram,
		PhysicalCount:      physicalGpus,
		LogicalCount:       int(logicalCount.Value()),
		LogicalVramPerGpu:  logicalVram,
	}, nil
}

func cleanAMDGPUName(gpuID string) string {
	gpuType := strings.ToLower(gpuID)

	unwanted := []string{
		"instinct",
		"radeon",
		"_",
		"oam",
		"amd",
		"series",
		"gpu",
	}

	for _, word := range unwanted {
		gpuType = strings.ReplaceAll(gpuType, word, "")
	}

	return strings.TrimSpace(gpuType)
}

func extractNvidiaNodeGpuInfo(node v1.Node) (*NodeGpuInfo, error) {
	nodeLabels := node.Labels

	productName, exists := nodeLabels[NvidiaProductNameLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract product name from labels, label %s does not exist", NvidiaProductNameLabelKey)
	}

	physicalGpusCount, exists := nodeLabels[NvidiaGpuCountLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract physical GPU count from labels, label %s does not exist", NvidiaGpuCountLabelKey)
	}
	physicalGpus, err := strconv.Atoi(physicalGpusCount)
	if err != nil {
		return nil, fmt.Errorf("failed to parse physical GPU count: %w", err)
	}

	gpuTotalVramValue, exists := nodeLabels[NvidiaGpuTotalMemoryLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract vRAM from labels, label %s does not exist", NvidiaGpuTotalMemoryLabelKey)
	}
	vramPerGpu, err := resource.ParseQuantity(gpuTotalVramValue + "Mi")
	if err != nil {
		return nil, fmt.Errorf("failed to parse vRAM from labels: %w", err)
	}
	physicalVramPerGpu := vramPerGpu

	resourceName := NvidiaGpuResourceName

	logicalGpus := physicalGpus

	migEnabled, exists := nodeLabels[NvidiaMigEnabledLabelKey]
	if exists && migEnabled == True {
		migProfile, exists := nodeLabels[NvidiaMigProfileLabelKey]
		if !exists {
			return nil, fmt.Errorf("failed to extract MIG profile, label %s does not exist", NvidiaMigProfileLabelKey)
		}
		resourceName = v1.ResourceName("nvidia.com/mig-" + migProfile)
		migSplit := strings.SplitN(migProfile, ".", 2)
		if len(migSplit) != 2 {
			return nil, fmt.Errorf("failed to parse MIG profile '%s'", migProfile)
		}
		migLogicalVram, err := resource.ParseQuantity(strings.ReplaceAll(migSplit[1], "gb", "Gi"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse MIG logical VRAM '%s': %w", migProfile, err)
		}
		vramPerGpu = migLogicalVram

		migCountValue, exists := nodeLabels[NvidiaMigCountLabelKey]
		if !exists {
			return nil, fmt.Errorf("failed to extract MIG count, label %s does not exist", NvidiaMigCountLabelKey)
		}
		migCount, err := strconv.Atoi(migCountValue)
		if err != nil {
			return nil, fmt.Errorf("failed to parse MIG count %s: %w", migCountValue, err)
		}
		logicalGpus *= migCount
	}

	return &NodeGpuInfo{
		Vendor:             v1alpha1.GpuVendorNvidia,
		ResourceName:       resourceName,
		Model:              productName,
		PhysicalCount:      physicalGpus,
		PhysicalVramPerGpu: physicalVramPerGpu,
		LogicalCount:       logicalGpus,
		LogicalVramPerGpu:  vramPerGpu,
	}, nil
}
