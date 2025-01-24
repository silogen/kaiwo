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
	"strings"
	"syscall"

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

	logrus.Infof("%s", reference.String())
	for _, child := range reference.Children {
		logrus.Infof("%s", child.String())
	}

	allPods := reference.GetPodsRecursive()

	if len(allPods) == 0 {
		logrus.Warn("No pods found for workload")
		return nil
	}

	if noAutoSelect {
		// Skip trying to select
	} else if len(allPods) == 1 {
		// If there is only a single pod with a single container, just list its logs
		logrus.Info("Found a single pod for workload")
		pod := allPods[0]

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

	podName, containerName, err, cancelled := ChoosePodAndContainer(*reference, false)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container for workload: %w", err)
	}

	if cancelled {
		return nil
	}

	return outputLogs(ctx, clientset, podName, containerName, tailLines, objectKey.Namespace, follow)
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

type PodContainerOption struct {
	// pod           corev1.Pod
	// containerName string
}

type OptionRow struct {
	status     string
	indent     int
	selectable bool
	// selected      bool
	type_         string
	name          string
	containerName string
	podName       string
	reference     workloads.WorkloadReference
}

func (or OptionRow) GetCells() []any {
	return []any{
		strings.Repeat(" ", or.indent) + or.type_,
		or.name,
	}
}

func (or OptionRow) IsSelectable() bool {
	return or.selectable
}

func (or OptionRow) GetData() *OptionRow {
	return &or
}

func traverse(node workloads.WorkloadReference, currentIdent int, onlyGPUPods bool) []OptionRow {
	var rows []OptionRow
	rows = append(rows, OptionRow{
		status:        "N/A",
		indent:        currentIdent,
		selectable:    false,
		type_:         node.GVK.String(),
		name:          node.Object.GetName(),
		containerName: "",
		podName:       "",
		reference:     node,
	})

	if node.IsLeaf {
		for _, pod := range node.Pods {
			if onlyGPUPods && !isGPUPod(pod) {
				continue
			}

			rows = append(rows, OptionRow{
				status:        "N/A",
				indent:        currentIdent + 1,
				selectable:    false,
				type_:         "Pod",
				name:          pod.Name,
				containerName: "",
				podName:       "",
			})

			for _, container := range pod.Status.InitContainerStatuses {
				rows = append(rows, OptionRow{
					status:        "N/A",
					indent:        currentIdent + 2,
					selectable:    true,
					type_:         "Init container",
					name:          container.Name,
					containerName: container.Name,
					podName:       pod.Name,
				})
			}

			for _, container := range pod.Status.ContainerStatuses {
				rows = append(rows, OptionRow{
					status:        "N/A",
					indent:        currentIdent + 2,
					selectable:    true,
					type_:         "Container",
					name:          container.Name,
					containerName: container.Name,
					podName:       pod.Name,
				})
			}
		}
	}

	for _, child := range node.Children {
		rows = append(rows, traverse(*child, currentIdent+1, onlyGPUPods)...)
	}
	return rows
}

func ChoosePodAndContainer(reference workloads.WorkloadReference, onlyGPUPods bool) (string, string, error, bool) {
	flatList := traverse(reference, 0, onlyGPUPods)
	entries := make([]tui.SelectTableEntry[OptionRow], len(flatList))

	for i, row := range flatList {
		entries[i] = row // OptionRow implements SelectTableEntry[OptionRow]
	}

	columns := []string{
		"Type",
		"Name",
	}
	selected, err := tui.RunSelectTable(entries, columns, "Select the container to view", true)

	if err != nil {
		return "", "", err, false
	}

	if selected == nil {
		return "", "", nil, true
	}

	return selected.podName, selected.containerName, nil, false
}

func isGPUPod(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			if resourceName == "nvidia.com/gpu" || resourceName == "amd.com/gpu" {
				return true
			}
		}
	}
	return false
}
