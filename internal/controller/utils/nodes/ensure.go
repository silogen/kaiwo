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
	"fmt"
	"strconv"

	"github.com/silogen/kaiwo/pkg/common"

	"github.com/silogen/kaiwo/pkg/cluster"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EnsureKaiwoNodeTask struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (t *EnsureKaiwoNodeTask) Name() string {
	return "EnsureKaiwoNode"
}

func (t *EnsureKaiwoNodeTask) Run(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	kaiwoNode, err := t.ensureKaiwoNode(ctx, obj.Node)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure Kaiwo node: %w", err)
	}
	if obj.KaiwoNode != nil {
		kaiwoNode.Status.Partitioning = obj.KaiwoNode.Status.Partitioning
		kaiwoNode.Status.Conditions = obj.KaiwoNode.Status.Conditions
	}
	obj.KaiwoNode = kaiwoNode
	return nil, nil
}

func (t *EnsureKaiwoNodeTask) ensureKaiwoNode(_ context.Context, node *corev1.Node) (*v1alpha1.KaiwoNode, error) {
	kaiwoNode := &v1alpha1.KaiwoNode{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KaiwoNode",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
	}
	if err := controllerutil.SetOwnerReference(node, kaiwoNode, t.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set Kaiwo node owner reference: %w", err)
	}

	// Update partitioning
	if value, exists := node.Labels[common.GpuPartitioningEnabledLabel]; exists {
		if parsedValue, err := strconv.ParseBool(value); err == nil {
			kaiwoNode.Spec.Partitioning.Enabled = parsedValue
		}
	}
	if value, exists := node.Labels[common.GpuPartitioningProfileLabel]; exists {
		kaiwoNode.Spec.Partitioning.Profile = v1alpha1.GpuPartitioningProfile(value)
	}

	// Update status
	var kaiwoNodeStatus v1alpha1.KaiwoNodeStatusType

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				kaiwoNodeStatus = v1alpha1.KaiwoNodeStatusReady
			} else {
				kaiwoNodeStatus = v1alpha1.KaiwoNodeStatusNotReady
			}
		}
	}
	kaiwoNode.Status.Status = kaiwoNodeStatus

	// Update resources
	nodeInfo, err := cluster.ExtractRawNodeResources(*node)
	if err != nil {
		return nil, fmt.Errorf("failed to extract raw node info: %w", err)
	}

	if nodeInfo.GpuInfo != nil {
		kaiwoNode.Status.NodeType = v1alpha1.NodeTypeGpu
	} else {
		kaiwoNode.Status.NodeType = v1alpha1.NodeTypeCpu
	}

	kaiwoNode.Status.Resources = v1alpha1.NodeResources{
		Memory: v1alpha1.NominalResourceWrapper{
			Nominal: nodeInfo.GetNominalMemory(),
			Actual:  nodeInfo.GetMemory(),
		},
		Cpu: v1alpha1.NominalResourceWrapper{
			Nominal: nodeInfo.GetNominalCPU(),
			Actual:  nodeInfo.GetCPU(),
		},
		Gpus: nodeInfo.GpuInfo,
	}

	kaiwoNode.Status.KueueFlavorName = nodeInfo.GetFlavorName()

	if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; exists {
		kaiwoNode.Status.IsControlPlane = true
	}
	if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
		kaiwoNode.Status.IsControlPlane = true
	}
	kaiwoNode.Status.Unschedulable = node.Spec.Unschedulable

	return kaiwoNode, nil
}
