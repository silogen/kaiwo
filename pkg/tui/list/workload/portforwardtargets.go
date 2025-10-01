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
	"context"
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/charmbracelet/huh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PortForwardInfo struct {
	PodName       string
	ContainerName string
	Port          int
}

type PortForwardTarget interface {
	// GetInfo provides port forwarding info for this particular type. If the info and error are both nil, this is take to mean
	// that the action has been cancelled
	GetInfo(ctx context.Context, clientset kubernetes.Clientset, namespace string) (*PortForwardInfo, error)

	// GetTableRow provides a table row that describes this target
	GetTableRow() PortForwardTableRow
}

type ServicePortForwardTarget struct {
	service     v1.Service
	servicePort v1.ServicePort
}

func (s ServicePortForwardTarget) GetInfo(ctx context.Context, clientset kubernetes.Clientset, namespace string) (*PortForwardInfo, error) {
	labelSelector := metav1.FormatLabelSelector(metav1.SetAsLabelSelector(s.service.Spec.Selector))

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for service %v", err)
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for service %s", s.service.Name)
	}

	// TODO choose first healthy pod?
	pod := pods.Items[0]

	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if int(port.ContainerPort) == s.servicePort.TargetPort.IntValue() {
				return &PortForwardInfo{
					PodName:       pod.Name,
					ContainerName: container.Name,
					Port:          int(port.ContainerPort),
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no pods with matching container ports found for service %s and port %s", s.service.Name, s.servicePort.Name)
}

func (s ServicePortForwardTarget) GetTableRow() PortForwardTableRow {
	return PortForwardTableRow{
		Type:     "Service",
		Name:     s.service.Name,
		PortName: s.servicePort.Name,
		Port:     s.servicePort.TargetPort.String(),
		Protocol: string(s.servicePort.Protocol),
	}
}

type ContainerPortForwardTarget struct {
	containerName string
	podName       string
	port          int
	portName      string
	protocol      string
}

func (s ContainerPortForwardTarget) GetInfo(_ context.Context, _ kubernetes.Clientset, _ string) (*PortForwardInfo, error) {
	return &PortForwardInfo{
		PodName:       s.podName,
		ContainerName: s.containerName,
		Port:          s.port,
	}, nil
}

func (s ContainerPortForwardTarget) GetTableRow() PortForwardTableRow {
	return PortForwardTableRow{
		Type:     "Container port",
		Name:     fmt.Sprintf("%s / %s (in pod: %s)", s.containerName, s.portName, s.podName),
		Port:     strconv.Itoa(s.port),
		PortName: s.portName,
		Protocol: s.protocol,
	}
}

type ContainerAnyPortForwardTarget struct {
	containerName string
	podName       string
}

func (s ContainerAnyPortForwardTarget) GetInfo(_ context.Context, _ kubernetes.Clientset, _ string) (*PortForwardInfo, error) {
	port := ""
	confirm := true

	for {
		f := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Container port").Value(&port),
			huh.NewConfirm().Affirmative("Continue").Negative("Cancel").Value(&confirm),
		))

		err := f.Run()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch input: %w", err)
		}

		if !confirm {
			return nil, nil
		}

		intPort, err := strconv.Atoi(port)
		if err != nil {
			logrus.Warnf("failed to convert port %s to int: %v", port, err)
		} else if intPort < 1 || intPort > 65535 {
			logrus.Warnf("port %s out of range", port)
		} else {
			return &PortForwardInfo{
				PodName:       s.podName,
				ContainerName: s.containerName,
				Port:          intPort,
			}, nil
		}

	}
}

func (s ContainerAnyPortForwardTarget) GetTableRow() PortForwardTableRow {
	return PortForwardTableRow{
		Type: "Container",
		Name: fmt.Sprintf("%s (pod: %s)", s.containerName, s.podName),
		Port: "Any",
	}
}
