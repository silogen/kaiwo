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
	"context"
	"fmt"
	"github.com/Masterminds/sprig/v3"
	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"os"
	"sigs.k8s.io/yaml"
	"strings"
	"text/template"
)

// ApplyWorkload runs the main workload submission routine
func ApplyWorkload(
	ctx context.Context,
	client dynamic.Interface,
	workload Workload,
	execFlags ExecFlags,
	templateContext WorkloadTemplateConfig,
) error {

	var resources []*unstructured.Unstructured

	if execFlags.CreateNamespace {
		namespaceResource, err := generateNamespaceManifestIfNotExists(ctx, client, templateContext.Meta.Namespace)
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

	additionalResourceManifests, err := workload.GenerateAdditionalResourceManifests(templateContext)

	if err != nil {
		return fmt.Errorf("failed to generate additional resource manifests: %w", err)
	}
	if additionalResourceManifests != nil && len(additionalResourceManifests) > 0 {
		resources = append(resources, additionalResourceManifests...)
	}

	// Choose the template
	var workloadTemplate []byte
	if execFlags.Template != "" {
		logrus.Infof("Using custom template: %s", execFlags.Template)
		workloadTemplate, err = os.ReadFile(execFlags.Template)
		if err != nil {
			return fmt.Errorf("failed to read template file: %w", err)
		}
	} else {
		workloadTemplate, err = workload.DefaultTemplate()
		if err != nil {
			return fmt.Errorf("failed to fetch default workload template: %w", err)
		}
	}

	templateResources, err := generateManifests(workloadTemplate, templateContext)
	if err != nil {
		return fmt.Errorf("failed to generate manifests: %w", err)
	}
	if templateResources == nil || len(templateResources) == 0 {
		return fmt.Errorf("failed to generate manifests: no resources found")
	}
	resources = append(resources, templateResources...)

	if execFlags.DryRun {
		printResources(resources)
	} else {
		if err := applyResources(resources, ctx, client); err != nil {
			return fmt.Errorf("failed to apply resources: %w", err)
		}
	}
	return nil
}

func generateNamespaceManifestIfNotExists(ctx context.Context, client dynamic.Interface, namespaceName string) (*unstructured.Unstructured, error) {
	logrus.Infof("Checking if namespace '%s' exists", namespaceName)

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

	_, err := client.Resource(gvr).Get(ctx, namespaceName, metav1.GetOptions{})
	if err == nil {
		logrus.Infof("Namespace '%s' already exists", namespaceName)
		return nil, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check namespace existence: %w", err)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": namespaceName,
			},
		},
	}, nil
}

// generateConfigMapManifest adds a config map resource
func generateConfigMapManifest(path string, workload Workload, metaConfig MetaFlags) (*unstructured.Unstructured, error) {
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
func generateManifests(workloadTemplate []byte, templateContext WorkloadTemplateConfig) ([]*unstructured.Unstructured, error) {
	parsedTemplate, err := template.New("main").Funcs(sprig.TxtFuncMap()).Parse(string(workloadTemplate))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var renderedYAML strings.Builder
	err = parsedTemplate.Execute(&renderedYAML, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	decoder := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	manifests := strings.Split(renderedYAML.String(), "---")

	var parsedManifests []*unstructured.Unstructured

	for _, manifest := range manifests {
		manifest = strings.TrimSpace(manifest)
		if manifest == "" {
			continue
		}

		obj := &unstructured.Unstructured{}
		_, _, err := decoder.Decode([]byte(manifest), nil, obj)
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML manifest: %w", err)
		}

		parsedManifests = append(parsedManifests, obj)
	}

	return parsedManifests, nil

}

// printResources prints each Kubernetes manifest in an array
func printResources(resources []*unstructured.Unstructured) {
	for _, resource := range resources {
		logrus.Infof("Generated %s: %s", resource.GetKind(), resource.GetName())
		data := resource.UnstructuredContent()

		// Marshal the map into YAML
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			logrus.Errorf("failed to convert unstructured object to YAML: %w", err)
		}
		fmt.Print(string(yamlBytes))
		fmt.Println("---")
	}
}

// applyResources applies (creates or updates if possible) each Kubernetes object within an array
func applyResources(resources []*unstructured.Unstructured, ctx context.Context, c dynamic.Interface) error {

	for _, resource := range resources {
		gvk := resource.GroupVersionKind()

		// Derive the GVR from the GVK
		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: strings.ToLower(gvk.Kind) + "s", // Pluralize the kind
		}

		var resourceInterface dynamic.ResourceInterface

		if gvk.Kind == "Namespace" {
			resourceInterface = c.Resource(gvr)
		} else {
			// Determine the namespace
			namespace := resource.GetNamespace()
			if namespace == "" {
				namespace = "default"
			}

			resourceInterface = c.Resource(gvr).Namespace(namespace)
		}

		// Try to create the resource
		_, err := resourceInterface.Create(ctx, resource, metav1.CreateOptions{})
		if err == nil {
			logrus.Infof("%s/%s submitted successfully", resource.GetKind(), resource.GetName())
			continue
		}

		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to apply %s/%s: %w", resource.GetKind(), resource.GetName(), err)
		} else {
			logrus.Warnf("%s/%s already exists. Skipping submit", resource.GetKind(), resource.GetName())
		}

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
	return nil
}
