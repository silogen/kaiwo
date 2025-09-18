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

package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/olekukonko/tablewriter"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

func BuildStatsCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "status",
		Short: "Show Kaiwo cluster status",
	}

	// AMD subcommand
	statsCmd.AddCommand(&cobra.Command{
		Use:   "amd",
		Short: "Show node status - looks for AMD GPUs",
		RunE:  runNodesStatsCmd("amd.com/gpu"),
	})

	// NVIDIA subcommand
	statsCmd.AddCommand(&cobra.Command{
		Use:   "nvidia",
		Short: "Show node status - looks for Nvidia GPUs",
		RunE:  runNodesStatsCmd("nvidia.com/gpu"),
	})

	return statsCmd
}

func extractGPUModel(poolLabel string) string {
	if strings.HasPrefix(poolLabel, common.CPUOnly) {
		return common.CPUOnly
	}

	parts := strings.Split(poolLabel, "-")
	if len(parts) < 2 {
		return "unknown"
	}
	return parts[1]
}

func fetchWithSpinner(ctx context.Context, k8sClient client.Client, list client.ObjectList) error {
	t := reflect.TypeOf(list)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	base := strings.TrimSuffix(t.Name(), "List")
	resourceName := strings.ToLower(base) + "s"

	var err error
	if spinErr := spinner.New().
		Title(fmt.Sprintf("Fetching %s", resourceName)).
		Action(func() { err = k8sClient.List(ctx, list, &client.ListOptions{}) }).
		Run(); spinErr != nil {
		panic(spinErr)
	}
	if err != nil {
		return fmt.Errorf("failed to list %s: %w", resourceName, err)
	}
	return nil
}

func runNodesStatsCmd(gpuResourceName v1.ResourceName) func(cmd *cobra.Command, args []string) error {
	return func(_ *cobra.Command, _ []string) error {
		clients, err := k8s.GetKubernetesClients()
		if err != nil {
			return fmt.Errorf("failed to get k8s clients: %w", err)
		}

		ctx := context.Background()

		nodeList := v1.NodeList{}

		allocatable := map[string]v1.ResourceList{}

		if err := fetchWithSpinner(ctx, clients.Client, &nodeList); err != nil {
			return fmt.Errorf("failed to fetch nodes: %w", err)
		}

		for _, node := range nodeList.Items {
			allocatable[node.Name] = node.Status.Allocatable
		}

		podList := v1.PodList{}
		if err := fetchWithSpinner(ctx, clients.Client, &podList); err != nil {
			return fmt.Errorf("failed to fetch pods: %w", err)
		}

		requested := map[string]v1.ResourceList{}
		podCount := make(map[string]int)

		for _, pod := range podList.Items {
			if pod.Spec.NodeName == "" {
				continue
			}
			podCount[pod.Spec.NodeName]++

			nodeName := pod.Spec.NodeName
			if _, ok := requested[nodeName]; !ok {
				requested[nodeName] = v1.ResourceList{}
			}
			for _, container := range pod.Spec.Containers {
				for resourceType, resourceAmount := range container.Resources.Requests {
					if _, ok := requested[nodeName][resourceType]; !ok {
						requested[nodeName][resourceType] = resource.Quantity{}
					}
					currentQuantity := requested[nodeName][resourceType]
					currentQuantity.Add(resourceAmount)
					requested[nodeName][resourceType] = currentQuantity
				}
			}
		}

		nodesTable := tablewriter.NewWriter(os.Stdout)
		nodesTable.SetHeader([]string{"NODE", "GPU MODEL", "GPU (used/alloc %)", "CPU (used/alloc %)", "MEM (used/alloc %)", "PODS (used/alloc %)"})
		nodesTable.SetAutoWrapText(false)
		nodesTable.SetAlignment(tablewriter.ALIGN_LEFT)
		nodesTable.SetCaption(true, fmt.Sprintf("GPU requests are looked up by the resource name '%s'", gpuResourceName))

		const bytesInGi = 1024 * 1024 * 1024

		for _, node := range nodeList.Items {
			name := node.Name

			// GPU
			allocGPU := allocatable[name][gpuResourceName]
			usedGPU := requested[name][gpuResourceName]
			gpuModel := extractGPUModel(node.Labels["kaiwo/nodepool"])
			gpuPct := float64(usedGPU.Value()) / float64(allocGPU.Value()) * 100
			if math.IsNaN(gpuPct) {
				gpuPct = 0
			}
			gpuCell := fmt.Sprintf("%d/%d (%.1f%%)", int(usedGPU.Value()), allocGPU.Value(), gpuPct)

			// CPU
			allocCPU := allocatable[name][v1.ResourceCPU]
			usedCPU := requested[name][v1.ResourceCPU]
			usedCores := float64(usedCPU.MilliValue() / 1000)
			allocCores := float64(allocCPU.MilliValue() / 1000)
			cpuPct := usedCores / allocCores * 100
			cpuCell := fmt.Sprintf("%.1f/%.1f (%.1f%%)", usedCores, allocCores, cpuPct)

			// MEM
			allocMemB := allocatable[name][v1.ResourceMemory]
			usedMemB := requested[name][v1.ResourceMemory]
			usedGi := float64(usedMemB.Value()) / bytesInGi
			allocGi := float64(allocMemB.Value()) / bytesInGi
			memPct := usedGi / allocGi * 100
			memCell := fmt.Sprintf("%.1fGi/%.1fGi (%.1f%%)", usedGi, allocGi, memPct)

			// PODS
			allocPods := allocatable[name][v1.ResourcePods]
			allocPodsV := allocPods.Value()
			usedPods := int64(podCount[name])
			podsPct := float64(usedPods) / float64(allocPodsV) * 100
			podCell := fmt.Sprintf("%d/%d (%.1f%%)", usedPods, allocPodsV, podsPct)

			nodesTable.Append([]string{name, gpuModel, gpuCell, cpuCell, memCell, podCell})
		}

		nodesTable.Render()

		return nil
	}
}
