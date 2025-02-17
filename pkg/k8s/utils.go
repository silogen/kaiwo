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

package k8s

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
)

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

//type EnvFile struct {
//	EnvVars []EnvVarInput `yaml:"envVars"`
//}
//
//func ReadEnvFile(filePath string) ([]corev1.EnvVar, []SecretVolume, error) {
//	file, err := os.Open(filePath)
//	if err != nil {
//		return nil, nil, fmt.Errorf("failed to open env file: %w", err)
//	}
//	defer func() {
//		if cerr := file.Close(); cerr != nil {
//			// Log the error, as defer cannot modify the return values of the outer function
//			logrus.Warnf("failed to close file: %v", cerr)
//		}
//	}()
//
//	var envFile EnvFile
//	if err := yaml.NewDecoder(file).Decode(&envFile); err != nil {
//		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
//	}
//
//	var envVars []corev1.EnvVar
//	var secretVolumes []SecretVolume
//
//	for _, input := range envFile.EnvVars {
//		if input.Value != "" {
//			// Normal environment variable
//			envVars = append(envVars, corev1.EnvVar{
//				Name:  input.Name,
//				Value: input.Value,
//			})
//		} else if input.FromSecret != nil {
//			// Secret-based environment variable
//			envVars = append(envVars, corev1.EnvVar{
//				Name: input.FromSecret.Name,
//				ValueFrom: &corev1.EnvVarSource{
//					SecretKeyRef: &corev1.SecretKeySelector{
//						LocalObjectReference: corev1.LocalObjectReference{
//							Name: input.FromSecret.Secret,
//						},
//						Key: input.FromSecret.Key,
//					},
//				},
//			})
//		} else if input.MountSecret != nil {
//			// Secret-based volume mount
//			secretVolumes = append(secretVolumes, SecretVolume{
//				Name:       fmt.Sprintf("%s-volume", input.MountSecret.Secret),
//				SecretName: input.MountSecret.Secret,
//				Key:        input.MountSecret.Key,
//				SubPath:    filepath.Base(input.MountSecret.Path), // File name to mount
//				MountPath:  input.MountSecret.Path,
//			})
//			envVars = append(envVars, corev1.EnvVar{
//				Name:  input.MountSecret.Name,
//				Value: input.MountSecret.Path, // Set the mount path as an environment variable
//			})
//		}
//	}
//
//	return envVars, secretVolumes, nil
//}

// MinimalizeAndConvertToYAML converts a runtime.Object or client.Object to its YAML representation
// while removing read-only fields like `metadata.creationTimestamp`, `status`, and others.
func MinimalizeAndConvertToYAML(s *runtime.Scheme, obj runtime.Object) (string, error) {
	// Convert the resource to an unstructured map

	gvks, _, err := s.ObjectKinds(obj)
	if err != nil {
		return "", fmt.Errorf("failed to get GVK for object: %w", err)
	}
	if len(gvks) > 0 {
		obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return "", fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	// Inject apiVersion and kind into the unstructured map, as this is not always included in the runtime.Object
	gvk := obj.GetObjectKind().GroupVersionKind()
	groupVersion := gvk.GroupVersion()
	if groupVersion.Group == "" {
		unstructuredMap["apiVersion"] = groupVersion.Version
	} else {
		unstructuredMap["apiVersion"] = fmt.Sprintf("%s/%s", groupVersion.Group, groupVersion.Version)
	}

	unstructuredMap["kind"] = gvk.Kind

	// Remove unwanted fields
	removeUnwantedFields(unstructuredMap)

	var b bytes.Buffer

	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2)
	err = yamlEncoder.Encode(&unstructuredMap)
	if err != nil {
		return "", fmt.Errorf("failed to convert object to yaml: %w", err)
	}

	return b.String(), nil
}

// removeUnwantedFields removes common server-side generated fields
// TODO find out if there is a better way to do this
func removeUnwantedFields(obj map[string]any) {
	if metadata, ok := obj["metadata"].(map[string]any); ok {
		delete(metadata, "creationTimestamp")
		delete(metadata, "managedFields")
		delete(metadata, "uid")
		delete(metadata, "selfLink")
		delete(metadata, "generation")
	}

	// Remove the status field
	delete(obj, "status")
}
