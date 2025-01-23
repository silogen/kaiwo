// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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
	"testing"
)

func TestCalculateNumberOfReplicas(t *testing.T) {
	tests := []struct {
		name                   string
		requestedGpus          int
		gpusPerNode            int
		expectedNumReplicas    int
		expectedNodeGpuRequest int
	}{
		{
			name:                   "Single node case",
			requestedGpus:          4,
			gpusPerNode:            8,
			expectedNumReplicas:    1,
			expectedNodeGpuRequest: 4,
		},
		{
			name:                   "Multiple nodes with perfect fit",
			requestedGpus:          16,
			gpusPerNode:            8,
			expectedNumReplicas:    2,
			expectedNodeGpuRequest: 8,
		},
		{
			name:                   "Multiple nodes with remainder",
			requestedGpus:          18,
			gpusPerNode:            8,
			expectedNumReplicas:    3,
			expectedNodeGpuRequest: 6,
		},
		{
			name:                   "Multiple nodes with poor fit",
			requestedGpus:          25,
			gpusPerNode:            4,
			expectedNumReplicas:    25,
			expectedNodeGpuRequest: 1,
		},
		{
			name:                   "No GPUs",
			requestedGpus:          0,
			gpusPerNode:            4,
			expectedNumReplicas:    1,
			expectedNodeGpuRequest: 0,
		},
		{
			name:                   "Negative GPUs",
			requestedGpus:          -1,
			gpusPerNode:            4,
			expectedNumReplicas:    0,
			expectedNodeGpuRequest: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numReplicas, nodeGpuRequest := CalculateNumberOfReplicas(tt.requestedGpus, tt.gpusPerNode, nil)

			if numReplicas != tt.expectedNumReplicas {
				t.Errorf("numReplicas: got %d, expected %d", numReplicas, tt.expectedNumReplicas)
			}
			if nodeGpuRequest != tt.expectedNodeGpuRequest {
				t.Errorf("nodeGpuRequest: got %d, expected %d", nodeGpuRequest, tt.expectedNodeGpuRequest)
			}
		})
	}
}
