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

package ray

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
)

type Deployment struct {
	workloads.WorkloadBase
	RayService rayv1.RayService
	RayCluster *rayv1.RayCluster
	HeadPod    *corev1.Pod
	WorkerPods []*corev1.Pod
}

type DeploymentFlags struct {
	Serveconfig string
}

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const ServeconfigFilename = "serveconfig"

func (d *Deployment) DefaultTemplate() ([]byte, error) {
	if DeploymentTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return DeploymentTemplate, nil
}

func (d *Deployment) IgnoreFiles() []string {
	return []string{ServeconfigFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename, workloads.TemplateFileName}
}

func (d *Deployment) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(execFlags.WorkloadFiles[ServeconfigFilename])
	if err != nil {
		return nil, fmt.Errorf("failed to read serveconfig file: %w", err)
	}

	return DeploymentFlags{Serveconfig: strings.TrimSpace(string(contents))}, nil
}

func (d *Deployment) GetObject() client.Object {
	return &d.RayService
}

func (d *Deployment) SetFromObject(obj client.Object) error {
	converted, ok := obj.(*rayv1.RayService)
	if !ok {
		return fmt.Errorf("expected Deployment, got %T", obj)
	}
	d.RayService = *converted
	return nil
}

func (d *Deployment) ResolveStructure(ctx context.Context, k8sClient client.Client) error {
	// Fetch ray cluster

	clusterLabelSelector := client.MatchingLabels{
		"ray.io/originated-from-cr-name": d.RayService.Name,
		"ray.io/originated-from-crd":     "RayService",
	}

	logrus.Debugln(clusterLabelSelector)

	clusterList := &rayv1.RayClusterList{}
	if err := k8sClient.List(ctx, clusterList, clusterLabelSelector); err != nil {
		return fmt.Errorf("could not list clusters: %w", err)
	}
	if len(clusterList.Items) == 0 {
		return fmt.Errorf("no clusters found")
	}
	if len(clusterList.Items) > 1 {
		return fmt.Errorf("more than one clusters found")
	}
	d.RayCluster = &clusterList.Items[0]

	clusterPodLabelSelector := client.MatchingLabels{
		"ray.io/cluster": d.RayCluster.Name,
	}

	clusterPodList := &corev1.PodList{}
	if err := k8sClient.List(ctx, clusterPodList, clusterPodLabelSelector); err != nil {
		return fmt.Errorf("could not list cluster pods: %w", err)
	}

	// Clear
	d.HeadPod = nil
	d.WorkerPods = []*corev1.Pod{}

	for _, pod := range clusterPodList.Items {
		nodeType := pod.Labels["ray.io/node-type"]
		if nodeType == "worker" {
			d.WorkerPods = append(d.WorkerPods, &pod)
		} else if nodeType == "head" {
			if d.HeadPod == nil {
				d.HeadPod = &pod
			} else {
				logrus.Warn("More than one head pod encountered")
			}
		} else {
			logrus.Warnf("Encountered unknown node type: %s", nodeType)
		}
	}

	return nil
}

func (d *Deployment) ListKnownPods() []workloads.WorkloadPod {
	var pods []workloads.WorkloadPod

	if d.HeadPod != nil {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *d.HeadPod,
			LogicalGroup: "head",
		})
	} else {
		logrus.Debug("No head pod")
	}

	for _, pod := range d.WorkerPods {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *pod,
			LogicalGroup: "worker",
		})
	}
	return pods
}

func (d *Deployment) GetStatus() string {
	return "N/A (TODO)"
}

func (d *Deployment) GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error) {
	if d.RayCluster == nil {
		return []corev1.Service{}, fmt.Errorf("no ray cluster set, run ServiceReference.Load() first")
	}

	serviceLabelSelector := client.MatchingLabels{
		"ray.io/cluster": d.RayCluster.Name,
	}
	serviceList := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, serviceList, serviceLabelSelector); err != nil {
		return nil, fmt.Errorf("could not list services: %w", err)
	}

	return serviceList.Items, nil
}
