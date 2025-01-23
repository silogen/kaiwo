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

package deployments

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const EntrypointFilename = "entrypoint"

type Deployment struct{}

type DeploymentFlags struct {
	Entrypoint string
}

func (deployment Deployment) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(filepath.Join(execFlags.Path, EntrypointFilename))
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnln("No entrypoint file found. Expecting entrypoint in image")
			return DeploymentFlags{Entrypoint: ""}, nil
		} else {
			return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
		}
	}

	entrypoint := string(contents)

	entrypoint = strings.ReplaceAll(entrypoint, "\n", " ")    // Flatten multiline string
	entrypoint = strings.ReplaceAll(entrypoint, "\"", "\\\"") // Escape double quotes
	entrypoint = fmt.Sprintf("\"%s\"", entrypoint)            // Wrap the entire command in quotes

	return DeploymentFlags{Entrypoint: entrypoint}, nil

}

func (deployment Deployment) ConvertObject(object runtime.Object) (runtime.Object, bool) {
	obj, ok := object.(*appsv1.Deployment)
	return obj, ok
}

func (deployment Deployment) DefaultTemplate() ([]byte, error) {
	if DeploymentTemplate == nil {
		return nil, fmt.Errorf("deployment template is empty")
	}
	return DeploymentTemplate, nil
}

func (deployment Deployment) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename}
}

func (deployment Deployment) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (deployment Deployment) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (deployment Deployment) GenerateAdditionalResourceManifests(k8sClient client.Client, templateContext workloads.WorkloadTemplateConfig) ([]runtime.Object, error) {
	return []runtime.Object{}, nil
}
