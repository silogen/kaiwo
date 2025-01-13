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
	"context"
	_ "embed"
	"fmt"
	"github.com/silogen/ai-workload-orchestrator/pkg/k8s"
	"github.com/silogen/ai-workload-orchestrator/pkg/templates"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
)

//go:embed rayjob.yaml.tmpl
var JobTemplate []byte

const EntrypointFilename = "entrypoint"

type JobLoader struct {
	Entrypoint string
	Kueue      k8s.KueueArgs
}

func (r *JobLoader) Load(path string) error {

	contents, err := os.ReadFile(filepath.Join(path, EntrypointFilename))

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = string(contents)

	client, err := k8s.GetDynamicClient()

	if err != nil {
		return err
	}

	gpuCount, err := k8s.GetDefaultResourceFlavorGpuCount(context.TODO(), client)

	if err != nil {
		return err
	}

	r.Kueue = k8s.KueueArgs{
		NodeGpuCount: gpuCount,
	}

	return nil
}

func (r *JobLoader) DefaultTemplate() []byte {
	return JobTemplate
}

func (r *JobLoader) IgnoreFiles() []string {
	return []string{EntrypointFilename}
}

func (r *JobLoader) ModifyResources(resources *[]*unstructured.Unstructured, args templates.WorkloadArgs) error {

	c, err := k8s.GetDynamicClient()
	if err != nil {
		return err
	}

	// Handle kueue local queue
	localQueue, err := k8s.PrepareLocalClusterQueue(args, c)
	if err != nil {
		return err
	}

	*resources = append(*resources, localQueue)

	return nil
}
