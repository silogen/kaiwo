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
	"bufio"
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"slices"
	"strings"

	 corev1 "k8s.io/api/core/v1"
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
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}

	return configMap, nil
}

type SecretVolumeItem struct {
	Key  string
	Path string
}

type SecretVolume struct {
	Name      string
	Items     []SecretVolumeItem
}

func readEnvFile(filePath string) ([]corev1.EnvVar, []SecretVolume, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	var envVars []corev1.EnvVar
	var volumes []SecretVolume

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid env file format: %s", line)
		}

		key := parts[0]
		value := parts[1]

		// Handle environment variables from secrets (format: "secretName:key")
		if strings.Contains(value, ":") {
			refParts := strings.SplitN(value, ":", 2)
			envVars = append(envVars, corev1.EnvVar{
				Name: key,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: refParts[0]},
						Key:                  refParts[1],
					},
				},
			})
		} else if strings.HasPrefix(key, "MOUNT_SECRET_") {
			// Handle secret volume mounting (e.g., MOUNT_SECRET_XXX=/sub_path:secret:secret-key)
			mountParts := strings.Split(value, ":")
			if len(mountParts) < 3 {
				return nil, nil, fmt.Errorf("invalid MOUNT_SECRET format: %s", value)
			}

			secretName := strings.TrimPrefix(key, "MOUNT_SECRET_")
			secretKey := mountParts[0]
			subPath := mountParts[1]

			// Add volume definition
			found := false
			for i := range volumes {
				if volumes[i].Name == secretName {
					volumes[i].Items = append(volumes[i].Items, SecretVolumeItem{
						Key:  secretKey,
						Path: subPath,
					})
					found = true
					break
				}
			}

			if !found {
				volumes = append(volumes, SecretVolume{
					Name:      secretName,
					Items: []SecretVolumeItem{
						{
							Key:  secretKey,
							Path: subPath,
						},
					},
				})
			}
		} else {
			// Handle direct environment variable values
			envVars = append(envVars, corev1.EnvVar{
				Name:  key,
				Value: value,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to read env file: %w", err)
	}

	return envVars, volumes, nil
}
