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

package workloadjob

import (
	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/silogen/kaiwo/pkg/workloads/common"
)

func NewKaiwoJobHandler(scheme *runtime.Scheme, job *kaiwo.KaiwoJob) common.WorkloadHandler {
	if job.Spec.IsBatchJob() {
		return common.WorkloadHandler{Workload: &BatchJobHandler{KaiwoJob: job, Scheme: scheme}}
	}
	return common.WorkloadHandler{Workload: &RayJobHandler{KaiwoJob: job, Scheme: scheme}}
}
