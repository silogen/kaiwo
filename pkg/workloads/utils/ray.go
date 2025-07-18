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

	commonSpec := workload.GetCommonSpec()
	workerOptions := workloadutils.WithBaseOptions(workload)
	workerOptions = append(workerOptions, workloadutils.WithGpuSchedulingOptions(config, commonSpec.Resources, gpuSchedulingResult)...)
	workerOptions = append(workerOptions, workloadutils.WithImage(config.Ray.DefaultRayImage))

	// Update worker group specs
	for i := range rayClusterSpec.WorkerGroupSpecs {
		workloadutils.UpdatePodTemplateSpec(&rayClusterSpec.WorkerGroupSpecs[i].Template, workerOptions...)
		rayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
	}

	headOptions := workloadutils.WithBaseOptions(workload)
	headOptions = append(headOptions, workloadutils.WithGpuEnvVars(gpuSchedulingResult))
	headOptions = append(headOptions, workloadutils.WithResourceRequirements(commonSpec.Resources))
	headOptions = append(headOptions,
		workloadutils.WithRayHeadPodResourceRequirements(resource.MustParse(config.Ray.HeadPodMemory)),
		workloadutils.WithImage(config.Ray.DefaultRayImage),
	)

	// Update head group spec
	workloadutils.UpdatePodTemplateSpec(&rayClusterSpec.HeadGroupSpec.Template, headOptions...)

	return nil
}
