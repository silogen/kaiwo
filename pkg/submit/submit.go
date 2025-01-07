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
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ConfigMap struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   map[string]string `yaml:"metadata"`
	Data       map[string]string `yaml:"data"`
}

const ENTRYPOINT_FILENAME = "entrypoint"
const SERVECONFIG_FILENAME = "serveconfig"

var skipFiles = map[string]struct{}{
	ENTRYPOINT_FILENAME:  {},
	SERVECONFIG_FILENAME: {},
}

func readTemplate(templatePath string) (map[string]interface{}, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	var template map[string]interface{}
	if err := yaml.Unmarshal(content, &template); err != nil {
		return nil, err
	}

	return template, nil
}

func writeTemplate(outputPath string, content map[string]interface{}) error {
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	yamlContent, err := yaml.Marshal(content)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, yamlContent, 0644)
}

func transformServeConfig(templatePath, outputPath string, metadataName, namespace string) error {
	template, err := readTemplate(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read serve config template: %w", err)
	}

	metadata := template["metadata"].(map[string]interface{})
	metadata["name"] = metadataName
	metadata["namespace"] = namespace

	return writeTemplate(outputPath, template)
}

func transformEntrypoint(templatePath, outputPath string, entrypointContent string) error {
	template, err := readTemplate(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read entrypoint template: %w", err)
	}

	spec := template["spec"].(map[string]interface{})
	spec["entrypoint"] = fmt.Sprintf("/bin/bash -c \"%s\"", entrypointContent)

	return writeTemplate(outputPath, template)
}

func isBinaryFile(content []byte) bool {
	return bytes.Contains(content, []byte{0})
}

// Generate ConfigMap from a directory
func generateConfigMap(dir string, configmap_name string, namespace string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	data := make(map[string]string)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if _, skip := skipFiles[file.Name()]; skip {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Skip binary files
		if isBinaryFile(content) {
			logrus.Warnf("Skipping binary file: %s", file.Name())
			continue
		}

		// Use YAML literal block style for all non-binary files
		data[file.Name()] = string(content)
	}

	configMap := ConfigMap{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Metadata: map[string]string{
			"name":      configmap_name,
			"namespace": namespace,
		},
		Data: data,
	}

	yamlData, err := yaml.Marshal(&configMap)
	if err != nil {
		return fmt.Errorf("failed to marshal ConfigMap: %w", err)
	}

	outputDir := filepath.Join(dir, "compiled")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create compiled directory: %w", err)
	}

	outputPath := filepath.Join(outputDir, "configmap.yaml")
	if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write ConfigMap to file: %w", err)
	}

	logrus.Infof("ConfigMap successfully generated at %s", outputPath)
	return nil
}

func formatJSON(content []byte) (string, error) {
	var prettyJSON map[string]interface{}
	if err := yaml.Unmarshal(content, &prettyJSON); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	formatted, err := yaml.Marshal(prettyJSON)
	if err != nil {
		return "", fmt.Errorf("failed to format JSON: %w", err)
	}

	return string(formatted), nil
}

func Submit(path, image string, job, service bool, gpus int) error {
	if path == "" || (!job && !service) || (job && service) || gpus <= 0 {
		return fmt.Errorf("invalid flags: ensure --path, --job/--service, and --gpus are provided")
	}

	logrus.Infof("Submitting workload from path: %s", path)

	workload_name := strings.ToLower(strings.Join(strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '_'
	}), "-"))

	namespace := "av-test" //to be fixed by another PR

	generateConfigMap(path, workload_name, namespace)

	if service {
		serveConfigPath := filepath.Join(path, SERVECONFIG_FILENAME)
		serveOutputPath := filepath.Join(path, "compiled", "ray-services", "serveconfig.yaml")
		if err := transformServeConfig(serveConfigPath, serveOutputPath, "vllm-distributed-inference-test", "placeholder-namespace"); err != nil {
			return fmt.Errorf("failed to transform serveconfig: %w", err)
		}
	}

	if job {
		entrypointPath := filepath.Join(path, ENTRYPOINT_FILENAME)
		entrypointContent, err := os.ReadFile(entrypointPath)
		if err != nil {
			return fmt.Errorf("failed to read entrypoint file: %w", err)
		}
		entrypointOutputPath := filepath.Join(path, "compiled", "ray-jobs", "entrypoint.yaml")
		if err := transformEntrypoint(entrypointTemplatePath, entrypointOutputPath, string(entrypointContent)); err != nil {
			return fmt.Errorf("failed to transform entrypoint: %w", err)
		}
	}

	logrus.Info("Workload submitted successfully.")
	return nil
}
