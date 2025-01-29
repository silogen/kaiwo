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
	"errors"
	"fmt"
	"path"

	"github.com/Masterminds/sprig/v3"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"

	"os"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime"
)

const TemplateFileName = "template"

// ApplyWorkload runs the main workload submission routine
func ApplyWorkload(
	ctx context.Context,
	k8sClient client.Client,
	workload Workload,
	execFlags ExecFlags,
	templateContext WorkloadTemplateConfig,
) error {

	var resources []runtime.Object

	if execFlags.CreateNamespace {
		namespaceResource, err := generateNamespaceManifestIfNotExists(ctx, k8sClient, templateContext.Meta.Namespace)
		if err != nil {
			return fmt.Errorf("failed to generate namespace resource: %w", err)
		}
		if namespaceResource != nil {
			resources = append(resources, namespaceResource)
		}
	}

	if execFlags.Path != "" {
		configMapResource, err := generateConfigMapManifest(execFlags.Path, workload, templateContext.Meta)
		if err != nil {
			return fmt.Errorf("failed to generate configmap resource: %w", err)
		}
		if configMapResource != nil {
			resources = append(resources, configMapResource)
			templateContext.Meta.HasConfigMap = true
		}
	}

	workloadTemplate, err := getWorkloadTemplate(execFlags, workload)
	if err != nil {
		return fmt.Errorf("failed to get workload template: %w", err)
	}

	templateResources, err := generateManifests(k8sClient, workloadTemplate, templateContext, workload)
	if err != nil {
		return fmt.Errorf("Check workload type. Failed to generate manifests: %w", err)
	}
	if len(templateResources) == 0 {
		return fmt.Errorf("failed to generate manifests: no resources found")
	}
	resources = append(resources, templateResources...)

	s, err := k8s.GetScheme()
	if err != nil {
		return fmt.Errorf("failed to get k8s scheme: %w", err)
	}

	if execFlags.DryRun {
		printResources(&s, resources)
	} else {
		if err := applyResources(resources, ctx, k8sClient); err != nil {
			return fmt.Errorf("failed to apply resources: %w", err)
		}
	}
	return nil
}

func getWorkloadTemplate(execFlags ExecFlags, workload Workload) ([]byte, error) {
	// Check if a custom template is explicitly provided
	if execFlags.Template != "" {
		logrus.Infof("Using custom template: %s", execFlags.Template)
		return readTemplateFile(execFlags.Template)
	}

	// Check for a template file in the specified path
	templateFilePath := path.Join(execFlags.Path, TemplateFileName)
	if exists, err := fileExists(templateFilePath); err != nil {
		return nil, fmt.Errorf("failed to check custom template file: %w", err)
	} else if exists {
		logrus.Infof("Using custom template %s instead of default template", templateFilePath)
		return readTemplateFile(templateFilePath)
	}

	return workload.DefaultTemplate()
}

func readTemplateFile(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file %s: %w", filePath, err)
	}
	return data, nil
}

func fileExists(filePath string) (bool, error) {
	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func generateNamespaceManifestIfNotExists(
	ctx context.Context,
	k8sClient client.Client,
	namespaceName string,
) (*corev1.Namespace, error) {
	logrus.Debugf("Checking if namespace '%s' exists", namespaceName)

	ns := &corev1.Namespace{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)
	if err == nil {
		logrus.Info("Namespace already exists")
		return nil, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check namespace existence: %w", err)
	}

	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}, nil
}

// generateConfigMapManifest adds a config map resource
func generateConfigMapManifest(path string, workload Workload, metaConfig MetaFlags) (*corev1.ConfigMap, error) {
	configMap, err := k8s.GenerateConfigMapFromDir(path, metaConfig.Name, metaConfig.Namespace, workload.IgnoreFiles())
	if err != nil {
		return nil, fmt.Errorf("failed to generate ConfigMap: %w", err)
	}
	if configMap != nil {
		return configMap, nil
	}
	return nil, nil
}

// generateManifests prepares a list of Kubernetes manifests to apply
func generateManifests(k8sClient client.Client, workloadTemplate []byte, templateContext WorkloadTemplateConfig, workload Workload) ([]runtime.Object, error) {
	parsedTemplate, err := template.New("main").Funcs(sprig.TxtFuncMap()).Parse(string(workloadTemplate))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var renderedYAML strings.Builder
	err = parsedTemplate.Execute(&renderedYAML, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	scheme, err := k8s.GetScheme()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scheme: %w", err)
	}

	decoder := serializer.NewCodecFactory(&scheme).UniversalDeserializer()

	obj, _, err := decoder.Decode([]byte(renderedYAML.String()), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	converted, ok := workload.ConvertObject(obj)
	if !ok {
		return nil, fmt.Errorf("failed to convert manifest, ensure it is of the correct type")
	}

	additionalWorkloadManifests, err := workload.GenerateAdditionalResourceManifests(k8sClient, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to generate additional resource manifests: %w", err)
	}

	return append(additionalWorkloadManifests, []runtime.Object{converted}...), nil

}

// printResources prints each Kubernetes manifest in an array
func printResources(s *runtime.Scheme, resources []runtime.Object) {
	for _, resource := range resources {
		clientObject := resource.(client.Object)

		cleanedResource, err := k8s.MinimalizeAndConvertToYAML(s, clientObject)

		if err != nil {
			logrus.Errorf("Failed to marshal object to YAML %s: %v", clientObject.GetName(), err)
			continue
		}

		fmt.Print(cleanedResource)
		fmt.Println("---")
	}
}

// applyResources applies (creates or updates if possible) each Kubernetes object within an array
func applyResources(resources []runtime.Object, ctx context.Context, k8sClient client.Client) error {

	for _, resource := range resources {
		// Ensure the resource implements client.Object
		obj, ok := resource.(client.Object)
		if !ok {
			return fmt.Errorf("resource does not implement client.Object: %T", resource)
		}

		// Access metadata for logging
		objMeta, err := meta.Accessor(obj)
		if err != nil {
			return fmt.Errorf("failed to access metadata for resource: %w", err)
		}

		logrus.Debugf("Applying resource %T: %s/%s", resource, objMeta.GetNamespace(), objMeta.GetName())

		// Check if the resource exists
		key := client.ObjectKey{
			Namespace: objMeta.GetNamespace(),
			Name:      objMeta.GetName(),
		}

		existing := resource.DeepCopyObject().(client.Object)

		err = k8sClient.Get(ctx, key, existing)

		if err == nil {
			logrus.Warnf("%s/%s already exists. Skipping submit. Use --version flag if you really want to create another resource of this kind", objMeta.GetNamespace(), objMeta.GetName())
			continue
		}

		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get resource %s/%s: %w", objMeta.GetNamespace(), objMeta.GetName(), err)
		}

		if err := k8sClient.Create(ctx, obj); err != nil {
			return fmt.Errorf("failed to create resource %s/%s: %w", objMeta.GetNamespace(), objMeta.GetName(), err)
		}

		logrus.Infof("resource %s/%s created successfully", objMeta.GetNamespace(), objMeta.GetName())

		continue

		// TODO: Rethink update logic which now fails with "immutable field" errors
		// Resource already exists, update it
		// existing, err := c.Resource(gvr).Namespace(namespace).Get(ctx, resource.GetName(), metav1.GetOptions{})
		// if err != nil {
		// 	return fmt.Errorf("failed to get existing %s/%s: %w", resource.GetKind(), resource.GetName(), err)
		// }

		// resource.SetResourceVersion(existing.GetResourceVersion())
		// _, err = c.Resource(gvr).Namespace(namespace).Update(ctx, resource, metav1.UpdateOptions{})
		// if err != nil {
		// 	return fmt.Errorf("failed to update %s/%s: %w", resource.GetKind(), resource.GetName(), err)
		// }

		// logrus.Infof("%s/%s updated successfully", resource.GetKind(), resource.GetName())
	}
	logrus.Info("To monitor and manage your workloads interactively, run $ kaiwo list -n mynamespace")

	return nil
}
