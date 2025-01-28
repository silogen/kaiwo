// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/silogen/kaiwo/pkg/workloads/factory"
	"github.com/silogen/kaiwo/pkg/workloads/utils"
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
			parsedCommand := utils.ParseCommand(command)
			if err := utils.ValidateCommand(parsedCommand[0]); err != nil {
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

			command := utils.ParseCommand(execCommand)
			if err := utils.ValidateCommand(command[0]); err != nil {
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
	// Use Bubble Tea to select the pod/container
	workload, objectKey, err := factory.GetWorkloadAndObjectKey(args[0], execNamespace)
	if err != nil {
		return fmt.Errorf("failed to get workload and object key: %w", err)
	}

	if workload == nil || objectKey.Name == "" {
		return fmt.Errorf("workload or object key is nil")
	}

	ctx := context.TODO()

	k8sClient, err := k8s.GetClient()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	kubeconfig, _ := k8s.GetKubeConfig()
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	reference, err := workload.BuildReference(ctx, k8sClient, objectKey)
	if err != nil {
		return fmt.Errorf("failed to build workload reference: %w", err)
	}

	allPods := reference.GetPods()
	if len(allPods) == 0 {
		return fmt.Errorf("no pods found for workload %s", args[0])
	}

	var predicates []utils.PodSelectionPredicate

	if gpuPodsOnly {
		predicates = []utils.PodSelectionPredicate{utils.IsGPUPod}
	}

	podName, containerName, err, cancelled := utils.ChoosePodAndContainer(ctx, k8sClient, reference, predicates...)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container: %w", err)
	}
	if cancelled {
		return nil
	}

	return utils.ExecInContainer(ctx, clientset, config, podName, containerName, execNamespace, command, execInteractive, execTTY)
}
