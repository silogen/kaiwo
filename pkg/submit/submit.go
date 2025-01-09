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
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/silogen/ai-workload-orchestrator/pkg/k8s"
	"github.com/silogen/ai-workload-orchestrator/pkg/templates"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
)

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

	// TODO Include username from whoami
	var workload_name string
	if args.Name == "" {
		workload_name = strings.ToLower(strings.Join(strings.FieldsFunc(args.Path, func(r rune) bool {
			return r == '/' || r == '_'
		}), "-"))
		args.Name = workload_name
	} else {
		workload_name = args.Name
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
	configMap, err := k8s.GenerateConfigMapFromDir(args.Path, workload_name, args.Namespace, loader.IgnoreFiles())
	if err != nil {
		return fmt.Errorf("failed to generate ConfigMap: %w", err)
	}

	var workloadTemplate []byte

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

	// Marshal the template into a Kubernetes object
	templatedManifests, err := k8s.DecodeYAMLToObjects(renderedYAML.String())

	if err != nil {
		return fmt.Errorf("failed to decode workload manifest YAML: %w", err)
	}

	// Create namespace

	namespace := k8s.CreateNamespace(args.Namespace)

	workloadManifests := append([]runtime.Object{namespace, &configMap}, templatedManifests...)

	if args.DryRun {
		logrus.Info("Dry run enabled, printing generated workload to console")

		// configMapYAML, err := yaml.Marshal(configMap)
		yamlPrinter := printers.YAMLPrinter{}

		printedYaml := strings.Builder{}
		for _, obj := range workloadManifests {
			err = yamlPrinter.PrintObj(obj, &printedYaml)
			if err != nil {
				logrus.Errorf("failed to marshal object: %v", err)
			} else {
				logrus.Info("Object:")
				fmt.Println(string(printedYaml.String()))
				fmt.Println("---")
			}
			printedYaml.Reset()
		}
		return nil
	}

	logrus.Info("Workload submitted successfully.")
	return nil
}
