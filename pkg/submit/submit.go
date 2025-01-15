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

package submit

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"os"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/silogen/kaiwo/pkg/templates"
	"github.com/silogen/kaiwo/pkg/utils"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
)

// TODO clarify naming?
type TemplateConfig struct {
	// The base workload args
	Base utils.WorkloadArgs

	// The type-specific workload args
	Workload      templates.WorkloadLoader
	EnvVars       []corev1.EnvVar
	SecretVolumes []k8s.SecretVolume
}

func Submit(args utils.WorkloadArgs) error {

	var envVars []corev1.EnvVar
	var secretVolumes []k8s.SecretVolume
	envFilePath := filepath.Join(args.Path, utils.ENV_FILENAME)
	if _, err := os.Stat(envFilePath); err == nil {
		logrus.Infof("Found env file at %s, parsing environment variables and secret volumes", envFilePath)
		envVars, secretVolumes, err = k8s.ReadEnvFile(envFilePath)
		if err != nil {
			return fmt.Errorf("failed to parse env file: %w", err)
		}
		logrus.Infof("Parsed %d environment variables and %d secret volumes from env file", len(envVars), len(secretVolumes))
	}

	args, loader, err := initializeLoader(args, envVars)

	if err != nil {
		return err
	}

	var c dynamic.Interface

	if !args.DryRun {
		logrus.Infof("Initializing Kubernetes client")
		c, err = k8s.InitializeClient()
		if err != nil {
			return fmt.Errorf("failed to initialize Kubernetes client: %v", err)
		}
	}

	resources := []*unstructured.Unstructured{}

	// Handle namespace creation
	if args.CreateNamespace {
		err := addNamespaceResource(args, c, &resources)
		if err != nil {
			return err
		}
	}

	if args.Path != "" {
		args, err = addConfigMapResource(args, loader, &resources)
		if err != nil {
			return err
		}
	}

	if err := loader.AdditionalResources(&resources, args); err != nil {
		return err
	}

	// Process workload template
	err = processWorkloadTemplate(args, loader, &resources, envVars, secretVolumes)
	if err != nil {
		return err
	}

	if args.DryRun {
		printResources(resources)
	} else {
		err = applyResources(resources, c)
		if err != nil {
			return fmt.Errorf("failed to apply resources: %w", err)
		}
	}

	return nil
}

// initializeLoader validates and initializes the workload loader
func initializeLoader(args utils.WorkloadArgs, envVars []corev1.EnvVar) (utils.WorkloadArgs, templates.WorkloadLoader, error) {
	if err := templates.ValidateWorkloadArgs(args); err != nil {
		return args, nil, err
	}

	args.Name = setWorkloadName(args.Name, args.Path, args.Image)

	if args.Type == "" {
		args.Type = "job"
	}

	loader := templates.GetWorkloadLoader(args.Type)
	if args.Path != "" {
		if err := loader.Load(args, envVars); err != nil {
			return args, nil, fmt.Errorf("failed to load workload: %w", err)
		}
	}

	return args, loader, nil
}

func setWorkloadName(workloadName string, path string, image string) string {
	if workloadName == "" {
		currentUser, err := user.Current()
		if err != nil {
			panic(fmt.Sprintf("Failed to fetch the current user: %v", err))
		}

		var appendix string

		if path != "" {
			appendix = sanitizeStringForKubernetes(filepath.Base(path))
		} else {
			appendix = sanitizeStringForKubernetes(image)
		}
		return strings.Join([]string{currentUser.Username, appendix}, "-")
	}
	return workloadName
}

func sanitizeStringForKubernetes(path string) string {
	replacer := strings.NewReplacer(
		":", "-",
		"/", "-",
		"\\", "-",
		"_", "-",
		".", "-",
	)
	return strings.ToLower(replacer.Replace(path))
}

// addNamespaceResource adds a namespace resource if needed
func addNamespaceResource(args utils.WorkloadArgs, c dynamic.Interface, resources *[]*unstructured.Unstructured) error {
	namespace := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": args.Namespace,
			},
		},
	}

	if args.DryRun {
		logrus.Info("Including namespace definition, skipping existence check due to dry-run mode")
		*resources = append(*resources, &namespace)
		return nil
	}

	logrus.Infof("Checking if namespace '%s' exists", args.Namespace)
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err := c.Resource(gvr).Get(context.TODO(), args.Namespace, metav1.GetOptions{})
	if err == nil {
		logrus.Infof("Namespace '%s' already exists", args.Namespace)
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check namespace existence: %v", err)
	}

	*resources = append(*resources, &namespace)
	return nil
}

// addConfigMapResource adds a config map resource
func addConfigMapResource(args utils.WorkloadArgs, loader templates.WorkloadLoader, resources *[]*unstructured.Unstructured) (utils.WorkloadArgs, error) {
	configMap, err := k8s.GenerateConfigMapFromDir(args.Path, args.Name, args.Namespace, loader.IgnoreFiles())
	if err != nil {
		return args, fmt.Errorf("failed to generate ConfigMap: %w", err)
	}
	if configMap != nil {
		*resources = append(*resources, configMap)
		args.ConfigMap = true
	}
	return args, nil
}

// processWorkloadTemplate renders and parses the workload template
func processWorkloadTemplate(
	args utils.WorkloadArgs,
	loader templates.WorkloadLoader,
	resources *[]*unstructured.Unstructured,
	envVars []corev1.EnvVar,
	secretVolumes []k8s.SecretVolume,
) error {
	var workloadTemplate []byte
	var err error

	if args.TemplatePath == "" {
		workloadTemplate = loader.DefaultTemplate()
	} else {
		logrus.Infof("Using custom template: %s", args.TemplatePath)
		workloadTemplate, err = os.ReadFile(args.TemplatePath)
		if err != nil {
			return fmt.Errorf("failed to read template file: %w", err)
		}
	}

	templateContext := TemplateConfig{
		Base:          args,
		Workload:      loader,
		EnvVars:       envVars,
		SecretVolumes: secretVolumes,
	}

	parsedTemplate, err := template.New("main").Funcs(sprig.TxtFuncMap()).Parse(string(workloadTemplate))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var renderedYAML strings.Builder
	err = parsedTemplate.Execute(&renderedYAML, templateContext)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	decoder := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	manifests := strings.Split(renderedYAML.String(), "---")

	for _, manifest := range manifests {
		manifest = strings.TrimSpace(manifest)
		if manifest == "" {
			continue
		}

		obj := &unstructured.Unstructured{}
		_, _, err := decoder.Decode([]byte(manifest), nil, obj)
		if err != nil {
			return fmt.Errorf("failed to decode YAML manifest: %v", err)
		}

		*resources = append(*resources, obj)
	}
	return nil
}

func printResources(resources []*unstructured.Unstructured) {
	for _, resource := range resources {
		logrus.Infof("Generated %s: %s", resource.GetKind(), resource.GetName())
		data := resource.UnstructuredContent()

		// Marshal the map into YAML
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			logrus.Errorf("failed to convert unstructured object to YAML: %v", err)
		}
		fmt.Print(string(yamlBytes))
		fmt.Println("---")
	}
}

func applyResources(resources []*unstructured.Unstructured, c dynamic.Interface) error {

	for _, resource := range resources {
		gvk := resource.GroupVersionKind()

		// Derive the GVR from the GVK
		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: strings.ToLower(gvk.Kind) + "s", // Pluralize the kind
		}

		// Determine the namespace
		namespace := resource.GetNamespace()
		if namespace == "" {
			namespace = "default"
		}

		// Try to create the resource
		_, err := c.Resource(gvr).Namespace(namespace).Create(context.TODO(), resource, metav1.CreateOptions{})
		if err == nil {
			logrus.Infof("%s/%s submitted successfully", resource.GetKind(), resource.GetName())
			continue
		}

		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to apply %s/%s: %v", resource.GetKind(), resource.GetName(), err)
		} else {
			logrus.Warnf("%s/%s already exists. Skipping submit", resource.GetKind(), resource.GetName())
		}

		// TODO: Rethink update logic which now fails with "immutable field" errors
		// Resource already exists, update it
		// existing, err := c.Resource(gvr).Namespace(namespace).Get(context.TODO(), resource.GetName(), metav1.GetOptions{})
		// if err != nil {
		// 	return fmt.Errorf("failed to get existing %s/%s: %v", resource.GetKind(), resource.GetName(), err)
		// }

		// resource.SetResourceVersion(existing.GetResourceVersion())
		// _, err = c.Resource(gvr).Namespace(namespace).Update(context.TODO(), resource, metav1.UpdateOptions{})
		// if err != nil {
		// 	return fmt.Errorf("failed to update %s/%s: %v", resource.GetKind(), resource.GetName(), err)
		// }

		// logrus.Infof("%s/%s updated successfully", resource.GetKind(), resource.GetName())
	}
	return nil
}
