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

package deployments

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
)

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const EntrypointFilename = "entrypoint"

// Deployment is a workload wrapper around the basic Kubernetes Deployment
type Deployment struct {
	workloads.WorkloadBase
	Deployment appsv1.Deployment

	// ReplicaSets caches the replica sets linked to this deployment
	ReplicaSets []ReplicaSetReference
}

type ReplicaSetReference struct {
	ReplicaSet appsv1.ReplicaSet

	// Pods caches the pods linked to this replica set
	Pods []corev1.Pod
}

type DeploymentFlags struct {
	Entrypoint string
}

func (d *Deployment) DefaultTemplate() ([]byte, error) {
	if DeploymentTemplate == nil {
		return nil, fmt.Errorf("deployment template is empty")
	}
	return DeploymentTemplate, nil
}

func (d *Deployment) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename, workloads.TemplateFileName}
}

func (d *Deployment) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(execFlags.WorkloadFiles[EntrypointFilename])
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnln("No entrypoint file found. Expecting entrypoint in image")
			return DeploymentFlags{Entrypoint: ""}, nil
		} else {
			return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
		}
	}

	entrypoint := string(contents)

	entrypoint = strings.ReplaceAll(entrypoint, "\n", " ")    // Flatten multiline string
	entrypoint = strings.ReplaceAll(entrypoint, "\"", "\\\"") // Escape double quotes
	entrypoint = fmt.Sprintf("\"%s\"", entrypoint)            // Wrap the entire command in quotes

	return DeploymentFlags{Entrypoint: entrypoint}, nil
}

func (d *Deployment) GetObject() client.Object {
	return &d.Deployment
}

func (d *Deployment) SetFromObject(obj client.Object) error {
	converted, ok := obj.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("expected Deployment, got %T", obj)
	}
	d.Deployment = *converted
	return nil
}

func (d *Deployment) ResolveStructure(ctx context.Context, k8sClient client.Client) error {
	logrus.Debugf("Loading deployment reference %s", d.Deployment.Name)
	replicaSets := &appsv1.ReplicaSetList{}
	labelSelector := client.MatchingLabels(d.Deployment.Spec.Selector.MatchLabels)

	if err := k8sClient.List(ctx, replicaSets, client.InNamespace(d.Deployment.Namespace), labelSelector); err != nil {
		return fmt.Errorf("could not list replicasets: %w", err)
	}

	d.ReplicaSets = nil

	for _, replicaSet := range replicaSets.Items {

		logrus.Debugf("Found ReplicaSet %s", replicaSet.Name)

		replicaSetRef := ReplicaSetReference{
			ReplicaSet: replicaSet,
		}

		pods := &corev1.PodList{}
		if err := k8sClient.List(ctx, pods, client.InNamespace(d.Deployment.Namespace), client.MatchingLabels(replicaSet.Spec.Selector.MatchLabels)); err != nil {
			return fmt.Errorf("could not list pods: %w", err)
		}

		for _, pod := range pods.Items {
			logrus.Debugf("Found Pod %s", pod.Name)
			replicaSetRef.Pods = append(replicaSetRef.Pods, pod)
		}

		d.ReplicaSets = append(d.ReplicaSets, replicaSetRef)
	}

	return nil
}

func (d *Deployment) ListKnownPods() []workloads.WorkloadPod {
	var workloadPods []workloads.WorkloadPod

	logrus.Debugf("Getting pods for deployment %s", d.Deployment.Name)

	for _, replicaSet := range d.ReplicaSets {
		logrus.Debugf("Found replica set %s", replicaSet.ReplicaSet.Name)
		for _, pod := range replicaSet.Pods {
			logrus.Debugf("Found pod %s", pod.Name)
			workloadPods = append(workloadPods, workloads.WorkloadPod{
				Pod:          pod,
				LogicalGroup: fmt.Sprintf("ReplicaSet: %s", replicaSet.ReplicaSet.Name),
			})
		}
	}

	return workloadPods
}

func (d *Deployment) GetStatus() string {
	return "N/A (TODO)"
}
