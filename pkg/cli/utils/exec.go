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

package cliutils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var DefaultMonitorCommand = "watch -n 1 rocm-smi"

func ParseCommand(command string) []string {
	return []string{"/bin/sh", "-c", command}
}

func ValidateCommand(command string) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("error: %s not found in the container", command)
	}
	return nil
}

func ExecInContainer(
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
	logrus.Debugf("Executing command: %v in container %s of pod %s", command, containerName, podName)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		cancel()
	}()

	// Get terminal size for full-width rendering
	termWidth, termHeight := getTerminalSize()

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

	executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               tty,
		TerminalSizeQueue: &fixedSizeQueue{Width: termWidth, Height: termHeight},
	})

	if errors.Is(ctx.Err(), context.Canceled) {
		logrus.Debug("Execution canceled by User")
		return nil
	}

	if err != nil {
		return fmt.Errorf("stream execution error: %w", err)
	}

	logrus.Debug("Executor stream finished successfully")
	return nil
}

// fixedSizeQueue implements the TerminalSizeQueue interface
type fixedSizeQueue struct {
	Width  int
	Height int
}

func (q *fixedSizeQueue) Next() *remotecommand.TerminalSize {
	return &remotecommand.TerminalSize{Width: uint16(q.Width), Height: uint16(q.Height)}
}

// getTerminalSize fetches the current terminal size
func getTerminalSize() (int, int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80, 24 // Default to 80x24 if size can't be determined
	}
	return width, height
}
