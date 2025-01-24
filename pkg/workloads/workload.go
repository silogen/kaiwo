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

package workloads

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Workload represents the workload as it is meant to be created
type Workload interface {

	// GenerateTemplateContext creates the workload-specific context that can be passed to the template
	GenerateTemplateContext(ExecFlags) (any, error)

	//// GenerateName provides a name that can be used to describe the workload
	//GenerateName() string

	// DefaultTemplate returns a default template to use for this workload
	DefaultTemplate() ([]byte, error)

	ConvertObject(object runtime.Object) (runtime.Object, bool)

	// IgnoreFiles lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string

	// GetPods returns a list of pods that are associated with this workload
	GetPods() ([]corev1.Pod, error)

	// GetServices returns a list of services that are associated with this workload
	GetServices() ([]corev1.Service, error)

	// GenerateAdditionalResourceManifests allow
	GenerateAdditionalResourceManifests(k8sClient client.Client, templateContext WorkloadTemplateConfig) ([]runtime.Object, error)

	// BuildReference builds the workload reference from this workload
	BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (*WorkloadReference, error)
}

// WorkloadReference contains all the primary resources that represent a particular workload
type WorkloadReference struct {
	// Object is the primary Kubernetes object
	Object client.Object

	// Pods lists any pods that this resource manages, if any
	Pods []corev1.Pod

	// IsLeaf denotes whether this wrapper should contain any pods
	IsLeaf bool

	// Children list any direct descendents this wrapper logically owns
	Children []*WorkloadReference

	GVK schema.GroupVersionKind
}

//func (w WorkloadReference) GetGVK() schema.GroupVersionKind {
//	return w.Object.GetObjectKind().GroupVersionKind()
//}

func (w WorkloadReference) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: w.Object.GetNamespace(),
		Name:      w.Object.GetName(),
	}
}

func (w WorkloadReference) String() string {
	gvk := w.GVK
	return fmt.Sprintf("%s/%s %s (%s/%s)", gvk.Group, gvk.Version, gvk.Kind, w.Object.GetNamespace(), w.Object.GetName())
}

func (w WorkloadReference) GetPodsRecursive() []corev1.Pod {
	return getPodsRecursive(&w)
}

func getPodsRecursive(w *WorkloadReference) []corev1.Pod {
	var pods []corev1.Pod

	if w.IsLeaf {
		pods = append(pods, w.Pods...)
	} else {
		for _, child := range w.Children {
			pods = append(pods, child.GetPodsRecursive()...)
		}
	}
	return pods
}
