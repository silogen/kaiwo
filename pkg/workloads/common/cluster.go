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
	"math"
	"strconv"
	"strings"

	errors2 "k8s.io/apimachinery/pkg/api/errors"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

var ErrNoSuitableNode = errors.New("no suitable node found")

type NodeType string

const (
	NodeTypeLabelKey             = KaiwoLabelBase + "/node.type"
	NodeGpusPartitionedLabelKey  = KaiwoLabelBase + "/node.gpu.partitioned"
	NodeStatusLabelKey           = KaiwoLabelBase + "/node.status"
	NodeGpuVendorLabelKey        = KaiwoLabelBase + "/node.gpu.vendor"
	NodeGpuModelLabelKey         = KaiwoLabelBase + "/node.gpu.model"
	NodeGpuPhysicalCountLabelKey = KaiwoLabelBase + "/node.gpu"
	NodeGpuPhysicalVramLabelKey  = KaiwoLabelBase + "/node.gpu.vram"
	NodeGpuLogicalCountLabelKey  = KaiwoLabelBase + "/node.gpu.logical"
	NodeGpuLogicalVramLabelKey   = KaiwoLabelBase + "/node.gpu.logical.vram"
	DefaultNodePoolLabelKey      = KaiwoLabelBase + "/node.pool"

	NodeTypeCpuOnly NodeType = "cpu-only"
	NodeTypeGpu     NodeType = "gpu"
)

// ClusterContext provides context of the cluster and its resources to help build downstream objects
type ClusterContext struct {
	Nodes            []v1alpha1.KaiwoNode
	KaiwoQueueConfig *v1alpha1.KaiwoQueueConfig
}

// GetClusterContext gathers the cluster context
func GetClusterContext(ctx context.Context, k8sClient client.Client) (*ClusterContext, error) {
	config := ConfigFromContext(ctx)
	nodeList := &v1alpha1.KaiwoNodeList{}
	if err := k8sClient.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	clusterCtx := ClusterContext{
		Nodes: []v1alpha1.KaiwoNode{},
	}
	for _, node := range nodeList.Items {
		if config.Nodes.ExcludeMasterNodesFromNodePools && node.Status.IsControlPlane {
			continue
		}
		clusterCtx.Nodes = append(clusterCtx.Nodes, node)
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
	GpuInfo *v1alpha1.NodeGpuInfo
}

//
//// ExtractLabeledNodeInfo extracts the NodeInfo from a Kaiwo-labeled node
//func ExtractLabeledNodeInfo(node v1.Node) (*NodeInfo, error) {
//	info := &NodeInfo{
//		Node: node,
//		Type: NodeType(node.Labels[NodeTypeLabelKey]),
//	}
//
//	if info.Type == "" {
//		return nil, fmt.Errorf("no node type label found for node %s, ensure nodes are correctly labeled", node.Name)
//	}
//
//	if info.Type == CPUOnly {
//		return info, nil
//	}
//
//	gpuInfo := NodeGpuInfo{}
//
//	vendor, exists := node.Labels[NodeGpuVendorLabelKey]
//	if !exists {
//		return nil, fmt.Errorf("no vendor label '%s' for node %s", NodeGpuLogicalVramLabelKey, node.Name)
//	}
//	gpuInfo.Vendor = v1alpha1.GpuVendor(vendor)
//
//	gpuInfo.Model, exists = node.Labels[NodeGpuModelLabelKey]
//	if !exists {
//		return nil, fmt.Errorf("no model label '%s' found for node %s", NodeGpuModelLabelKey, node.Name)
//	}
//
//	gpuInfo.ResourceName = VendorToResourceName(gpuInfo.Vendor)
//
//	var err error
//
//	gpuInfo.PhysicalCount, err = baseutils.ExtractAndConvertLabelIfExists(node.Labels, NodeGpuPhysicalCountLabelKey, strconv.Atoi)
//	if err != nil {
//		return nil, err
//	}
//	gpuInfo.LogicalCount, err = baseutils.ExtractAndConvertLabel(node.Labels, NodeGpuLogicalCountLabelKey, strconv.Atoi)
//	if err != nil {
//		return nil, err
//	}
//
//	gpuInfo.PhysicalVramPerGpu, err = baseutils.ExtractAndConvertLabelIfExists(node.Labels, NodeGpuPhysicalVramLabelKey, resource.ParseQuantity)
//	if err != nil {
//		return nil, err
//	}
//	gpuInfo.LogicalVramPerGpu, err = baseutils.ExtractAndConvertLabelIfExists(node.Labels, NodeGpuLogicalVramLabelKey, resource.ParseQuantity)
//	if err != nil {
//		return nil, err
//	}
//
//	info.GpuInfo = &gpuInfo
//
//	return info, nil
//}

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

// GetFlavorName returns the Kueue flavor name that this node should belong to
func (info NodeInfo) GetFlavorName() string {
	var components []string
	gpuInfo := info.GpuInfo
	if info.IsCpuOnlyNode() {
		components = append(components, "cpu-only")
	} else {
		model := strings.ReplaceAll(gpuInfo.Model, "-", "")
		gpuComponent := fmt.Sprintf("%dgpu.%s.%s", gpuInfo.LogicalCount, string(gpuInfo.Vendor), model)
		if partitioned := gpuInfo.IsPartitioned; partitioned != nil && *partitioned {
			gpuComponent += ".partitioned"
		}
		if gpuInfo.LogicalVramPerGpu != nil {
			gpuComponent += "." + strings.ToLower(baseutils.QuantityToGi(*gpuInfo.LogicalVramPerGpu))
		}
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

// ExtractRawNodeResources extracts the node resources from system labels
func ExtractRawNodeResources(node v1.Node) (*NodeInfo, error) {
	gpuInfo, err := ExtractNodeGpuInfo(node)
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
	// Mandatory fields, parsing a node will fail if the following fields cannot be extracted

	// Vendor is the vendor of the GPU (primarily `amd` or `nvidia`)
	Vendor v1alpha1.GpuVendor

	// Model is the model name of the GPU
	Model string

	// ResourceName is the GPU resource name for this GPU
	ResourceName v1.ResourceName

	// LogicalCount is the number of logical GPUs in the node and is taken from the Kubernetes node status capacity. If the GPUs are not partitioned, this is the same as PhysicalCount
	LogicalCount int

	// Optional fields, some features may not be available if the following fields are not set

	// PhysicalCount is the number of physical GPUs in the node
	PhysicalCount *int

	// PhysicalVramPerGpu is the vRAM that each physical GPU has.
	PhysicalVramPerGpu *resource.Quantity

	// LogicalVramPerGpu is the vRAM that each logical GPU sees. If the node's GPUs are not partitioned, this is the
	// total vRAM that each GPU has. If the node's GPUs are partitioned, this is the vRAM that each partition has.
	LogicalVramPerGpu *resource.Quantity
}

// IsPartitioned tells if a node's GPUs are partitioned or not. If it is unclear (missing labels),
// nil is returned.
func (gpuInfo NodeGpuInfo) IsPartitioned() *bool {
	if gpuInfo.PhysicalCount == nil {
		return nil
	}
	return baseutils.Pointer(*gpuInfo.PhysicalCount != gpuInfo.LogicalCount)
}

// ModelCleaned returns the cleaned model name (compliant with RFC1123)
func (gpuInfo NodeGpuInfo) ModelCleaned() string {
	return baseutils.MakeRFC1123Compliant(gpuInfo.Model)
}

const (
	NodePartitionedGpusTaint = KaiwoLabelBase + "partitioned-gpu"
)

// The code below is for extracting information from the AMD / NVIDIA system labels

const (
	AmdProductNameLabelKey      = "amd.com/gpu.product-name"
	AmdGpuFamilyAiCountLabelKey = "beta.amd.com/gpu.family.AI"
	AmdGpuVramLabelKey          = "amd.com/gpu.vram"

	NvidiaProductNameLabelKey = "nvidia.com/gpu.product"
	// NvidiaGpuCountLabelKey       = "nvidia.com/gpu.count"
	// NvidiaGpuTotalMemoryLabelKey = "nvidia.com/gpu.memory.total"
	// NvidiaMigEnabledLabelKey     = "nvidia.com/mig.enabled"
	// NvidiaMigCountLabelKey       = "nvidia.com/gpu.mig.count"
	// NvidiaMigProfileLabelKey     = "nvidia.com/gpu.mig.profile"
)

func ExtractNodeGpuInfo(node v1.Node) (*v1alpha1.NodeGpuInfo, error) {
	if _, exists := node.Labels[AmdProductNameLabelKey]; exists {
		return extractAmdNodeGpuInfo(node)
	} else if _, exists := node.Labels[NvidiaProductNameLabelKey]; exists {
		return extractNvidiaNodeGpuInfo(node)
	}
	return nil, nil
}

func extractAmdNodeGpuInfo(node v1.Node) (*v1alpha1.NodeGpuInfo, error) {
	labels := node.Labels

	productName, ok := labels[AmdProductNameLabelKey]
	if !ok {
		return nil, fmt.Errorf("missing label %q", AmdProductNameLabelKey)
	}

	// logical count (total logical GPUs seen by K8s)
	capQty, ok := node.Status.Capacity[AmdGpuResourceName]
	if !ok {
		return nil, fmt.Errorf("node has no capacity entry %q", AmdGpuResourceName)
	}
	logicalCount := int(capQty.Value())

	// 3) physical GPU count (e.g. 8 on an MI300X)
	physGPUPtr, err := baseutils.ExtractAndConvertLabelIfExists(
		labels, AmdGpuFamilyAiCountLabelKey, strconv.Atoi,
	)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", AmdGpuFamilyAiCountLabelKey, err)
	}

	// 4) logical VRAM per *logical* GPU (e.g. 24Gi)
	logicalVramPtr, err := baseutils.ExtractAndConvertLabelIfExists(
		labels, AmdGpuVramLabelKey,
		func(s string) (resource.Quantity, error) { return resource.ParseQuantity(s + "i") },
	)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", AmdGpuVramLabelKey, err)
	}

	// 5) compute physical VRAM per *physical* GPU by:
	//      partitionFactor = ceil(logicalCount / physicalGpus)
	//      physicalVram = logicalVram * partitionFactor
	var physicalVramPtr *resource.Quantity
	if physGPUPtr != nil && logicalVramPtr != nil {
		pf := int(math.Ceil(float64(logicalCount) / float64(*physGPUPtr)))
		q := logicalVramPtr.DeepCopy()
		q.Mul(int64(pf))
		physicalVramPtr = baseutils.Pointer(q)
	}

	var isPartitioned *bool = nil
	if physGPUPtr != nil {
		isPartitioned = baseutils.Pointer(*physGPUPtr != logicalCount)
	}

	return &v1alpha1.NodeGpuInfo{
		Vendor:             v1alpha1.GpuVendorAmd,
		ResourceName:       AmdGpuResourceName,
		Model:              cleanAMDGPUName(productName),
		PhysicalCount:      physGPUPtr,
		LogicalCount:       logicalCount,
		PhysicalVramPerGpu: physicalVramPtr,
		LogicalVramPerGpu:  logicalVramPtr,
		IsPartitioned:      isPartitioned,
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

	return baseutils.MakeRFC1123Compliant(strings.TrimSpace(gpuType))
}

// extractNvidiaNodeGpuInfo extracts Nvidia node information
// currently only supports parsing the logical GPU count
func extractNvidiaNodeGpuInfo(node v1.Node) (*v1alpha1.NodeGpuInfo, error) {
	nodeLabels := node.Labels

	// Mandatory info

	productName, exists := nodeLabels[NvidiaProductNameLabelKey]
	if !exists {
		return nil, fmt.Errorf("failed to extract product name from labels, label %s does not exist", NvidiaProductNameLabelKey)
	}

	logicalGpusCount, exists := node.Status.Capacity[NvidiaGpuResourceName]
	if !exists {
		return nil, fmt.Errorf("failed to extract NVIDIA GPU node info, no NVIDIA resource present")
	}

	// Optional info

	//var physicalGpus *int = nil
	//if physicalGpusCount, exists := nodeLabels[NvidiaGpuCountLabelKey]; exists {
	//	gpus, err := strconv.Atoi(physicalGpusCount)
	//	if err != nil {
	//		return nil, fmt.Errorf("failed to parse physical GPU count: %w", err)
	//	}
	//	physicalGpus = baseutils.Pointer(gpus)
	//}
	//
	//gpuTotalVramValue, exists := nodeLabels[NvidiaGpuTotalMemoryLabelKey]
	//if !exists {
	//	return nil, fmt.Errorf("failed to extract vRAM from labels, label %s does not exist", NvidiaGpuTotalMemoryLabelKey)
	//}
	//vramPerGpu, err := resource.ParseQuantity(gpuTotalVramValue + "Mi")
	//if err != nil {
	//	return nil, fmt.Errorf("failed to parse vRAM from labels: %w", err)
	//}
	//physicalVramPerGpu := vramPerGpu
	//
	//resourceName := NvidiaGpuResourceName
	//
	//logicalGpus := physicalGpus
	//
	//migEnabled, exists := nodeLabels[NvidiaMigEnabledLabelKey]
	//if exists && migEnabled == True {
	//	migProfile, exists := nodeLabels[NvidiaMigProfileLabelKey]
	//	if !exists {
	//		return nil, fmt.Errorf("failed to extract MIG profile, label %s does not exist", NvidiaMigProfileLabelKey)
	//	}
	//	resourceName = v1.ResourceName("nvidia.com/mig-" + migProfile)
	//	migSplit := strings.SplitN(migProfile, ".", 2)
	//	if len(migSplit) != 2 {
	//		return nil, fmt.Errorf("failed to parse MIG profile '%s'", migProfile)
	//	}
	//	migLogicalVram, err := resource.ParseQuantity(strings.ReplaceAll(migSplit[1], "gb", "Gi"))
	//	if err != nil {
	//		return nil, fmt.Errorf("failed to parse MIG logical VRAM '%s': %w", migProfile, err)
	//	}
	//	vramPerGpu = migLogicalVram
	//
	//	migCountValue, exists := nodeLabels[NvidiaMigCountLabelKey]
	//	if !exists {
	//		return nil, fmt.Errorf("failed to extract MIG count, label %s does not exist", NvidiaMigCountLabelKey)
	//	}
	//	migCount, err := strconv.Atoi(migCountValue)
	//	if err != nil {
	//		return nil, fmt.Errorf("failed to parse MIG count %s: %w", migCountValue, err)
	//	}
	//	logicalGpus *= migCount
	//}

	return &v1alpha1.NodeGpuInfo{
		Vendor:       v1alpha1.GpuVendorNvidia,
		Model:        baseutils.MakeRFC1123Compliant(productName),
		LogicalCount: int(logicalGpusCount.Value()),
		ResourceName: NvidiaGpuResourceName,
	}, nil
}
