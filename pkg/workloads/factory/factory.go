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

package factory

import (
	"context"
	"fmt"
	"strings"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/silogen/kaiwo/pkg/workloads/deployments"
	"github.com/silogen/kaiwo/pkg/workloads/jobs"
	"github.com/silogen/kaiwo/pkg/workloads/ray"
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

func ListObjects(ctx context.Context, k8sClient client.Client, workloadType string, opts ...client.ListOption) ([]workloads.WorkloadReference, error) {
	var workloadReferences []workloads.WorkloadReference

	// TODO refactor
	switch workloadType {
	case "rayjob":
		rayJobList := &rayv1.RayJobList{}
		if err := k8sClient.List(ctx, rayJobList, opts...); err != nil {
			return nil, err
		}
		for _, rayJob := range rayJobList.Items {
			workloadReferences = append(workloadReferences, &ray.JobReference{
				RayJob: rayJob,
				WorkloadReferenceBase: workloads.WorkloadReferenceBase{
					WorkloadObject: &rayJob,
				},
			})
		}
	case "rayservice":
		rayServiceList := &rayv1.RayServiceList{}
		if err := k8sClient.List(ctx, rayServiceList, opts...); err != nil {
			return nil, err
		}
		for _, rayService := range rayServiceList.Items {
			workloadReferences = append(workloadReferences, &ray.ServiceReference{
				RayService: rayService,
				WorkloadReferenceBase: workloads.WorkloadReferenceBase{
					WorkloadObject: &rayService,
				},
			})
		}
	case "job":
		jobList := &batchv1.JobList{}
		if err := k8sClient.List(ctx, jobList, opts...); err != nil {
			return nil, err
		}
		for _, job := range jobList.Items {
			workloadReferences = append(workloadReferences, &jobs.JobReference{
				Job: job,
				WorkloadReferenceBase: workloads.WorkloadReferenceBase{
					WorkloadObject: &job,
				},
			})
		}
	case "deployment":
		deploymentList := &appsv1.DeploymentList{}
		if err := k8sClient.List(ctx, deploymentList, opts...); err != nil {
			return nil, err
		}
		for _, deployment := range deploymentList.Items {
			workloadReferences = append(workloadReferences, &deployments.DeploymentReference{
				Deployment: deployment,
				WorkloadReferenceBase: workloads.WorkloadReferenceBase{
					WorkloadObject: &deployment,
				},
			})
		}
	default:
		return nil, fmt.Errorf("unknown workload type: %s", workloadType)
	}
	return workloadReferences, nil
}
