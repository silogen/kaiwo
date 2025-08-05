package nodeutils

import (
	"context"

	v1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads/common"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
)

// NodeLabelsAndTaintsTask is responsible for ensuring that the nodes are labeled and tainted correctly
type NodeLabelsAndTaintsTask struct {
	Client client.Client
}

func (t *NodeLabelsAndTaintsTask) Name() string { return "NodeLabelsAndTaints" }

func (t *NodeLabelsAndTaintsTask) Run(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	t.ensureNodeLabels(ctx, obj)
	return nil, nil
}

func (t *NodeLabelsAndTaintsTask) ensureNodeLabels(ctx context.Context, obj *KaiwoNodeWrapper) {
	config := common.ConfigFromContext(ctx)

	labels := obj.Node.GetLabels()
	labels[common.NodeTypeLabelKey] = string(obj.KaiwoNode.Status.NodeType)
	labels[common.DefaultNodePoolLabelKey] = obj.KaiwoNode.Status.KueueFlavorName

	if !obj.KaiwoNode.Status.IsControlPlane || !config.Nodes.ExcludeMasterNodesFromNodePools {
		labels[common.DefaultKaiwoWorkerLabel] = common.True
	}

	labels[common.NodeStatusLabelKey] = string(obj.KaiwoNode.Status.Status)

	if obj.KaiwoNode.Status.NodeType == v1alpha1.NodeTypeCpu {
		return
	}

	gpuInfo := obj.KaiwoNode.Status.Resources.Gpus

	if partitioned := gpuInfo.IsPartitioned; partitioned != nil && *partitioned {
		labels[common.NodeGpusPartitionedLabelKey] = common.True
	} else if partitioned != nil && !*partitioned {
		labels[common.NodeGpusPartitionedLabelKey] = common.False
	}

	labels[common.NodeGpuVendorLabelKey] = string(gpuInfo.Vendor)
	labels[common.NodeGpuModelLabelKey] = gpuInfo.Model
	// labels[NodeGpuLogicalCountLabelKey] = strconv.Itoa(gpuInfo.LogicalCount)

	//if gpuInfo.PhysicalCount != nil {
	//	labels[NodeGpuPhysicalCountLabelKey] = strconv.Itoa(*gpuInfo.PhysicalCount)
	//}
	//if gpuInfo.PhysicalVramPerGpu != nil {
	//	labels[NodeGpuPhysicalVramLabelKey] = baseutils.QuantityToGi(*gpuInfo.PhysicalVramPerGpu)
	//}
	if gpuInfo.LogicalVramPerGpu != nil {
		labels[common.NodeGpuLogicalVramLabelKey] = baseutils.QuantityToGi(*gpuInfo.LogicalVramPerGpu)
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
	config := common.ConfigFromContext(ctx)

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
				Key:    common.NodePartitionedGpusTaint,
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
