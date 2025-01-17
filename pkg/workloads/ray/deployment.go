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

package ray

import (
	_ "embed"
	"fmt"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
)

type Deployment struct{}

type DeploymentFlags struct {
	Serveconfig string
}

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const ServeconfigFilename = "serveconfig"

func (deployment Deployment) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	logrus.Debugf("Loading ray service from %s", execFlags.Path)

	contents, err := os.ReadFile(filepath.Join(execFlags.Path, ServeconfigFilename))

	if err != nil {
		return nil, fmt.Errorf("failed to read serveconfig file: %w", err)
	}

	return DeploymentFlags{Serveconfig: string(contents)}, nil
}

func (deployment Deployment) DefaultTemplate() ([]byte, error) {
	if DeploymentTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return DeploymentTemplate, nil
}

//func (service Deployment) GenerateName() string {
//	return utils.BuildWorkloadName(service.Shared.Name, service.Shared.Path, service.Deployment.Image)
//}

func (deployment Deployment) IgnoreFiles() []string {
	return []string{ServeconfigFilename, workloads.KaiwoconfigFilename}
}

func (deployment Deployment) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (deployment Deployment) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (deployment Deployment) GenerateAdditionalResourceManifests(_ workloads.WorkloadTemplateConfig) ([]*unstructured.Unstructured, error) {
	return []*unstructured.Unstructured{}, nil
}
