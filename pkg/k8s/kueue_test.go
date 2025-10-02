/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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
