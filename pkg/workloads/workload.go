/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package workloads

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Workload interface {

	// GenerateTemplateContext creates the workload-specific context that can be passed to the template
	GenerateTemplateContext(ExecFlags) (any, error)

	//// GenerateName provides a name that can be used to describe the workload
	//GenerateName() string

	// DefaultTemplate returns a default template to use for this workload
	DefaultTemplate() ([]byte, error)

	// IgnoreFiles lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string

	// GetPods returns a list of pods that are associated with this workload
	GetPods() ([]corev1.Pod, error)

	// GetServices returns a list of services that are associated with this workload
	GetServices() ([]corev1.Service, error)

	// GenerateAdditionalResourceManifests allow
	GenerateAdditionalResourceManifests(WorkloadTemplateConfig) ([]*unstructured.Unstructured, error)
}
