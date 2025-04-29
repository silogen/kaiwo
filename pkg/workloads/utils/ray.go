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

package workloadutils

import (
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetRayClusterTemplate(dangerous bool, resourceRequirements v1.ResourceRequirements) *rayv1.RayClusterSpec {
	return &rayv1.RayClusterSpec{
		EnableInTreeAutoscaling: baseutils.Pointer(false),
		HeadGroupSpec: rayv1.HeadGroupSpec{
			RayStartParams: map[string]string{
				"dashboard-host": "0.0.0.0",
			},
			Template: GetPodTemplate(resource.MustParse("1Gi"), dangerous, resourceRequirements, "ray-head"),
		},
		WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
			{
				GroupName:      "default-worker-group",
				Replicas:       baseutils.Pointer(int32(1)),
				MinReplicas:    baseutils.Pointer(int32(1)),
				MaxReplicas:    baseutils.Pointer(int32(1)),
				RayStartParams: map[string]string{},
				Template:       GetPodTemplate(resource.MustParse("200Gi"), dangerous, resourceRequirements, "ray-worker"), // TODO: add to CRD as configurable field
			},
		},
	}
}
