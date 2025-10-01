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

package utils

import (
	"context"

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
func UpdateRayClusterSpec(ctx context.Context, clusterCtx workloadutils.ClusterContext, workload workloadutils.KaiwoWorkload, rayClusterSpec *rayv1.RayClusterSpec) {
	config := workloadutils.ConfigFromContext(ctx)

	// Calculate scheduling config for workers
	resourceConfig := workloadutils.CalculateResourceConfig(ctx, clusterCtx, workload, true)

	// Update worker group specs
	for i := range rayClusterSpec.WorkerGroupSpecs {
		workloadutils.UpdatePodSpec(config, workload, resourceConfig, &rayClusterSpec.WorkerGroupSpecs[i].Template, false)
		rayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(resourceConfig.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(resourceConfig.Replicas))
		rayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(resourceConfig.Replicas))
	}

	// Update scheduling config for head group spec
	if headMemoryOverride := resource.MustParse(config.Ray.HeadPodMemory); headMemoryOverride.Value() > 0 {
		if resourceConfig.DefaultResources == nil {
			resourceConfig.DefaultResources = &v1.ResourceRequirements{
				Limits:   v1.ResourceList{},
				Requests: v1.ResourceList{},
			}
		}
		resourceConfig.DefaultResources.Limits[v1.ResourceMemory] = headMemoryOverride
		resourceConfig.DefaultResources.Requests[v1.ResourceMemory] = headMemoryOverride
	}

	// Update head group spec
	workloadutils.UpdatePodSpec(config, workload, resourceConfig, &rayClusterSpec.HeadGroupSpec.Template, true)

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
}
