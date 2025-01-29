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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (WorkloadReference, error)
}

type WorkloadPod struct {
	Pod          corev1.Pod
	LogicalGroup string
}

type WorkloadReferenceBase struct {
	WorkloadObject client.Object
}

func (workloadReferenceBase WorkloadReferenceBase) GetName() string {
	return workloadReferenceBase.WorkloadObject.GetName()
}

func (workloadReferenceBase WorkloadReferenceBase) GetNamespace() string {
	return workloadReferenceBase.WorkloadObject.GetNamespace()
}

func (workloadReferenceBase WorkloadReferenceBase) GetKaiwoUser() string {
	return workloadReferenceBase.WorkloadObject.GetLabels()[KaiwoUsernameLabel]
}

type WorkloadReference interface {
	// Load loads the current state from k8s
	Load(ctx context.Context, k8sClient client.Client) error

	// GetPods returns the pods that the reference is currently aware of
	GetPods() []WorkloadPod

	GetStatus() string

	GetName() string

	GetNamespace() string

	GetKaiwoUser() string
}
