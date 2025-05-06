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

	"k8s.io/apimachinery/pkg/api/meta"

	"sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/olekukonko/tablewriter"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"
)

func BuildStatsCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use: "stats",
	}

	nodesStatsCmd := &cobra.Command{
		Use:  "nodes",
		RunE: runNodesStatsCmd,
	}
	statsCmd.AddCommand(nodesStatsCmd)

	queueStatsCmd := &cobra.Command{
		Use:  "queues",
		RunE: runQueueStatsCmd,
	}
	statsCmd.AddCommand(queueStatsCmd)

	return statsCmd
}

var gpuResourceName = v1.ResourceName("amd.com/gpu")

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

func runNodesStatsCmd(_ *cobra.Command, _ []string) error {
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
	nodesTable.SetHeader([]string{"NODE", "GPU (used/alloc %)", "CPU (used/alloc %)", "MEM (used/alloc %)", "PODS (used/alloc %)"})
	nodesTable.SetAutoWrapText(false)
	nodesTable.SetAlignment(tablewriter.ALIGN_LEFT)
	nodesTable.SetCaption(true, fmt.Sprintf("GPU requests are looked up by the resource name '%s'", gpuResourceName))

	const bytesInGi = 1024 * 1024 * 1024

	for _, node := range nodeList.Items {
		name := node.Name

		// GPU
		allocGPU := allocatable[name][gpuResourceName]
		usedGPU := requested[name][gpuResourceName]
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

		nodesTable.Append([]string{name, gpuCell, cpuCell, memCell, podCell})
	}

	nodesTable.Render()

	return nil
}

func runQueueStatsCmd(cmd *cobra.Command, args []string) error {
	clients, err := k8s.GetKubernetesClients()
	if err != nil {
		return fmt.Errorf("failed to get k8s clients: %w", err)
	}

	ctx := context.Background()

	clusterQueueList := v1beta1.ClusterQueueList{}
	if err := fetchWithSpinner(ctx, clients.Client, &clusterQueueList); err != nil {
		return fmt.Errorf("failed to fetch cluster queues: %w", err)
	}

	localQueueList := v1beta1.LocalQueueList{}
	if err := fetchWithSpinner(ctx, clients.Client, &localQueueList); err != nil {
		return fmt.Errorf("failed to fetch local queues: %w", err)
	}

	workloadList := v1beta1.WorkloadList{}
	if err := fetchWithSpinner(ctx, clients.Client, &workloadList); err != nil {
		return fmt.Errorf("failed to fetch workloads: %w", err)
	}

	// 3) Prepare data structures
	// map[clusterQueueName] -> []LocalQueue
	localQueuesByCluster := make(map[string][]v1beta1.LocalQueue, len(clusterQueueList.Items))

	// GPU counts at cluster level
	clusterAdmittedGPU := make(map[string]int, len(clusterQueueList.Items))
	clusterPendingGPU := make(map[string]int, len(clusterQueueList.Items))

	// GPU counts at local-queue level: map[clusterQueueName] -> map[localQueueName] -> count
	localAdmittedGPU := make(map[string]map[string]int, len(clusterQueueList.Items))
	localPendingGPU := make(map[string]map[string]int, len(clusterQueueList.Items))

	// Reverse lookup: map[namespace] -> map[localQueueName] -> clusterQueueName
	localQueueToCluster := make(map[string]map[string]string, len(localQueueList.Items))

	// 3a) Initialize cluster-level entries
	for _, clusterQueue := range clusterQueueList.Items {
		localQueuesByCluster[clusterQueue.Name] = nil
		clusterAdmittedGPU[clusterQueue.Name] = 0
		clusterPendingGPU[clusterQueue.Name] = 0
		localAdmittedGPU[clusterQueue.Name] = make(map[string]int)
		localPendingGPU[clusterQueue.Name] = make(map[string]int)
	}

	// 3b) Initialize local-level entries and build reverse lookup
	for _, localQueue := range localQueueList.Items {
		// Ensure the namespace map exists
		if _, ok := localQueueToCluster[localQueue.Namespace]; !ok {
			localQueueToCluster[localQueue.Namespace] = make(map[string]string)
		}
		clusterName := string(localQueue.Spec.ClusterQueue)
		// Map this LocalQueue to its ClusterQueue
		localQueueToCluster[localQueue.Namespace][localQueue.Name] = clusterName

		// Append the LocalQueue under its ClusterQueue
		localQueuesByCluster[clusterName] = append(localQueuesByCluster[clusterName], localQueue)

		// Zero out our GPU counters at the local level
		localAdmittedGPU[clusterName][localQueue.Name] = 0
		localPendingGPU[clusterName][localQueue.Name] = 0
	}

	// 4) Filter for GPU-requesting workloads
	var gpuWorkloads []v1beta1.Workload
	for _, workload := range workloadList.Items {
		if condition := meta.FindStatusCondition(workload.Status.Conditions, v1beta1.WorkloadFinished); condition != nil {
			continue
		}
		requestsGpu := false

		for _, podSet := range workload.Spec.PodSets {
			// check each container in the PodTemplate
			for _, container := range podSet.Template.Spec.Containers {
				if qty, ok := container.Resources.Requests[gpuResourceName]; ok && qty.Value() > 0 {
					requestsGpu = true
					break
				}
			}
			if requestsGpu {
				break
			}
			for _, initContainer := range podSet.Template.Spec.InitContainers {
				if qty, ok := initContainer.Resources.Requests[gpuResourceName]; ok && qty.Value() > 0 {
					requestsGpu = true
					break
				}
			}
			if requestsGpu {
				break
			}
		}

		if requestsGpu {
			gpuWorkloads = append(gpuWorkloads, workload)
		}
	}

	// 5) Count admitted vs pending for each GPU-requesting workload
	for _, workload := range gpuWorkloads {
		namespace := workload.Namespace
		localQueueName := workload.Spec.QueueName

		// Find the ClusterQueue for this LocalQueue
		clusterQueueName := "<unknown>"
		if m, ok := localQueueToCluster[namespace]; ok {
			if cqName, ok2 := m[localQueueName]; ok2 {
				clusterQueueName = cqName
			}
		}

		if workload.Status.Admission != nil {
			clusterAdmittedGPU[clusterQueueName]++
			localAdmittedGPU[clusterQueueName][localQueueName]++
		} else {
			clusterPendingGPU[clusterQueueName]++
			localPendingGPU[clusterQueueName][localQueueName]++
		}
	}

	// 6) Render results in a table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Cluster queue", "Local queue", "Admitted", "Pending"})
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCaption(true, fmt.Sprintf("Only workloads that include requests to '%s' are included", gpuResourceName))

	for _, clusterQueue := range clusterQueueList.Items {
		// Cluster-level totals
		table.Append([]string{
			clusterQueue.Name,
			"[TOTAL]",
			fmt.Sprintf("%d", clusterAdmittedGPU[clusterQueue.Name]),
			fmt.Sprintf("%d", clusterPendingGPU[clusterQueue.Name]),
		})

		// Local-level breakdown
		for _, localQueue := range localQueuesByCluster[clusterQueue.Name] {
			table.Append([]string{
				clusterQueue.Name,
				localQueue.Name,
				fmt.Sprintf("%d", localAdmittedGPU[clusterQueue.Name][localQueue.Name]),
				fmt.Sprintf("%d", localPendingGPU[clusterQueue.Name][localQueue.Name]),
			})
		}
	}

	table.Render()
	return nil
}
