/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package templates

import (
	_ "embed"

	"github.com/silogen/ai-workload-orchestrator/pkg/templates/jobs"
	"github.com/silogen/ai-workload-orchestrator/pkg/templates/ray"
	
)

const RayServiceType = "rayservice"
const RayJobType = "rayjob"
const JobType = "job"

var WorkloadTypes = []string{RayServiceType, RayJobType, JobType}

func GetWorkloadLoader(workloadType string) (WorkloadLoader) {
	switch workloadType {
	case RayServiceType:
		return &ray.RayServiceLoader{}
	case RayJobType:
		return &ray.RayJobLoader{}
	case JobType:
		return &jobs.JobLoader{}
	default:
		return &jobs.JobLoader{}
	}
}
