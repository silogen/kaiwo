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

package common

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

const (
	AmdGpuResourceName    = corev1.ResourceName("amd.com/gpu")
	NvidiaGpuResourceName = corev1.ResourceName("nvidia.com/gpu")
)

var (
	DefaultMemory = resource.MustParse("16Gi")
	DefaultCPU    = resource.MustParse("2")
)

func VendorToResourceName(vendor v1alpha1.GpuVendor) corev1.ResourceName {
	switch vendor {
	case v1alpha1.GpuVendorAmd:
		return AmdGpuResourceName
	case v1alpha1.GpuVendorNvidia:
		return NvidiaGpuResourceName
	}
	panic(fmt.Sprintf("unknown vendor: %v", vendor))
}

var (
	ErrMissingGpuFields         = errors.New("missing gpu fields `count`, `totalVram` or `totalComputeUnits`")
	ErrReplicasCannotFit        = errors.New("required replicas cannot fit into cluster")
	ErrInvalidGpuResourceValues = errors.New("invalid gpu resource values")
)

//// CalculateResourceConfig calculates the workload scheduling config based on the requested GPUs and available cluster resources
//func CalculateResourceConfig(
//	ctx context.Context,
//	clusterCtx ClusterContext,
//	resourceRequirements v1alpha1.GpuResourceRequirements,
//	replicas *int,
//) (*CalculatedResourceConfig, error) {
//	if resourceRequirements.Count == nil && resourceRequirements.TotalComputeUnits == nil && resourceRequirements.TotalVram == nil {
//		return nil, ErrMissingGpuFields
//	}
//	matchingNodes, err := clusterCtx.FindMatchingNodes(resourceRequirements)
//	if err != nil {
//		return nil, fmt.Errorf("failed to find best matching node: %w", err)
//	}
//
//	if resourceRequirements.TotalVram != nil && resourceRequirements.TotalVram.Value() == 0 {
//		return nil, fmt.Errorf("%w: totalVram cannot be zero", ErrInvalidGpuResourceValues)
//	}
//	if resourceRequirements.TotalComputeUnits != nil && resourceRequirements.TotalComputeUnits.Value() == 0 {
//		return nil, fmt.Errorf("%w: totalComputeUnits cannot be zero", ErrInvalidGpuResourceValues)
//	}
//
//	bestMatchingNode := matchingNodes[0]
//
//	calculatedResourceRequirements := v1alpha1.GpuResourceRequirements{
//		Flavor:            resourceRequirements.Flavor,
//		Models:            resourceRequirements.Models,
//		Vendor:            resourceRequirements.Vendor,
//		Count:             resourceRequirements.Count,
//		TotalVram:         resourceRequirements.TotalVram,
//		TotalComputeUnits: resourceRequirements.TotalComputeUnits,
//	}
//
//	// If GPU count is not explicitly given, calculate it based on other parameters
//	if baseutils.ValueOrDefault(resourceRequirements.Count) == 0 {
//		gpuCount := 0
//
//		if resourceRequirements.TotalVram != nil {
//			gpuCount = int(math.Ceil(float64(resourceRequirements.TotalVram.Value()) / float64(bestMatchingNode.GpuInfo.LogicalComputeUnits.Value())))
//		}
//		if resourceRequirements.TotalComputeUnits != nil {
//			gpuCount = max(gpuCount, int(math.Ceil(float64(resourceRequirements.TotalComputeUnits.Value())/float64(bestMatchingNode.GpuInfo.LogicalComputeUnits.Value()))))
//		}
//
//		calculatedResourceRequirements.Count = &gpuCount
//	}
//
//	// If replicas is set, ensure cluster can fit the requested number of replicas
//	if replicas != nil {
//		possibleReplicas := 0
//		for _, node := range matchingNodes {
//			replicasOnNode := math.Floor(float64(node.GpuInfo.LogicalCount.Value()) / float64(*calculatedResourceRequirements.Count))
//			possibleReplicas += int(replicasOnNode)
//		}
//		if possibleReplicas < *replicas {
//			return nil, ErrReplicasCannotFit
//		}
//	}
//
//	return &CalculatedResourceConfig{
//		CalculatedResourceRequirements: calculatedResourceRequirements,
//		BestMatchedNode:                bestMatchingNode.NodeResourceInfo,
//	}, nil
//}
//
//func GetGpuResourceRequest(workload KaiwoWorkload) (corev1.ResourceName, *resource.Quantity) {
//	spec := workload.GetCommonSpec()
//	if spec.Resources.Requests == nil {
//		return "", nil
//	}
//	if val, exists := spec.Resources.Requests[AmdGpuResourceName]; exists {
//		return AmdGpuResourceName, &val
//	}
//	if val, exists := spec.Resources.Requests[NvidiaGpuResourceName]; exists {
//		return NvidiaGpuResourceName, &val
//	}
//	return "", nil
//}
