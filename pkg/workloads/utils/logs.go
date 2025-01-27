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

package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/tui"
	"github.com/silogen/kaiwo/pkg/workloads"
)

func OutputLogs(
	workload workloads.Workload,
	ctx context.Context,
	k8sClient client.Client,
	clientset *kubernetes.Clientset,
	objectKey client.ObjectKey,
	tailLines int64,
	noAutoSelect bool,
	follow bool,
) error {
	reference, err := workload.BuildReference(ctx, k8sClient, objectKey)

	if err != nil {
		return fmt.Errorf("failed to get workload reference: %w", err)
	}

	if err := reference.Load(ctx, k8sClient); err != nil {
		return fmt.Errorf("failed to load workload reference: %w", err)
	}

	allPods := reference.GetPods()

	if len(allPods) == 0 {
		logrus.Warn("No pods found for workload")
		return nil
	}

	if noAutoSelect {
		// Skip trying to select
	} else if len(allPods) == 1 {
		// If there is only a single pod with a single container, just list its logs
		logrus.Info("Found a single pod for workload")
		pod := allPods[0].Pod

		if len(pod.Status.ContainerStatuses) == 0 {
			logrus.Warn("No containers found for workload")
			return nil
		}

		if len(pod.Status.ContainerStatuses) == 1 {
			logrus.Infof("Found a single container for pod %s, defaulting to this one", pod.Name)
			if len(pod.Status.InitContainerStatuses) > 0 {
				logrus.Warn("Pod init containers found for workload, not displaying logs for these. Disable auto select to choose init containers")
			}
			return outputLogs(ctx, clientset, pod.Name, pod.Status.ContainerStatuses[0].Name, tailLines, objectKey.Namespace, follow)
		} else {
			logrus.Infof("Found multiple containers for pod %s", pod.Name)
		}
	} else {
		logrus.Infof("Found multiple pods for workload")
	}

	podName, containerName, err, cancelled := choosePodAndContainer(reference)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container for workload: %w", err)
	}

	if cancelled {
		return nil
	}

	return outputLogs(ctx, clientset, podName, containerName, tailLines, objectKey.Namespace, follow)
}

var (
	containerSelectColumns = []string{
		"Logical group",
		"Pod name",
		"Pod phase",
		"Container name",
		"Container status",
	}
)

// choosePodAndContainer allows the user to choose the pod and the container they want to interact with
// As the workload reference structure is dynamic and not structured, the output is rendered one step at a time
// The user can still navigate back up the reference tree and choose a different branch
func choosePodAndContainer(reference workloads.WorkloadReference) (string, string, error, bool) {

	allPods := reference.GetPods()

	var data [][]string

	containerStatusToRow := func(pod workloads.WorkloadPod, containerStatus corev1.ContainerStatus, isInitContainer bool) []string {
		containerStatusMsg := ""

		if containerStatus.State.Running != nil {
			containerStatusMsg = fmt.Sprintf("Running since %s", containerStatus.State.Running.StartedAt.Format(time.RFC3339))
		} else if containerStatus.State.Waiting != nil {
			containerStatusMsg = fmt.Sprintf("Waiting (%s)", containerStatus.State.Waiting.Reason)
		} else if containerStatus.State.Terminated != nil {
			containerStatusMsg = fmt.Sprintf("Terminated (%s)", containerStatus.State.Terminated.Reason)
		} else {
			containerStatusMsg = "N/A"
		}

		// TODO add
		//prefix := ""
		//if isInitContainer {
		//	prefix = "[init] "
		//}

		return []string{
			pod.LogicalGroup,
			pod.Pod.Name,
			string(pod.Pod.Status.Phase),
			containerStatus.Name,
			containerStatusMsg,
		}
	}

	for _, pod := range allPods {
		for _, container := range pod.Pod.Status.ContainerStatuses {
			data = append(data, containerStatusToRow(pod, container, false))
		}
		for _, container := range pod.Pod.Status.InitContainerStatuses {
			data = append(data, containerStatusToRow(pod, container, true))
		}
		logrus.Infof("Found pod %s (%s)", pod.Pod.Name, pod.LogicalGroup)
	}

	title := ""
	selectedRow, err := tui.RunSelectTable(data, containerSelectColumns, title, true)

	selectedContainerName := ""
	selectedPodName := ""

	if selectedRow != nil {
		selectedPodName = (*selectedRow)[1]
		selectedContainerName = (*selectedRow)[3]
	}

	return selectedPodName, selectedContainerName, err, selectedRow == nil && err != nil
}

// outputLogs outputs logs for a given pod container to the standard output
func outputLogs(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	podName string,
	containerName string,
	tailLines int64,
	namespace string,
	follow bool,
) error {
	logrus.Infof("Outputting logs for pod: %s (container: %s)", podName, containerName)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle Ctrl+C or SIGTERM
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopCh
		logrus.Debugln("Received termination signal, closing log stream...")
		cancel() // Cancel the context to terminate log streaming
	}()

	// Set up Pod log options
	podLogOpts := &corev1.PodLogOptions{
		Container: containerName,
		Follow:    follow,
	}
	if tailLines > 0 {
		podLogOpts.TailLines = &tailLines
	}

	// Get the log stream
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	logStream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("error streaming logs for pod %s container %s: %w", podName, containerName, err)
	}

	// Channel to signal when log streaming is done
	done := make(chan struct{})

	defer func() {
		<-done
		if closeErr := logStream.Close(); closeErr != nil {
			logrus.Errorf("Error closing log stream for pod %s container %s: %v\n", podName, containerName, closeErr)
		}
	}()

	// Goroutine to read and print logs
	go func() {
		defer close(done)
		buf := make([]byte, 2000)
		for {
			select {
			case <-ctx.Done():
				logrus.Debugf("Context canceled, stopping log stream...")
				return
			default:
				n, err := logStream.Read(buf)
				if n > 0 {
					fmt.Print(string(buf[:n]))
				}
				if err != nil {
					if err == io.EOF {
						return
					}
					// Check if the error is due to context cancellation
					if errors.Is(err, context.Canceled) || err.Error() == "context canceled" {
						logrus.Tracef("Log stream terminated by context cancellation.")
						break
					}
					logrus.Errorf("Error reading log stream: %v\n", err)
					break
				}
			}
		}
	}()

	// Wait for the log streaming to complete or a termination signal
	select {
	case <-done:
		logrus.Debugf("Log streaming completed.")
	case <-ctx.Done():
		logrus.Debugf("Log streaming canceled.")
	}

	return nil
}
