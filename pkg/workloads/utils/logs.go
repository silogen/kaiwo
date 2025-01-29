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

package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// OutputLogs outputs logs for a given pod container to the standard output
func OutputLogs(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	podName string,
	containerName string,
	tailLines int64,
	namespace string,
	follow bool,
) error {
	logrus.Debugf("Outputting logs for pod: %s (container: %s)", podName, containerName)
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
						return
					}
					logrus.Errorf("Error reading log stream: %v\n", err)
					return
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
