// Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
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
	"path/filepath"

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

type Job struct{}

type JobFlags struct {
	Entrypoint string
}

func (job Job) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(filepath.Join(execFlags.Path, EntrypointFilename))
	if err != nil {
		return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	return JobFlags{Entrypoint: string(contents)}, nil
}

func (job Job) DefaultTemplate() ([]byte, error) {
	if JobTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return JobTemplate, nil
}

func (job Job) ConvertObject(object runtime.Object) (runtime.Object, bool) {
	obj, ok := object.(*rayv1.RayJob)
	return obj, ok
}

//func (job Job) GenerateName() string {
//	return utils.BuildWorkloadName(job.Shared.Name, job.Shared.Path, job.Job.Image)
//}

func (job Job) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename, workloads.TemplateFileName}
}

func (job Job) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (job Job) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (job Job) GenerateAdditionalResourceManifests(k8sClient client.Client, templateContext workloads.WorkloadTemplateConfig) ([]runtime.Object, error) {
	localClusterQueueManifest, err := workloads.CreateLocalClusterQueueManifest(k8sClient, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create local cluster queue manifest: %w", err)
	}

	return []runtime.Object{localClusterQueueManifest}, nil
}

func (job Job) BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (workloads.WorkloadReference, error) {
	obj := &rayv1.RayJob{}

	if err := k8sClient.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("could not get job: %w", err)
	}
	jobRef := &JobReference{
		RayJob: *obj,
		WorkloadReferenceBase: workloads.WorkloadReferenceBase{
			WorkloadObject: obj,
		},
	}
	if err := jobRef.Load(ctx, k8sClient); err != nil {
		return nil, fmt.Errorf("could not load job: %w", err)
	}
	return jobRef, nil
}

type JobReference struct {
	workloads.WorkloadReferenceBase
	RayJob       rayv1.RayJob
	SubmitterPod *corev1.Pod
	HeadPod      *corev1.Pod
	WorkerPods   []*corev1.Pod
}

func (jobRef *JobReference) Load(ctx context.Context, k8sClient client.Client) error {
	// Find cluster pods
	clusterPodLabelSelector := client.MatchingLabels{
		"ray.io/cluster": jobRef.RayJob.Status.RayClusterName,
	}

	clusterPodList := &corev1.PodList{}
	if err := k8sClient.List(ctx, clusterPodList, clusterPodLabelSelector); err != nil {
		return fmt.Errorf("could not list cluster pods: %w", err)
	}

	// Clear
	jobRef.HeadPod = nil
	jobRef.WorkerPods = []*corev1.Pod{}

	for _, pod := range clusterPodList.Items {
		nodeType := pod.Labels["ray.io/node-type"]
		if nodeType == "worker" {
			jobRef.WorkerPods = append(jobRef.WorkerPods, &pod)
		} else if nodeType == "head" {
			if jobRef.HeadPod == nil {
				jobRef.HeadPod = &pod
			} else {
				logrus.Warn("More than one head pod encountered")
			}
		} else {
			logrus.Warnf("Encountered unknown node type: %s", nodeType)
		}
	}

	// Find submitter pod
	submitterPodLabelSelector := client.MatchingLabels{
		"batch.kubernetes.io/job-name": jobRef.RayJob.GetName(),
		"job-name":                     jobRef.RayJob.GetName(),
	}

	submitterPodList := &corev1.PodList{}
	if err := k8sClient.List(ctx, submitterPodList, submitterPodLabelSelector); err != nil {
		return fmt.Errorf("could not list submitter pods: %w", err)
	}

	switch len(submitterPodList.Items) {
	case 0:
		logrus.Warn("No submitter pods found")
	case 1:
		jobRef.SubmitterPod = &submitterPodList.Items[0]
	default:
		logrus.Warn("More than one submitter pods found")
	}

	return nil
}

func (jobRef *JobReference) GetPods() []workloads.WorkloadPod {
	var pods []workloads.WorkloadPod

	if jobRef.SubmitterPod != nil {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *jobRef.SubmitterPod,
			LogicalGroup: "submitter",
		})
	}

	if jobRef.HeadPod != nil {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *jobRef.HeadPod,
			LogicalGroup: "head",
		})
	}

	for _, pod := range jobRef.WorkerPods {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *pod,
			LogicalGroup: "worker",
		})
	}
	return pods
}

func (jobRef *JobReference) GetStatus() string {
	return "N/A (TODO)"
}
