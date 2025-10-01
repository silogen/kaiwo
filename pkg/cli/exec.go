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

	list "github.com/silogen/kaiwo/pkg/tui/list/pod"

	"github.com/spf13/cobra"

	"github.com/silogen/kaiwo/pkg/k8s"
)

var (
	execNamespace   string
	execInteractive bool
	execTTY         bool
)

func BuildMonitorCmd(name string, command string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name + " <workloadType>/<workloadName>",
		Args:  cobra.ExactArgs(1),
		Short: fmt.Sprintf("Run %s in a container", command),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedCommand := utils2.ParseCommand(command)
			if err := utils2.ValidateCommand(parsedCommand[0]); err != nil {
				return err
			}
			return executeContainerCommand(args, parsedCommand, true)
		},
	}

	cmd.Flags().StringVarP(&execNamespace, "namespace", "n", "kaiwo", "Namespace of the workload")

	return cmd
}

func BuildExecCommand() *cobra.Command {
	var execCommand string

	cmd := &cobra.Command{
		Use:   "exec <workloadType>/<workloadName>",
		Args:  cobra.ExactArgs(1),
		Short: "Execute a command inside a container interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			if execCommand == "" {
				return fmt.Errorf("--command flag is required")
			}

			command := utils2.ParseCommand(execCommand)
			if err := utils2.ValidateCommand(command[0]); err != nil {
				return err
			}
			return executeContainerCommand(args, command, false)
		},
	}

	cmd.Flags().StringVarP(&execCommand, "command", "", "", "Command to execute in the container")
	cmd.Flags().StringVarP(&execNamespace, "namespace", "n", "kaiwo", "Namespace of the workload")
	cmd.Flags().BoolVarP(&execInteractive, "interactive", "i", true, "Enable interactive mode")
	cmd.Flags().BoolVarP(&execTTY, "tty", "t", true, "Enable TTY")

	return cmd
}

func executeContainerCommand(args []string, command []string, gpuPodsOnly bool) error {
	ctx := context.Background()

	clients, err := k8s.GetKubernetesClients()
	if err != nil {
		return fmt.Errorf("failed to get k8s clients: %w", err)
	}

	// Use Bubble Tea to select the pod/container
	workload, err := utils2.GetWorkload(ctx, clients.Client, args[0], execNamespace)
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

	var predicates []utils2.PodSelectionPredicate

	if gpuPodsOnly {
		predicates = []utils2.PodSelectionPredicate{utils2.IsGPUPod}
	}

	podName, containerName, err, cancelled := list.ChoosePodAndContainer(ctx, *clients, workload, predicates...)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container: %w", err)
	}
	if cancelled {
		return nil
	}

	return utils2.ExecInContainer(ctx, clients.Clientset, clients.Kubeconfig, podName, containerName, execNamespace, command, execInteractive, execTTY)
}
