package factory

import (
	"fmt"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/silogen/kaiwo/pkg/workloads/deployments"
	"github.com/silogen/kaiwo/pkg/workloads/jobs"
	"github.com/silogen/kaiwo/pkg/workloads/ray"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func GetWorkload(workloadType string) (workloads.Workload, error) {
	switch workloadType {
	case "rayjob":
		return ray.Job{}, nil
	case "rayservice":
		return ray.Deployment{}, nil
	case "job":
		return jobs.Job{}, nil
	case "deployment":
		return deployments.Deployment{}, nil
	default:
		return nil, fmt.Errorf("unknown workload type: %s", workloadType)
	}
}

func GetWorkloadAndObjectKey(workloadDescriptor string, namespace string) (workloads.Workload, client.ObjectKey, error) {

	key := client.ObjectKey{}

	parts := strings.Split(workloadDescriptor, "/")
	if len(parts) != 2 {
		return nil, key, fmt.Errorf("invalid workload descriptor: %s", workloadDescriptor)
	}
	workload, err := GetWorkload(parts[0])

	if err != nil {
		return nil, key, err
	}

	key.Namespace = namespace
	key.Name = parts[1]

	return workload, key, nil
}
