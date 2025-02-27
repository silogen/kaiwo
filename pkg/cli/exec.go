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

//
//import (
//	"context"
//	"fmt"
//
//	utils2 "github.com/silogen/kaiwo/pkg/cli/utils"
//
//	list "github.com/silogen/kaiwo/pkg/tui/list/pod"
//
//	"github.com/spf13/cobra"
//
//	"github.com/silogen/kaiwo/pkg/k8s"
//	"github.com/silogen/kaiwo/pkg/workloads/factory"
//)
//
//var (
//	execNamespace   string
//	execInteractive bool
//	execTTY         bool
//)
//
//func BuildMonitorCmd(name string, command string) *cobra.Command {
//	cmd := &cobra.Command{
//		Use:   name + " <workloadType>/<workloadName>",
//		Args:  cobra.ExactArgs(1),
//		Short: fmt.Sprintf("Run %s in a container", command),
//		RunE: func(cmd *cobra.Command, args []string) error {
//			parsedCommand := utils2.ParseCommand(command)
//			if err := utils2.ValidateCommand(parsedCommand[0]); err != nil {
//				return err
//			}
//			return executeContainerCommand(args, parsedCommand, true)
//		},
//	}
//
//	cmd.Flags().StringVarP(&execNamespace, "namespace", "n", "kaiwo", "Namespace of the workload")
//
//	return cmd
//}
//
//func BuildExecCommand() *cobra.Command {
//	var execCommand string
//
//	cmd := &cobra.Command{
//		Use:   "exec <workloadType>/<workloadName>",
//		Args:  cobra.ExactArgs(1),
//		Short: "Execute a command inside a container interactively",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			if execCommand == "" {
//				return fmt.Errorf("--command flag is required")
//			}
//
//			command := utils2.ParseCommand(execCommand)
//			if err := utils2.ValidateCommand(command[0]); err != nil {
//				return err
//			}
//			return executeContainerCommand(args, command, false)
//		},
//	}
//
//	cmd.Flags().StringVarP(&execCommand, "command", "", "", "Command to execute in the container")
//	cmd.Flags().StringVarP(&execNamespace, "namespace", "n", "kaiwo", "Namespace of the workload")
//	cmd.Flags().BoolVarP(&execInteractive, "interactive", "i", true, "Enable interactive mode")
//	cmd.Flags().BoolVarP(&execTTY, "tty", "t", true, "Enable TTY")
//
//	return cmd
//}
//
//func executeContainerCommand(args []string, command []string, gpuPodsOnly bool) error {
//	// Use Bubble Tea to select the pod/container
//	workload, objectKey, err := factory.GetWorkloadAndObjectKey(args[0], execNamespace)
//	if err != nil {
//		return fmt.Errorf("failed to get workload and object key: %w", err)
//	}
//
//	if workload == nil || objectKey.Name == "" {
//		return fmt.Errorf("workload or object key is nil")
//	}
//
//	ctx := context.TODO()
//
//	clients, err := k8s.GetKubernetesClients()
//	if err != nil {
//		return fmt.Errorf("failed to get k8s clients: %w", err)
//	}
//
//	if err := workload.LoadFromObjectKey(ctx, clients.Client, objectKey); err != nil {
//		return fmt.Errorf("failed to build workload reference: %w", err)
//	}
//
//	allPods := workload.ListKnownPods()
//	if len(allPods) == 0 {
//		return fmt.Errorf("no pods found for workload %s", args[0])
//	}
//
//	var predicates []utils2.PodSelectionPredicate
//
//	if gpuPodsOnly {
//		predicates = []utils2.PodSelectionPredicate{utils2.IsGPUPod}
//	}
//
//	podName, containerName, err, cancelled := list.ChoosePodAndContainer(ctx, *clients, workload, predicates...)
//	if err != nil {
//		return fmt.Errorf("failed to choose pod and container: %w", err)
//	}
//	if cancelled {
//		return nil
//	}
//
//	return utils2.ExecInContainer(ctx, clients.Clientset, clients.Kubeconfig, podName, containerName, execNamespace, command, execInteractive, execTTY)
//}
