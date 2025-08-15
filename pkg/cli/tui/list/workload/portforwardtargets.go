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
