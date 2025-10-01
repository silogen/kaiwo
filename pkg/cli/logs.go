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

package cmd

import (
	"context"
	"fmt"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/common"

	utils2 "github.com/silogen/kaiwo/pkg/cli/utils"

	"github.com/spf13/cobra"

	"github.com/silogen/kaiwo/pkg/k8s"
	list "github.com/silogen/kaiwo/pkg/tui/list/pod"
)

var (
	// defaultContainer bool
	follow bool
	// since            string
	// sinceTime        string
	tailLines int
	// timestamps bool
	// previous         bool
	// stream           string
	namespaceLogs string
)

func BuildLogCmd() *cobra.Command {
	// If no accessor is given, the user can choose
	cmd := &cobra.Command{
		Use:   "logs <workloadType>/<workloadName>",
		Args:  cobra.ExactArgs(1),
		Short: "Display logs for a workload. You will be prompted to choose the target container if there is more than one",
		RunE:  executeLogsCommand,
	}
	// cmd.Flags().BoolVarP(&defaultContainer, "default", "d", defaultContainer, "Select the default container within the workload")
	// cmd.Flags().StringVarP(&since, "since", "", "0s", "Only return logs newer than a relative duration")
	// cmd.Flags().StringVarP(&sinceTime, "since-time", "", "", "Only return logs after a specific date (RFC3339)")
	cmd.Flags().IntVarP(&tailLines, "tail", "", -1, "Number of lines to show from the end of the log")
	// cmd.Flags().BoolVarP(&timestamps, "timestamps", "", false, "Show timestamps")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	// cmd.Flags().BoolVarP(&previous, "previous", "p", false, "If true, print the logs for the previous instance of the container in a pod if it exists.")
	// cmd.Flags().StringVarP(&stream, "stream", "", string(StreamTypeAll), "Specify which container log stream to return to the client. One of All, Stdout or Stderr.")
	cmd.Flags().StringVarP(&namespaceLogs, "namespace", "n", "kaiwo", "Namespace of the workload")
	return cmd
}

func executeLogsCommand(_ *cobra.Command, args []string) error {
	ctx := context.Background()

	clients, err := k8s.GetKubernetesClients()
	if err != nil {
		return fmt.Errorf("failed to get k8s clients: %w", err)
	}

	// Use Bubble Tea to select the pod/container
	workload, err := utils2.GetWorkload(ctx, clients.Client, args[0], namespaceLogs)
	if err != nil {
		return fmt.Errorf("failed to get workload and object key: %w", err)
	}

	allPods, err := workloadutils.GetWorkloadPods(ctx, clients.Client, workload)
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}
	if len(allPods) == 0 {
		return fmt.Errorf("no pods found for workload %s", args[0])
	}

	podName, containerName, err, cancelled := list.ChoosePodAndContainer(ctx, *clients, workload)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container: %w", err)
	}
	if cancelled {
		return nil
	}

	return utils2.OutputLogs(
		ctx,
		clients.Clientset,
		podName,
		containerName,
		int64(tailLines),
		namespaceLogs,
		follow,
	)
}
