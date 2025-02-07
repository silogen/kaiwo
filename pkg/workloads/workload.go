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

package workloads

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/silogen/kaiwo/pkg/k8s"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkloadBase is the base object that all workloads share
type WorkloadBase struct{}

func (wb WorkloadBase) GetName() string {
	return wb.GetObject().GetName()
}

func (wb WorkloadBase) GetNamespace() string {
	return wb.GetObject().GetNamespace()
}

func (wb WorkloadBase) GetKaiwoUser() string {
	return wb.GetObject().GetLabels()[KaiwoUsernameLabel]
}

func (wb WorkloadBase) GetObject() client.Object {
	panic("Method should be overridden")
}

func (wb WorkloadBase) GetServices(_ context.Context, _ client.Client) ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (wb WorkloadBase) SetFromObject(_ client.Object) error {
	panic("Method should be overridden")
}

func (wb WorkloadBase) SetFromTemplate(workloadTemplate []byte, templateContext WorkloadTemplateConfig) error {
	parsedTemplate, err := template.New("main").Funcs(sprig.TxtFuncMap()).Parse(string(workloadTemplate))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var renderedYAML strings.Builder
	err = parsedTemplate.Execute(&renderedYAML, templateContext)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	scheme, err := k8s.GetScheme()
	if err != nil {
		return fmt.Errorf("failed to fetch scheme: %w", err)
	}

	decoder := serializer.NewCodecFactory(&scheme).UniversalDeserializer()

	obj, _, err := decoder.Decode([]byte(renderedYAML.String()), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to decode manifest: %w", err)
	}

	converted, ok := obj.(client.Object)

	if !ok {
		return fmt.Errorf("failed to decode manifest")
	}

	return wb.SetFromObject(converted)
}

// LoadFromObjectKey loads the workload from Kubernetes based on the object key (name and namespace)
func (wb WorkloadBase) LoadFromObjectKey(ctx context.Context, k8sClient client.Client, key client.ObjectKey) error {
	base := wb.GetObject().DeepCopyObject().(client.Object)
	err := k8sClient.Get(ctx, key, base)
	if err != nil {
		return err
	}
	return wb.SetFromObject(base)
}

func (wb WorkloadBase) Reload(ctx context.Context, k8sClient client.Client) error {
	return wb.LoadFromObjectKey(ctx, k8sClient, client.ObjectKey{
		Namespace: wb.GetNamespace(),
		Name:      wb.GetName(),
	})
}

func (wb WorkloadBase) GenerateAdditionalResourceManifests(_ context.Context, _ client.Client, _ WorkloadTemplateConfig) ([]client.Object, error) {
	return []client.Object{}, nil
}

// Workload represents the generic workload object
type Workload interface {
	// GenerateTemplateContext creates the workload-specific context that can be passed to the template
	GenerateTemplateContext(ExecFlags) (any, error)

	// DefaultTemplate returns a default template to use for this workload
	DefaultTemplate() ([]byte, error)

	SetFromTemplate([]byte, WorkloadTemplateConfig) error

	// IgnoreFiles lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string

	// GenerateAdditionalResourceManifests returns additional manifests that are required for creating the workload
	GenerateAdditionalResourceManifests(context.Context, client.Client, WorkloadTemplateConfig) ([]client.Object, error)

	// Reload loads the current state from Kubernetes again
	Reload(context.Context, client.Client) error

	// LoadFromObjectKey loads the state from Kubernetes given an object key (name and namespace)
	LoadFromObjectKey(context.Context, client.Client, client.ObjectKey) error

	// ResolveStructure loads the current expanded structure from k8s to make it easier to interact with the workload
	ResolveStructure(ctx context.Context, k8sClient client.Client) error

	// ListKnownPods returns the pods that the reference is currently aware of
	ListKnownPods() []WorkloadPod

	// GetStatus returns a human-readable status of the workload
	GetStatus() string

	GetName() string

	GetObject() client.Object

	GetNamespace() string

	GetKaiwoUser() string

	GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error)
}

type WorkloadPod struct {
	Pod          corev1.Pod
	LogicalGroup string
}
