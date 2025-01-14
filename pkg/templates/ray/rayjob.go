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
	"github.com/silogen/ai-workload-orchestrator/pkg/utils"
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

func (r *JobLoader) Load(args utils.WorkloadArgs) error {

	contents, err := os.ReadFile(filepath.Join(args.Path, EntrypointFilename))

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = string(contents)

	client, err := k8s.GetDynamicClient()

	if err != nil {
		return err
	}

	// TODO make labelKey dynamic
	gpuCount, err := k8s.GetDefaultResourceFlavorGpuCount(context.TODO(), client, "beta.amd.com/gpu.family.AI")

	if err != nil {
		return err
	}

	numReplicas, nodeGpuRequest := k8s.CalculateNumberOfReplicas(args.GPUs, gpuCount)

	r.Kueue = k8s.KueueArgs{
		GPUsAvailablePerNode:    gpuCount,
		RequestedGPUsPerReplica: nodeGpuRequest,
		RequestedNumReplicas:    numReplicas,
	}

	return nil
}

func (r *JobLoader) DefaultTemplate() []byte {
	return JobTemplate
}

func (r *JobLoader) IgnoreFiles() []string {
	return []string{EntrypointFilename, utils.KAIWOCONFIG_FILENAME}
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