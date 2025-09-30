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

package workloadlist

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/common"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/silogen/kaiwo/pkg/k8s"
	tuicomponents "github.com/silogen/kaiwo/pkg/tui/components"
)

// runPortForward runs the main target selection and port forwarding routine
// TODO remove container ports if they are defined by a service
func runPortForward(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState) (tuicomponents.StepResult, tuicomponents.RunStep[tuicomponents.RunState], error) {
	workloadReference := state.Workload

	var err error

	var services []v1.Service

	loadServices := func() {
		services, err = workloadutils.GetWorkloadServices(ctx, clients.Client, workloadReference)
	}

	if spinnerErr := spinner.New().Title("Discovering services").Action(loadServices).Run(); spinnerErr != nil {
		return tuicomponents.StepResultErr, nil, spinnerErr
	}

	if err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to get services for workload %v", err)
	}

	var targets []PortForwardTarget
	var containerAnyPortTargets []PortForwardTarget

	for _, service := range services {
		for _, port := range service.Spec.Ports {
			targets = append(targets, ServicePortForwardTarget{service: service, servicePort: port})
		}
	}

	// TODO add spinner
	workloadPods, err := workloadutils.GetWorkloadPods(ctx, clients.Client, workloadReference)
	if err != nil {
		return tuicomponents.StepResultErr, nil, fmt.Errorf("failed to get pods for workload %v", err)
	}
	for _, pod := range workloadPods {
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				targets = append(targets, ContainerPortForwardTarget{
					containerName: container.Name,
					podName:       pod.Name,
					port:          int(port.ContainerPort),
					portName:      port.Name,
					protocol:      string(port.Protocol),
				})
			}
			containerAnyPortTargets = append(containerAnyPortTargets, ContainerAnyPortForwardTarget{podName: pod.Name, containerName: container.Name})
		}
	}

	targets = append(targets, containerAnyPortTargets...)

	var data [][]string

	for _, target := range targets {
		row := target.GetTableRow()
		data = append(data, []string{
			row.Type,
			row.Name,
			row.PortName,
			row.Port,
			row.Protocol,
		})
	}

	columns := []string{
		"Type",
		"Name",
		"Port name",
		"Port",
		"Protocol",
	}

	index, result, err := tuicomponents.RunSelectTable(data, columns, "Select the target", true)
	if err != nil {
		return tuicomponents.StepResultErr, nil, err
	}
	if result == tuicomponents.StepResultOk {
		var cancelled bool
		cancelled, err = doPortForward(ctx, clients, state, targets[index])
		if cancelled {
			return tuicomponents.StepResultPrevious, nil, err
		}
		return result, nil, nil
	}
	return result, nil, nil
}

func promptForLocalPort(info PortForwardInfo) (int, string, bool, error) {
	port := strconv.Itoa(info.Port)
	host := "localhost"

	for {
		confirm := true
		f := huh.NewForm(huh.NewGroup(
			huh.NewNote().Title("Pod: "+info.PodName),
			huh.NewNote().Title("Container: "+info.ContainerName),
			huh.NewInput().Title("Local port").Value(&port),
			huh.NewInput().Title("Host").Value(&host),
			huh.NewConfirm().Title("Confirm").Negative("Cancel").Affirmative("Continue").Value(&confirm),
		))

		err := f.Run()
		if err != nil {
			return -1, "", false, fmt.Errorf("failed to fetch input: %w", err)
		}

		intPort, err := strconv.Atoi(port)
		if err != nil {
			logrus.Warnf("failed to convert port %s to an integer", port)
		} else if intPort < 1 || intPort > 65535 {
			logrus.Warnf("port %s is out of range", port)
		} else if !isValidHost(host) {
			logrus.Warnf("host %s is invalid", host)
		} else if !isPortFree(host, intPort) {
			logrus.Warnf("port %s:%s is in use", host, port)
		} else {
			return intPort, host, confirm, nil
		}
		if !confirm {
			return intPort, host, false, nil
		}
	}
}

func isValidHost(host string) bool {
	// Check if the host is a valid IP address
	if net.ParseIP(host) != nil {
		return true
	}

	// Regular expression to match a valid hostname
	hostnameRegex := `^([a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?\.)*[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	re := regexp.MustCompile(hostnameRegex)

	// Ensure the host is within the valid length (1-255 chars) and matches the regex
	return len(host) <= 255 && re.MatchString(host)
}

func doPortForward(ctx context.Context, clients k8s.KubernetesClients, state *tuicomponents.RunState, target PortForwardTarget) (bool, error) {
	var info *PortForwardInfo
	var err error

	info, err = target.GetInfo(ctx, *clients.Clientset, state.Namespace)
	if err != nil {
		return false, fmt.Errorf("failed to get pod info after retries: %w", err)
	}

	if info == nil {
		return true, nil
	}

	// Prompt user for local port
	localPort, host, confirm, err := promptForLocalPort(*info)
	if err != nil {
		return false, fmt.Errorf("failed to get local port: %w", err)
	}

	if !confirm {
		return true, nil
	}

	// Kubernetes API request setup
	req := clients.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(state.Namespace).
		Name(info.PodName).
		SubResource("portforward")

	url := req.URL()

	transport, upgrader, err := spdy.RoundTripperFor(clients.Kubeconfig)
	if err != nil {
		return false, fmt.Errorf("failed to create SPDY transport: %w", err)
	}

	// Set up port forwarder
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	errChan := make(chan error, 1)

	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	forwarder, err := portforward.NewOnAddresses(
		spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url),
		[]string{host},
		[]string{fmt.Sprintf("%d:%d", localPort, info.Port)},
		stopChan,
		readyChan,
		out,
		errOut,
	)
	if err != nil {
		return false, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			logrus.Errorf("Port forwarding error: %v", err)
			errChan <- err // Send error to main function
		}
	}()

	var connectionErr error

	spinnerErr := spinner.New().
		Title(fmt.Sprintf("Establishing port forward to %s:%d", info.PodName, info.Port)).
		Action(func() {
			select {
			case <-readyChan:
				return
			case err := <-errChan:
				connectionErr = err // Capture the error and exit the spinner
			case <-time.After(10 * time.Second):
				connectionErr = fmt.Errorf("timeout waiting for port forwarding to be ready")
				close(stopChan)
			}
		}).Run()

	if connectionErr != nil {
		return false, connectionErr
	}

	if spinnerErr != nil {
		return false, spinnerErr
	}

	logrus.Infof("Port forwarding established: %s:%d -> %s on port %d",
		host, localPort, info.PodName, info.Port)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalChan:
		logrus.Info("Received termination signal, stopping port forwarding...")
		close(stopChan)
		return false, nil
	case err := <-errChan:
		logrus.Error("Port forwarding failed, stopping...")
		close(stopChan)
		return false, fmt.Errorf("port forwarding error: %w", err)
	}
}

// isPortFree checks if a given port is available for use
func isPortFree(host string, port int) bool {
	address := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return false // Port is in use
	}
	_ = ln.Close() // Close immediately since we only check availability
	return true
}

type PortForwardTableRow struct {
	Type     string
	Name     string
	Port     string
	PortName string
	Protocol string
}
