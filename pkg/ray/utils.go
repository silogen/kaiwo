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

package ray

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/api"

	workloadutils "github.com/silogen/kaiwo/pkg/config"

	"github.com/silogen/kaiwo/pkg/podspec"

	"github.com/silogen/kaiwo/pkg/cluster"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetRayClusterTemplate(config workloadutils.KaiwoConfigContext, dangerous bool) *rayv1.RayClusterSpec {
	return &rayv1.RayClusterSpec{
		EnableInTreeAutoscaling: baseutils.Pointer(false),
		HeadGroupSpec: rayv1.HeadGroupSpec{
			RayStartParams: map[string]string{
				"dashboard-host": "0.0.0.0",
			},
			Template: podspec.GetPodTemplate(config, resource.MustParse("1Gi"), dangerous, "ray-head"),
		},
		WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
			{
				GroupName:      "default-worker-group",
				Replicas:       baseutils.Pointer(int32(1)),
				MinReplicas:    baseutils.Pointer(int32(1)),
				MaxReplicas:    baseutils.Pointer(int32(1)),
				RayStartParams: map[string]string{},
				Template:       podspec.GetPodTemplate(config, resource.MustParse("200Gi"), dangerous, "ray-worker"), // TODO: add to CRD as configurable field
			},
		},
	}
}

// UpdateRayClusterSpec updates a given Ray workload spec to match the Kaiwo workload inputs
func UpdateRayClusterSpec(ctx context.Context, clusterCtx api.ClusterContext, workload api.KaiwoWorkload, rayClusterSpec *rayv1.RayClusterSpec) error {
	config := workloadutils.ConfigFromContext(ctx)

	// Calculate scheduling config for workers
	spec := workload.GetCommonSpec()
	gpuSchedulingResult, err := cluster.CalculateGpuRequirements(ctx, clusterCtx, spec.GpuResources, spec.Replicas)
	if err != nil {
		return fmt.Errorf("failed calculating gpu requirements: %w", err)
	}

	commonSpec := workload.GetCommonSpec()
	workerOptions := podspec.WithBaseOptions(workload)
	workerOptions = append(workerOptions, podspec.WithGpuSchedulingOptions(config, commonSpec.Resources, gpuSchedulingResult)...)
	workerOptions = append(workerOptions, podspec.WithImage(config.Ray.DefaultRayImage))

	// Update worker group specs
	for i := range rayClusterSpec.WorkerGroupSpecs {
		podspec.UpdatePodTemplateSpec(&rayClusterSpec.WorkerGroupSpecs[i].Template, workerOptions...)
		rayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
	}

	headOptions := podspec.WithBaseOptions(workload)
	headOptions = append(headOptions, podspec.WithGpuEnvVars(gpuSchedulingResult))
	headOptions = append(headOptions, podspec.WithResourceRequirements(commonSpec.Resources))
	headOptions = append(headOptions,
		podspec.WithRayHeadPodResourceRequirements(resource.MustParse(config.Ray.HeadPodMemory)),
		podspec.WithImage(config.Ray.DefaultRayImage),
	)

	// Update head group spec
	podspec.UpdatePodTemplateSpec(&rayClusterSpec.HeadGroupSpec.Template, headOptions...)

	return nil
}
