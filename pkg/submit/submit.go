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
	"os/user"
	"path/filepath"
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

func getLastPartOfPath(path string) string {
	lastPart := filepath.Base(path)
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(lastPart, "/", "-"), "_", "-"))
}

func setWorkloadName(workloadName string, path string) string {
	if workloadName == "" {
		currentUser, err := user.Current()
		if err != nil {
			panic(fmt.Sprintf("Failed to fetch the current user: %v", err))
		}
		lastPartFromPath := getLastPartOfPath(path)
		return strings.Join([]string{currentUser.Username, lastPartFromPath}, "-")
	}
	return workloadName
}


func Submit(args templates.WorkloadArgs) error {
	if !args.DryRun {
		return fmt.Errorf("live run is not supported yet, please use --dry-run")
	}

	if err := templates.ValidateWorkloadArgs(args); err != nil {
		return err
	}

	args.Name = setWorkloadName(args.Name, args.Path)

	logrus.Infof("Submitting workload '%s'", args.Name)

	if args.Type == "" {
		args.Type = "job"

	}

	loader := templates.GetWorkloadLoader(args.Type)

	// Load workload
	if err := loader.Load(args.Path); err != nil {
		return fmt.Errorf("failed to load workload: %w", err)
	}

	// Generate ConfigMap
	configMap, err := k8s.GenerateConfigMapFromDir(args.Path, args.Name, args.Namespace, loader.IgnoreFiles())
	if err != nil {
		return fmt.Errorf("failed to generate ConfigMap: %w", err)
	}

	var workloadTemplate []byte

	// Generate main manifest
	if args.TemplatePath == "" {
		// Use the default template
		workloadTemplate = loader.DefaultTemplate()
	} else {
		// Use the provided template
		logrus.Infof("Using custom template: %s", args.TemplatePath)
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

	// TODO only create namespace if it doesn't exist (we don't want to delete namespaces when deleting resources)
	// namespace := k8s.CreateNamespace(args.Namespace)

	workloadManifests := append([]runtime.Object{&configMap}, templatedManifests...)

	if args.DryRun {
		logrus.Info("Dry-run. Printing generated workload to console")
		yamlPrinter := printers.YAMLPrinter{}

		printedYaml := strings.Builder{}
		for _, obj := range workloadManifests {
			err = yamlPrinter.PrintObj(obj, &printedYaml)
			if err != nil {
				logrus.Errorf("failed to marshal object: %v", err)
			} else {
				fmt.Println(string(printedYaml.String()))
			}
			printedYaml.Reset()
		}
		return nil
	}

	logrus.Info("Workload submitted successfully.")
	return nil
}
