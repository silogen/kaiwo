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

package controllerutils

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeResourceInfo struct {
	Name          string
	CPU           int
	Memory        int
	Labels        map[string]string
	Unschedulable bool
}

func CleanAMDGPUName(gpuID string) string {
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

func GetNodeResources(ctx context.Context, c client.Client) []NodeResourceInfo {
	var nodeList corev1.NodeList
	err := c.List(ctx, &nodeList)
	if err != nil {
		return []NodeResourceInfo{}
	}

	var nodes []NodeResourceInfo
	for _, node := range nodeList.Items {
		cpu := node.Status.Capacity.Cpu().Value()
		memory := node.Status.Capacity.Memory().Value() / (1024 * 1024 * 1024) // Convert to Gi
		Unschedulable := node.Spec.Unschedulable

		nodes = append(nodes, NodeResourceInfo{
			Name:          node.Name,
			CPU:           int(cpu),
			Memory:        int(memory),
			Labels:        node.Labels,
			Unschedulable: Unschedulable,
		})
	}

	return nodes
}

func LabelNode(ctx context.Context, c client.Client, nodeName, key, value string) error {
	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var node corev1.Node
		if err := c.Get(cctx, client.ObjectKey{Name: nodeName}, &node); err != nil {
			return err
		}

		if node.Labels != nil && node.Labels[key] == value {
			return nil
		}

		base := node.DeepCopy()
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.Labels[key] = value

		return c.Patch(cctx, &node, client.MergeFrom(base))
	})
}

func TaintNode(ctx context.Context, client client.Client, nodeName string, taint corev1.Taint) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var node corev1.Node
		if err := client.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
			return err
		}

		for _, t := range node.Spec.Taints {
			if t.MatchTaint(&taint) {
				return nil
			}
		}

		node.Spec.Taints = append(node.Spec.Taints, taint)
		return client.Update(ctx, &node)
	})
}
