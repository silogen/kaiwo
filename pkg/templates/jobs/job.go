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
	"github.com/silogen/ai-workload-orchestrator/pkg/k8s"
	"github.com/silogen/ai-workload-orchestrator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"strings"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const EntrypointFilename = "entrypoint"

type JobLoader struct {
	Entrypoint string
}

func (r *JobLoader) Load(args utils.WorkloadArgs) error {

	contents, err := os.ReadFile(filepath.Join(args.Path, EntrypointFilename))

	if contents == nil {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = strings.ReplaceAll(string(contents), "\n", " ") // Flatten multiline string
	r.Entrypoint = strings.ReplaceAll(r.Entrypoint, "\"", "\\\"")  // Escape double quotes
	r.Entrypoint = fmt.Sprintf("\"%s\"", r.Entrypoint)             // Wrap the entire command in quotes

	return nil
}

func (r *JobLoader) DefaultTemplate() []byte {
	return JobTemplate
}

func (r *JobLoader) IgnoreFiles() []string {
	return []string{EntrypointFilename, utils.KaiwoconfigFilename}
}

func (r *JobLoader) AdditionalResources(resources *[]*unstructured.Unstructured, args utils.WorkloadArgs) error {
	c, err := k8s.GetDynamicClient()
	if err != nil {
		return err
	}

	// Handle kueue local queue
	localQueue, err := k8s.PrepareLocalClusterQueue(args.Queue, args.Namespace, c)
	if err != nil {
		return err
	}

	*resources = append(*resources, localQueue)

	return nil
}
