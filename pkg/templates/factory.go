package templates

import (
	_ "embed"
	"fmt"

	"github.com/silogen/ai-workload-orchestrator/pkg/templates/ray"
)

const RayServiceType = "rayservice"
const RayJobType = "rayjob"

var WorkloadTypes = []string{RayServiceType, RayJobType}

func GetWorkloadLoader(workloadType string) (WorkloadLoader, error) {
	switch workloadType {
	case RayServiceType:
		return &ray.RayServiceLoader{}, nil
	case RayJobType:
		return &ray.RayJobLoader{}, nil
	default:
		return nil, fmt.Errorf("invalid workload type %s. Must be one of %v", workloadType, WorkloadTypes)
	}
}
