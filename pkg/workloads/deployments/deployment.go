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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/silogen/kaiwo/pkg/k8s"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/pkg/workloads"
)

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const EntrypointFilename = "entrypoint"

//
//type DeploymentReference struct {
//	Deployment  *appsv1.Deployment
//	Replicasets []ReplicaSetReference
//}
//
//type ReplicaSetReference struct {
//	ReplicaSet *appsv1.ReplicaSet
//	Pods       []corev1.Pod
//}
//
//// GetWorkloadWrapper builds the workload wrapper from a deployment
//func (d *DeploymentReference) GetWorkloadWrapper() (workloads.WorkloadReference, error) {
//
//	rootWrapper := workloads.WorkloadReference{
//		Object: d.Deployment,
//		ObjectKey: client.ObjectKey{
//			Namespace: d.Deployment.Namespace,
//			Name:      d.Deployment.Name,
//		},
//		GVK:    appsv1.SchemeGroupVersion.WithKind("Deployment"),
//		IsLeaf: false,
//	}
//
//	for _, replicaSet := range d.Replicasets {
//
//		replicasetWrapper := workloads.WorkloadReference{
//			Object: replicaSet.ReplicaSet,
//			ObjectKey: client.ObjectKey{
//				Namespace: d.Deployment.Namespace,
//				Name:      replicaSet.ReplicaSet.Name,
//			},
//			GVK:    appsv1.SchemeGroupVersion.WithKind("ReplicaSet"),
//			IsLeaf: true,
//		}
//		rootWrapper.Children = append(rootWrapper.Children, replicasetWrapper)
//
//		for _, pod := range replicaSet.Pods {
//			replicasetWrapper.Pods = append(replicasetWrapper.Pods, pod)
//		}
//	}
//
//	return rootWrapper, nil
//}

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

func (deployment Deployment) BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (*workloads.WorkloadReference, error) {
	obj := &appsv1.Deployment{}
	var gvk schema.GroupVersionKind

	logrus.Debugf("Building deployment reference for %s / %s", key.Name, key.Namespace)

	if err := k8sClient.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("could not get deployment: %w", err)
	}

	scheme, err := k8s.GetScheme()
	if err != nil {
		return nil, fmt.Errorf("could not get k8s scheme: %w", err)
	}

	gvk, err = apiutil.GVKForObject(obj, &scheme)

	if err != nil {
		return nil, fmt.Errorf("could not get k8s GVK: %w", err)
	}

	reference := workloads.WorkloadReference{
		Object: obj,
		IsLeaf: false,
		GVK:    gvk,
	}

	replicaSets := &appsv1.ReplicaSetList{}
	labelSelector := client.MatchingLabels(obj.Spec.Selector.MatchLabels)

	if err := k8sClient.List(ctx, replicaSets, client.InNamespace(key.Namespace), labelSelector); err != nil {
		return nil, fmt.Errorf("could not list replicasets: %w", err)
	}

	for _, replicaSet := range replicaSets.Items {

		gvk, err = apiutil.GVKForObject(&replicaSet, &scheme)

		if err != nil {
			return nil, fmt.Errorf("could not get k8s GVK: %w", err)
		}

		replicasetWrapper := &workloads.WorkloadReference{
			Object: &replicaSet,
			IsLeaf: true,
			GVK:    gvk,
		}
		reference.Children = append(reference.Children, replicasetWrapper)

		pods := &corev1.PodList{}
		if err := k8sClient.List(ctx, pods, client.InNamespace(key.Namespace), client.MatchingLabels(replicaSet.Spec.Selector.MatchLabels)); err != nil {
			return nil, fmt.Errorf("could not list pods: %w", err)
		}

		replicasetWrapper.Pods = append(replicasetWrapper.Pods, pods.Items...)
	}

	return &reference, nil

}
