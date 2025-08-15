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

package nodes

import (
	"context"

	"github.com/silogen/kaiwo/pkg/platform/cluster"

	"github.com/silogen/kaiwo/pkg/runtime/config"

	common2 "github.com/silogen/kaiwo/pkg/runtime/common"

	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// UpdateNodeLabelsAndTaints ensures that the nodes are labeled and tainted correctly
func UpdateNodeLabelsAndTaints(ctx context.Context, client client.Client, obj *KaiwoNodeWrapper) {
	ensureNodeLabels(ctx, obj)
	ensureTaints(ctx, obj)
}

func ensureNodeLabels(ctx context.Context, obj *KaiwoNodeWrapper) {
	cfg := config.ConfigFromContext(ctx)

	labels := obj.Node.GetLabels()
	labels[cluster.NodeTypeLabelKey] = string(obj.KaiwoNode.Status.NodeType)
	labels[cluster.DefaultNodePoolLabelKey] = obj.KaiwoNode.Status.KueueFlavorName

	if !obj.KaiwoNode.Status.IsControlPlane || !cfg.Nodes.ExcludeMasterNodesFromNodePools {
		labels[common2.DefaultKaiwoWorkerLabel] = common2.True
	}

	labels[cluster.NodeStatusLabelKey] = string(obj.KaiwoNode.Status.Status)

	if obj.KaiwoNode.Status.NodeType == v1alpha1.NodeTypeCpu {
		return
	}

	gpuInfo := obj.KaiwoNode.Status.Resources.Gpus

	if partitioned := gpuInfo.IsPartitioned; partitioned != nil && *partitioned {
		labels[cluster.NodeGpusPartitionedLabelKey] = common2.True
	} else if partitioned != nil && !*partitioned {
		labels[cluster.NodeGpusPartitionedLabelKey] = common2.False
	}

	labels[cluster.NodeGpuVendorLabelKey] = string(gpuInfo.Vendor)
	if gpuInfo.Model != nil {
		labels[cluster.NodeGpuModelLabelKey] = *gpuInfo.Model
	}

	if gpuInfo.LogicalVramPerGpu != nil {
		labels[cluster.NodeGpuLogicalVramLabelKey] = baseutils.QuantityToGi(*gpuInfo.LogicalVramPerGpu)
	}

	// Topology level defaults
	if _, exists := obj.Node.Labels[common2.DefaultTopologyBlockLabel]; !exists {
		obj.Node.Labels[common2.DefaultTopologyBlockLabel] = "block-a"
	}
	if _, exists := obj.Node.Labels[common2.DefaultTopologyRackLabel]; !exists {
		obj.Node.Labels[common2.DefaultTopologyRackLabel] = "rack-a"
	}

	obj.Node.SetLabels(labels)
}

func ensureTaints(ctx context.Context, obj *KaiwoNodeWrapper) {
	config := config.ConfigFromContext(ctx)

	var taints []v1.Taint

	if config.Nodes.AddTaintsToGpuNodes && obj.KaiwoNode.Status.NodeType == v1alpha1.NodeTypeGpu {
		gpuTaint := v1.Taint{
			Key:    config.Nodes.DefaultGpuTaintKey,
			Value:  common2.True,
			Effect: v1.TaintEffectNoSchedule,
		}
		taints = append(taints, gpuTaint)

		if partitioned := obj.KaiwoNode.Status.Resources.Gpus.IsPartitioned; partitioned != nil && *partitioned {
			taints = append(taints, v1.Taint{
				Key:    cluster.NodePartitionedGpusTaint,
				Value:  common2.True,
				Effect: v1.TaintEffectNoSchedule,
			})
		}
	}

	for _, taint := range taints {
		addTaint := true
		for _, t := range obj.Node.Spec.Taints {
			if t.MatchTaint(&taint) {
				addTaint = false
				break
			}
		}
		if addTaint {
			obj.Node.Spec.Taints = append(obj.Node.Spec.Taints, taint)
		}
	}
}
