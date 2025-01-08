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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/silogen/ai-workload-orchestrator/pkg/templates"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func isBinaryFile(content []byte) bool {
	return bytes.Contains(content, []byte{0})
}

// Generate ConfigMap from a directory
func generateConfigMap(dir string, configmap_name string, namespace string, skipFiles []string) (v1.ConfigMap, error) {
	files, err := os.ReadDir(dir)

	configMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmap_name,
			Namespace: namespace,
		},
		Data: make(map[string]string),
	}

	if err != nil {
		return configMap, fmt.Errorf("failed to read directory: %w", err)
	}

	data := make(map[string]string)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if slices.Contains(skipFiles, file.Name()) {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return configMap, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Skip binary files
		if isBinaryFile(content) {
			logrus.Warnf("Skipping binary file: %s", file.Name())
			continue
		}
		data[file.Name()] = string(content)
	}

	configMap.Data = data

	return configMap, nil
}

// TODO clarify naming?
type TemplateConfig struct {
	// The base workload args
	Base templates.WorkloadArgs

	// The type-specific workload args
	Workload templates.WorkloadLoader
}

func Submit(args templates.WorkloadArgs) error {

	if !args.DryRun {
		return fmt.Errorf("live run is not supported yet, please use --dry-run")
	}

	if err := templates.ValidateWorkloadArgs(args); err != nil {
		return err
	}

	var workload_name string
	if args.Name == "" {
		workload_name = strings.ToLower(strings.Join(strings.FieldsFunc(args.Path, func(r rune) bool {
			return r == '/' || r == '_'
		}), "-"))
	} else {
		workload_name = args.Name
		args.Name = workload_name
	}

	logrus.Infof("Submitting workload '%s' from path: %s", workload_name, args.Path)

	loader, err := templates.GetWorkloadLoader(args.Type)

	if err != nil {
		return fmt.Errorf("failed to get workload loader: %w", err)
	}

	// Load workload
	if err := loader.Load(args.Path); err != nil {
		return fmt.Errorf("failed to load workload: %w", err)
	}

	// Generate ConfigMap
	configMap, err := generateConfigMap(args.Path, workload_name, args.Namespace, loader.IgnoreFiles())
	if err != nil {
		return fmt.Errorf("failed to generate ConfigMap: %w", err)
	}

	var workloadTemplate []byte = make([]byte, 0)

	// Generate main manifest
	if args.TemplatePath == "" {
		// Use the default template
		logrus.Info("Using default template")
		workloadTemplate = loader.DefaultTemplate()
	} else {
		// Use the provided template
		logrus.Infof("Using template: %s", args.TemplatePath)
		workloadTemplate, err = os.ReadFile(args.TemplatePath)
		if err != nil {
			return fmt.Errorf("failed to read template file: %w", err)
		}
	}

	templateContext := TemplateConfig{
		Base:     args,
		Workload: loader,
	}

	parsedTemplate, err := template.New("main").Funcs(sprig.TxtFuncMap()).Parse(string(workloadTemplate))

	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Render the template
	var renderedYAML strings.Builder
	err = parsedTemplate.Execute(&renderedYAML, templateContext)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	if args.DryRun {
		logrus.Info("Dry run enabled, printing generated workload to console")
		logrus.Info("Workload manifest:")
		fmt.Println(renderedYAML.String())

		configMapYAML, err := yaml.Marshal(configMap)
		if err != nil {
			logrus.Errorf("failed to marshal ConfigMap: %v", err)
		}

		logrus.Info("ConfigMap:")
		fmt.Println(string(configMapYAML))
		return nil
	}

	logrus.Info("Workload submitted successfully.")
	return nil
}
