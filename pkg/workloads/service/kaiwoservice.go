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

package workloadservice

import (
	"k8s.io/apimachinery/pkg/runtime"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

func NewKaiwoServiceHandler(scheme *runtime.Scheme, kaiwoService *kaiwo.KaiwoService) common.WorkloadHandler {
	if kaiwoService.Spec.IsRayService() {
		return common.WorkloadHandler{Workload: &RayServiceHandler{Scheme: scheme, KaiwoService: kaiwoService}}
	}
	return common.WorkloadHandler{Workload: &DeploymentHandler{Scheme: scheme, KaiwoService: kaiwoService}}
}
