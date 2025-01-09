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
