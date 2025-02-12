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
	_ "embed"
	"fmt"
	"os"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const EntrypointFilename = "entrypoint"

type Job struct {
	workloads.WorkloadBase
	RayJob       rayv1.RayJob
	SubmitterPod *corev1.Pod
	HeadPod      *corev1.Pod
	WorkerPods   []*corev1.Pod
}

type JobFlags struct {
	Entrypoint string
}

func (j *Job) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(execFlags.WorkloadFiles[EntrypointFilename])
	if err != nil {
		return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	return JobFlags{Entrypoint: string(contents)}, nil
}

func (j *Job) DefaultTemplate() ([]byte, error) {
	if JobTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return JobTemplate, nil
}

func (j *Job) GetObject() client.Object {
	return &j.RayJob
}

func (j *Job) ConvertObject(object runtime.Object) (client.Object, bool) {
	obj, ok := object.(*rayv1.RayJob)
	return obj, ok
}

func (j *Job) SetFromObject(obj client.Object) error {
	converted, ok := obj.(*rayv1.RayJob)
	if !ok {
		return fmt.Errorf("expected Deployment, got %T", obj)
	}
	j.RayJob = *converted
	return nil
}

func (j *Job) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename, workloads.TemplateFileName}
}

func (j *Job) GenerateAdditionalResourceManifests(ctx context.Context, k8sClient client.Client, templateContext workloads.WorkloadTemplateConfig) ([]client.Object, error) {
	localClusterQueueManifest, err := workloads.CreateLocalClusterQueueManifest(ctx, k8sClient, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create local cluster queue manifest: %w", err)
	}

	return []client.Object{localClusterQueueManifest}, nil
}

func (j *Job) GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error) {
	serviceLabelSelector := client.MatchingLabels{
		"ray.io/cluster": j.RayJob.Status.RayClusterName,
	}
	serviceList := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, serviceList, serviceLabelSelector); err != nil {
		return nil, fmt.Errorf("could not list services: %w", err)
	}
	return serviceList.Items, nil
}

func (j *Job) ResolveStructure(ctx context.Context, k8sClient client.Client) error {
	// Find cluster pods
	clusterPodLabelSelector := client.MatchingLabels{
		"ray.io/cluster": j.RayJob.Status.RayClusterName,
	}

	clusterPodList := &corev1.PodList{}
	if err := k8sClient.List(ctx, clusterPodList, clusterPodLabelSelector); err != nil {
		return fmt.Errorf("could not list cluster pods: %w", err)
	}

	// Clear
	j.HeadPod = nil
	j.WorkerPods = []*corev1.Pod{}

	for _, pod := range clusterPodList.Items {
		nodeType := pod.Labels["ray.io/node-type"]
		if nodeType == "worker" {
			j.WorkerPods = append(j.WorkerPods, &pod)
		} else if nodeType == "head" {
			if j.HeadPod == nil {
				j.HeadPod = &pod
			} else {
				logrus.Warn("More than one head pod encountered")
			}
		} else {
			logrus.Warnf("Encountered unknown node type: %s", nodeType)
		}
	}

	// Find submitter pod
	submitterPodLabelSelector := client.MatchingLabels{
		"batch.kubernetes.io/job-name": j.RayJob.GetName(),
		"job-name":                     j.RayJob.GetName(),
	}

	submitterPodList := &corev1.PodList{}
	if err := k8sClient.List(ctx, submitterPodList, submitterPodLabelSelector); err != nil {
		return fmt.Errorf("could not list submitter pods: %w", err)
	}

	switch len(submitterPodList.Items) {
	case 0:
		logrus.Warn("No submitter pods found")
	case 1:
		j.SubmitterPod = &submitterPodList.Items[0]
	default:
		logrus.Warn("More than one submitter pods found")
	}

	return nil
}

func (j *Job) ListKnownPods() []workloads.WorkloadPod {
	var pods []workloads.WorkloadPod

	if j.SubmitterPod != nil {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *j.SubmitterPod,
			LogicalGroup: "submitter",
		})
	}

	if j.HeadPod != nil {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *j.HeadPod,
			LogicalGroup: "head",
		})
	}

	for _, pod := range j.WorkerPods {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *pod,
			LogicalGroup: "worker",
		})
	}
	return pods
}

func (j *Job) GetStatus() string {
	return "N/A (TODO)"
}
