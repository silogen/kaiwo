/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package utils

import (
	"context"
	"errors"
	"fmt"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/sirupsen/logrus"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"os"
	"os/signal"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"syscall"
)

//type StreamType string
//
//var (
//	StreamTypeAll    = StreamType("all")
//	StreamTypeStdout = StreamType("stdout")
//	StreamTypeStderr = StreamType("stderr")
//)

func OutputLogs(
	workload workloads.Workload,
	ctx context.Context,
	k8sClient client.Client,
	clientset *kubernetes.Clientset,
	objectKey client.ObjectKey,
	tailLines int64,
	noAutoSelect bool,
	//stream StreamType,
	follow bool,
) error {
	reference, err := workload.BuildReference(ctx, k8sClient, objectKey)

	if err != nil {
		return fmt.Errorf("failed to get workload reference: %w", err)
	}

	logrus.Infof(reference.String())
	for _, child := range reference.Children {
		logrus.Infof(child.String())
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
			return outputLogs(ctx, clientset, pod, pod.Status.ContainerStatuses[0].Name, tailLines, follow)
		} else {
			logrus.Infof("Found multiple containers for pod %s", pod.Name)
		}
	} else {
		logrus.Infof("Found multiple pods for workload")
	}

	pod, containerName, err := choosePodAndContainer(*reference)
	if err != nil {
		return fmt.Errorf("failed to choose pod and container for workload: %w", err)
	}

	return outputLogs(ctx, clientset, pod, containerName, tailLines, follow)
}

// outputLogs outputs logs for a given pod container to the standard output
func outputLogs(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	pod corev1.Pod,
	containerName string,
	tailLines int64,
	follow bool,
) error {
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
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, podLogOpts)
	logStream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("error streaming logs for pod %s container %s: %w", pod.Name, containerName, err)
	}

	// Channel to signal when log streaming is done
	done := make(chan struct{})

	defer func() {
		<-done
		if closeErr := logStream.Close(); closeErr != nil {
			logrus.Errorf("Error closing log stream for pod %s container %s: %v\n", pod.Name, containerName, closeErr)
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
						break // End of the log stream
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

// choosePodAndContainer allows the user to choose the pod and the container they want to interact with
func choosePodAndContainer(reference workloads.WorkloadReference) (corev1.Pod, string, error) {
	return corev1.Pod{}, "", nil
}
