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

package controllerutils

import (
	"context"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cpuMemoryDiscountFactor = 0.9
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

func getGPUCount(flavorName string) int {
	if strings.Contains(flavorName, "nvidia") || strings.Contains(flavorName, "amd") {
		parts := strings.Split(flavorName, "-")
		for _, p := range parts {
			if strings.HasSuffix(p, "gpu") {
				count, _ := strconv.Atoi(strings.TrimSuffix(p, "gpu"))
				return count
			}
		}
	}
	return 0
}

func getCPUCount(flavorName string) int {
	parts := strings.Split(flavorName, "-")
	for _, p := range parts {
		if strings.HasSuffix(p, "core") {
			count, _ := strconv.Atoi(strings.TrimSuffix(p, "core"))
			return count
		}
	}
	return 0
}

func getMemoryCount(flavorName string) int {
	parts := strings.Split(flavorName, "-")
	for _, p := range parts {
		if strings.HasSuffix(p, "Gi") {
			count, _ := strconv.Atoi(strings.TrimSuffix(p, "Gi"))
			return count
		}
	}
	return 0
}
