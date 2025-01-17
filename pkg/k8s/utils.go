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

package k8s

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"slices"
)

func isBinaryFile(content []byte) bool {
	return bytes.Contains(content, []byte{0})
}

// GenerateConfigMapFromDir generates a ConfigMap from a directory
func GenerateConfigMapFromDir(dir string, name string, namespace string, skipFiles []string) (*unstructured.Unstructured, error) {
	files, err := os.ReadDir(dir)

	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
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
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Skip binary files
		if isBinaryFile(content) {
			logrus.Warnf("Skipping binary file: %s", file.Name())
			continue
		}
		data[file.Name()] = string(content)
	}

	if len(data) == 0 {
		return nil, nil
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}

	return configMap, nil
}

type SecretVolume struct {
	Name       string
	SecretName string
	Key        string
	SubPath    string
	MountPath  string
}
type EnvVarInput struct {
	Name       string `yaml:"name,omitempty"`
	Value      string `yaml:"value,omitempty"`
	FromSecret *struct {
		Name   string `yaml:"name"`
		Secret string `yaml:"secret"`
		Key    string `yaml:"key"`
	} `yaml:"fromSecret,omitempty"`
	MountSecret *struct {
		Name   string `yaml:"name"`
		Secret string `yaml:"secret"`
		Key    string `yaml:"key"`
		Path   string `yaml:"path"`
	} `yaml:"mountSecret,omitempty"`
}

type EnvFile struct {
	EnvVars []EnvVarInput `yaml:"envVars"`
}

func ReadEnvFile(filePath string) ([]corev1.EnvVar, []SecretVolume, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	var envFile EnvFile
	if err := yaml.NewDecoder(file).Decode(&envFile); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var envVars []corev1.EnvVar
	var secretVolumes []SecretVolume

	for _, input := range envFile.EnvVars {
		if input.Value != "" {
			// Normal environment variable
			envVars = append(envVars, corev1.EnvVar{
				Name:  input.Name,
				Value: input.Value,
			})
		} else if input.FromSecret != nil {
			// Secret-based environment variable
			envVars = append(envVars, corev1.EnvVar{
				Name: input.FromSecret.Name,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: input.FromSecret.Secret,
						},
						Key: input.FromSecret.Key,
					},
				},
			})
		} else if input.MountSecret != nil {
			// Secret-based volume mount
			secretVolumes = append(secretVolumes, SecretVolume{
				Name:       fmt.Sprintf("%s-volume", input.MountSecret.Secret),
				SecretName: input.MountSecret.Secret,
				Key:        input.MountSecret.Key,
				SubPath:    filepath.Base(input.MountSecret.Path), // File name to mount
				MountPath:  input.MountSecret.Path,
			})
			envVars = append(envVars, corev1.EnvVar{
				Name:  input.MountSecret.Name,
				Value: input.MountSecret.Path, // Set the mount path as an environment variable
			})
		}
	}

	return envVars, secretVolumes, nil
}
