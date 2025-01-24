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
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

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
			parsedCommand := parseCommand(command)
			if err := validateCommand(parsedCommand[0]); err != nil {
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

			command := parseCommand(execCommand)
			if err := validateCommand(command[0]); err != nil {
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

func parseCommand(command string) []string {
	return []string{"/bin/sh", "-c", command}
}

func validateCommand(command string) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("Error: %s not found in the container", command)
	}
	return nil
}

func executeContainerCommand(args []string, command []string, gpuPodsOnly bool) error {
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

	allPods := reference.GetPodsRecursive()
	if len(allPods) == 0 {
		return fmt.Errorf("no pods found for workload %s", args[0])
	}

	podName, containerName, err, cancelled := utils.ChoosePodAndContainer(*reference, gpuPodsOnly)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container: %w", err)
	}
	if cancelled {
		return nil
	}

	return execInContainer(ctx, clientset, config, podName, containerName, execNamespace, command, execInteractive, execTTY)
}

func execInContainer(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	config *rest.Config,
	podName string,
	containerName string,
	namespace string,
	command []string,
	interactive bool,
	tty bool,
) error {
	logrus.Infof("Executing command: %v in container %s of pod %s", command, containerName, podName)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		cancel()
	}()

	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     interactive,
			Stdout:    true,
			Stderr:    true,
			TTY:       tty,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    tty,
	})
}
