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

package utils

import (
	"context"
	"fmt"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/common"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetRayClusterTemplate(config workloadutils.KaiwoConfigContext, dangerous bool) *rayv1.RayClusterSpec {
	return &rayv1.RayClusterSpec{
		EnableInTreeAutoscaling: baseutils.Pointer(false),
		HeadGroupSpec: rayv1.HeadGroupSpec{
			RayStartParams: map[string]string{
				"dashboard-host": "0.0.0.0",
			},
			Template: workloadutils.GetPodTemplate(config, resource.MustParse("1Gi"), dangerous, "ray-head"),
		},
		WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
			{
				GroupName:      "default-worker-group",
				Replicas:       baseutils.Pointer(int32(1)),
				MinReplicas:    baseutils.Pointer(int32(1)),
				MaxReplicas:    baseutils.Pointer(int32(1)),
				RayStartParams: map[string]string{},
				Template:       workloadutils.GetPodTemplate(config, resource.MustParse("200Gi"), dangerous, "ray-worker"), // TODO: add to CRD as configurable field
			},
		},
	}
}

// UpdateRayClusterSpec updates a given Ray workload spec to match the Kaiwo workload inputs
func UpdateRayClusterSpec(ctx context.Context, clusterCtx workloadutils.ClusterContext, workload workloadutils.KaiwoWorkload, rayClusterSpec *rayv1.RayClusterSpec) error {
	config := workloadutils.ConfigFromContext(ctx)

	// Calculate scheduling config for workers
	spec := workload.GetCommonSpec()
	gpuSchedulingResult, err := workloadutils.CalculateGpuRequirements(ctx, clusterCtx, spec.GpuResources, spec.Replicas)
	if err != nil {
		return fmt.Errorf("failed calculating gpu requirements: %w", err)
	}
	// Update worker group specs
	for i := range rayClusterSpec.WorkerGroupSpecs {
		workloadutils.UpdatePodSpec(config, workload, gpuSchedulingResult, &rayClusterSpec.WorkerGroupSpecs[i].Template)
		rayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
	}

	// Update head group spec
	workloadutils.UpdatePodSpec(config, workload, nil, &rayClusterSpec.HeadGroupSpec.Template)
	if headMemoryOverride := resource.MustParse(config.Ray.HeadPodMemory); headMemoryOverride.Value() > 0 {
		for i := range rayClusterSpec.HeadGroupSpec.Template.Spec.Containers {
			resources := rayClusterSpec.HeadGroupSpec.Template.Spec.Containers[i].Resources
			resources.Limits[v1.ResourceMemory] = headMemoryOverride
			resources.Requests[v1.ResourceMemory] = headMemoryOverride
			rayClusterSpec.HeadGroupSpec.Template.Spec.Containers[i].Resources = resources
		}
	}

	// Ensure image is set on all containers
	ensureImage := func(podSpec *v1.PodSpec) {
		for i := range podSpec.Containers {
			if podSpec.Containers[i].Image == "" {
				podSpec.Containers[i].Image = config.Ray.DefaultRayImage
			}
		}
	}

	ensureImage(&rayClusterSpec.HeadGroupSpec.Template.Spec)
	for i := range rayClusterSpec.WorkerGroupSpecs {
		ensureImage(&rayClusterSpec.WorkerGroupSpecs[i].Template.Spec)
	}
	return nil
}
