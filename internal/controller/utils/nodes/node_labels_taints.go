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

package nodeutils

import (
	"context"

	"github.com/silogen/kaiwo/pkg/common"

	"github.com/silogen/kaiwo/pkg/config"

	"github.com/silogen/kaiwo/pkg/cluster"

	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NodeLabelsAndTaintsTask is responsible for ensuring that the nodes are labeled and tainted correctly
type NodeLabelsAndTaintsTask struct {
	Client client.Client
}

func (t *NodeLabelsAndTaintsTask) Name() string { return "NodeLabelsAndTaints" }

func (t *NodeLabelsAndTaintsTask) Run(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	t.ensureNodeLabels(ctx, obj)
	t.ensureTaints(ctx, obj)
	return nil, nil
}

func (t *NodeLabelsAndTaintsTask) ensureNodeLabels(ctx context.Context, obj *KaiwoNodeWrapper) {
	cfg := config.ConfigFromContext(ctx)

	labels := obj.Node.GetLabels()
	labels[cluster.NodeTypeLabelKey] = string(obj.KaiwoNode.Status.NodeType)
	labels[cluster.DefaultNodePoolLabelKey] = obj.KaiwoNode.Status.KueueFlavorName

	if !obj.KaiwoNode.Status.IsControlPlane || !cfg.Nodes.ExcludeMasterNodesFromNodePools {
		labels[common.DefaultKaiwoWorkerLabel] = common.True
	}

	labels[cluster.NodeStatusLabelKey] = string(obj.KaiwoNode.Status.Status)

	if obj.KaiwoNode.Status.NodeType == v1alpha1.NodeTypeCpu {
		return
	}

	gpuInfo := obj.KaiwoNode.Status.Resources.Gpus

	if partitioned := gpuInfo.IsPartitioned; partitioned != nil && *partitioned {
		labels[cluster.NodeGpusPartitionedLabelKey] = common.True
	} else if partitioned != nil && !*partitioned {
		labels[cluster.NodeGpusPartitionedLabelKey] = common.False
	}

	labels[cluster.NodeGpuVendorLabelKey] = string(gpuInfo.Vendor)
	labels[cluster.NodeGpuModelLabelKey] = gpuInfo.Model

	if gpuInfo.LogicalVramPerGpu != nil {
		labels[cluster.NodeGpuLogicalVramLabelKey] = baseutils.QuantityToGi(*gpuInfo.LogicalVramPerGpu)
	}

	// Topology level defaults
	if _, exists := obj.Node.Labels[common.DefaultTopologyBlockLabel]; !exists {
		obj.Node.Labels[common.DefaultTopologyBlockLabel] = "block-a"
	}
	if _, exists := obj.Node.Labels[common.DefaultTopologyRackLabel]; !exists {
		obj.Node.Labels[common.DefaultTopologyRackLabel] = "rack-a"
	}

	obj.Node.SetLabels(labels)
}

func (t *NodeLabelsAndTaintsTask) ensureTaints(ctx context.Context, obj *KaiwoNodeWrapper) {
	config := config.ConfigFromContext(ctx)

	var taints []v1.Taint

	if config.Nodes.AddTaintsToGpuNodes && obj.KaiwoNode.Status.NodeType == v1alpha1.NodeTypeGpu {
		gpuTaint := v1.Taint{
			Key:    config.Nodes.DefaultGpuTaintKey,
			Value:  common.True,
			Effect: v1.TaintEffectNoSchedule,
		}
		taints = append(taints, gpuTaint)

		if partitioned := obj.KaiwoNode.Status.Resources.Gpus.IsPartitioned; partitioned != nil && *partitioned {
			taints = append(taints, v1.Taint{
				Key:    cluster.NodePartitionedGpusTaint,
				Value:  common.True,
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
