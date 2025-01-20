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

package jobs

import (
	_ "embed"
	"fmt"
	"github.com/silogen/kaiwo/pkg/workloads"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"strings"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const EntrypointFilename = "entrypoint"

type Job struct{}

type JobFlags struct {
	Entrypoint string
}

func (job Job) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(filepath.Join(execFlags.Path, EntrypointFilename))

	if contents == nil {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	entrypoint := string(contents)

	entrypoint = strings.ReplaceAll(entrypoint, "\n", " ")    // Flatten multiline string
	entrypoint = strings.ReplaceAll(entrypoint, "\"", "\\\"") // Escape double quotes
	entrypoint = fmt.Sprintf("\"%s\"", entrypoint)            // Wrap the entire command in quotes

	return JobFlags{Entrypoint: entrypoint}, nil

}

func (job Job) DefaultTemplate() ([]byte, error) {
	if JobTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return JobTemplate, nil
}

func (job Job) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename}
}

func (job Job) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (job Job) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (job Job) GenerateAdditionalResourceManifests(templateContext workloads.WorkloadTemplateConfig) ([]*unstructured.Unstructured, error) {
	localClusterQueueManifest, err := workloads.CreateLocalClusterQueueManifest(templateContext)

	if err != nil {
		return nil, fmt.Errorf("failed to create local cluster queue manifest: %w", err)
	}

	return []*unstructured.Unstructured{localClusterQueueManifest}, nil
}
