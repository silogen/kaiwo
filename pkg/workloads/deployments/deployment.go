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

package deployments

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/pkg/workloads"
)

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const EntrypointFilename = "entrypoint"

type Deployment struct{}

type DeploymentFlags struct {
	Entrypoint string
}

func (deployment Deployment) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(filepath.Join(execFlags.Path, EntrypointFilename))
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

func (deployment Deployment) ConvertObject(object runtime.Object) (runtime.Object, bool) {
	obj, ok := object.(*appsv1.Deployment)
	return obj, ok
}

func (deployment Deployment) DefaultTemplate() ([]byte, error) {
	if DeploymentTemplate == nil {
		return nil, fmt.Errorf("deployment template is empty")
	}
	return DeploymentTemplate, nil
}

func (deployment Deployment) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename, workloads.TemplateFileName}
}

func (deployment Deployment) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (deployment Deployment) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (deployment Deployment) GenerateAdditionalResourceManifests(k8sClient client.Client, templateContext workloads.WorkloadTemplateConfig) ([]runtime.Object, error) {
	return []runtime.Object{}, nil
}

func (deployment Deployment) BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (workloads.WorkloadReference, error) {
	obj := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("could not get deployment: %w", err)
	}
	deploymentRef := &DeploymentReference{
		Deployment: *obj,
	}
	return deploymentRef, nil
}

type DeploymentReference struct {
	Deployment  appsv1.Deployment
	ReplicaSets []ReplicaSetReference
}

type ReplicaSetReference struct {
	ReplicaSet appsv1.ReplicaSet
	Pods       []corev1.Pod
}

func (deploymentRef *DeploymentReference) Load(ctx context.Context, k8sClient client.Client) error {
	logrus.Debugf("Loading deployment reference %s", deploymentRef.Deployment.Name)
	replicaSets := &appsv1.ReplicaSetList{}
	labelSelector := client.MatchingLabels(deploymentRef.Deployment.Spec.Selector.MatchLabels)

	if err := k8sClient.List(ctx, replicaSets, client.InNamespace(deploymentRef.Deployment.Namespace), labelSelector); err != nil {
		return fmt.Errorf("could not list replicasets: %w", err)
	}

	deploymentRef.ReplicaSets = nil

	for _, replicaSet := range replicaSets.Items {

		logrus.Debugf("Found ReplicaSet %s", replicaSet.Name)

		replicaSetRef := ReplicaSetReference{
			ReplicaSet: replicaSet,
		}

		pods := &corev1.PodList{}
		if err := k8sClient.List(ctx, pods, client.InNamespace(deploymentRef.Deployment.Namespace), client.MatchingLabels(replicaSet.Spec.Selector.MatchLabels)); err != nil {
			return fmt.Errorf("could not list pods: %w", err)
		}

		for _, pod := range pods.Items {
			logrus.Debugf("Found Pod %s", pod.Name)
			replicaSetRef.Pods = append(replicaSetRef.Pods, pod)
		}

		deploymentRef.ReplicaSets = append(deploymentRef.ReplicaSets, replicaSetRef)
	}

	return nil
}

func (deploymentRef *DeploymentReference) GetPods() []workloads.WorkloadPod {
	var workloadPods []workloads.WorkloadPod

	logrus.Debugf("Getting pods for deployment %s", deploymentRef.Deployment.Name)

	for _, replicaSet := range deploymentRef.ReplicaSets {
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

func (deploymentRef *DeploymentReference) GetName() string {
	return deploymentRef.Deployment.GetName()
}

func (deploymentRef *DeploymentReference) GetStatus() string {
	return "N/A (TODO)"
}
