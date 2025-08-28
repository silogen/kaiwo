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

package scheduling

import (
	"math"
	"strconv"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// CalculateNumberOfReplicas attempts to balance the number of replicas by maximizing the number of GPUs used per node
func CalculateNumberOfReplicas(requestedGpus int, gpusPerNode int, envVars []corev1.EnvVar) (int, int) {
	// TODO handle cases where nodes are not empty and some GPUs are in use

	numReplicas := 0
	nodeGpuRequest := 0

	// Retrieve PIPELINE_PARALLELISM and TENSOR_PARALLELISM from envVars, if they exist
	var pipelineParallelism, tensorParallelism int
	if envVars != nil {
		for _, envVar := range envVars {
			switch envVar.Name {
			case "PIPELINE_PARALLELISM":
				val, err := strconv.Atoi(envVar.Value)
				if err == nil {
					pipelineParallelism = val
				} else {
					logrus.Warnf("Invalid PIPELINE_PARALLELISM value: %s", envVar.Value)
				}
			case "TENSOR_PARALLELISM":
				val, err := strconv.Atoi(envVar.Value)
				if err == nil {
					tensorParallelism = val
				} else {
					logrus.Warnf("Invalid TENSOR_PARALLELISM value: %s", envVar.Value)
				}
			}
		}

		// If PIPELINE_PARALLELISM and TENSOR_PARALLELISM are set, enforce their values
		if pipelineParallelism > 1 && tensorParallelism > 0 {
			numReplicas = pipelineParallelism
			nodeGpuRequest = tensorParallelism

			logrus.Infof("Found GPU scheduling info from env vars, PIPELINE_PARALLELISM: %d, TENSOR_PARALLELISM: %d", pipelineParallelism, tensorParallelism)

			if numReplicas*tensorParallelism != requestedGpus || tensorParallelism > gpusPerNode {
				logrus.Fatalf(
					"Mismatch between requested GPUs (%d) and calculated GPUs (%d) from PIPELINE_PARALLELISM (%d) and TENSOR_PARALLELISM (%d)",
					requestedGpus, numReplicas*tensorParallelism, pipelineParallelism, tensorParallelism,
				)
			}
			return numReplicas, nodeGpuRequest
		}
	}

	if tensorParallelism > gpusPerNode {
		logrus.Warnf("TENSOR_PARALLELISM (%d) exceeds available GPUs per node (%d). This will have significant negative performance impact unless you have extremely fast and low latency network.",
			tensorParallelism, gpusPerNode)
	}

	// Default logic if PIPELINE_PARALLELISM and TENSOR_PARALLELISM are not set
	if requestedGpus < 0 {
		logrus.Warnf("Cannot determine number of replicas for negative GPUs")
	} else if requestedGpus == 0 {
		// TODO determine if rational logic
		numReplicas = 1
	} else if requestedGpus <= gpusPerNode {
		// Can fit onto a single node
		numReplicas = 1
		nodeGpuRequest = requestedGpus
	} else {
		// Cannot fit onto a single node
		for nodeGpuRequest = gpusPerNode; nodeGpuRequest > 0; nodeGpuRequest-- {
			// If we can cleanly divide the number of GPUs
			if requestedGpus%nodeGpuRequest == 0 {
				numReplicas = requestedGpus / nodeGpuRequest
				break
			}
		}
	}

	maxGpusPerNode := math.Min(float64(gpusPerNode), float64(requestedGpus))

	if (float64(nodeGpuRequest) / maxGpusPerNode) < 0.5 {
		logrus.Warnf("Inefficient GPU allocation:\n"+
			"  - %d GPUs per node\n"+
			"  - %d GPUs assigned per replica\n"+
			"  - %d replicas\n"+
			"Check that requested GPUs (%d) is divisible by GPUs per node (%d).",
			gpusPerNode, nodeGpuRequest, numReplicas, requestedGpus, gpusPerNode)
	}

	return numReplicas, nodeGpuRequest
}
