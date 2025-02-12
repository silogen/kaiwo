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

package jobs

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const EntrypointFilename = "entrypoint"

type Job struct {
	workloads.WorkloadBase
	Job  batchv1.Job
	Pods []corev1.Pod
}

type JobFlags struct {
	Entrypoint string
}

func (j *Job) DefaultTemplate() ([]byte, error) {
	if JobTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return JobTemplate, nil
}

func (j *Job) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(execFlags.WorkloadFiles[EntrypointFilename])
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Warnln("No entrypoint file found. Expecting entrypoint in image")
			return JobFlags{Entrypoint: ""}, nil
		} else {
			return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
		}
	}

	entrypoint := string(contents)
	entrypoint = strings.ReplaceAll(entrypoint, "\n", " ")    // Flatten multiline string
	entrypoint = strings.ReplaceAll(entrypoint, "\"", "\\\"") // Escape double quotes
	entrypoint = fmt.Sprintf("\"%s\"", entrypoint)            // Wrap the entire command in quotes

	return JobFlags{Entrypoint: entrypoint}, nil
}

func (j *Job) GetObject() client.Object {
	return &j.Job
}

func (j *Job) SetFromObject(obj client.Object) error {
	converted, ok := obj.(*batchv1.Job)
	if !ok {
		return fmt.Errorf("expected Job, got %T", obj)
	}
	j.Job = *converted
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

func (j *Job) ResolveStructure(ctx context.Context, k8sClient client.Client) error {
	logrus.Debugf("loading job reference %s", j.Job.Name)

	pods := &corev1.PodList{}

	controllerUID, exists := j.Job.Spec.Template.Labels["batch.kubernetes.io/controller-uid"]

	if !exists {
		return fmt.Errorf("controller-uid label is missing in the Job's pod template")
	}

	logrus.Infof("Using controller uid: %s", controllerUID)

	labelSelector := client.MatchingLabels{
		"batch.kubernetes.io/controller-uid": controllerUID,
		"controller-uid":                     controllerUID,
		"batch.kubernetes.io/job-name":       j.Job.GetName(),
		"job-name":                           j.Job.GetName(),
	}
	if err := k8sClient.List(ctx, pods, client.InNamespace(j.Job.Namespace), labelSelector); err != nil {
		return fmt.Errorf("could not list pods: %w", err)
	}

	logrus.Debugf("Found %d pods", len(pods.Items))

	// Clear existing pods and append the new ones
	j.Pods = nil
	j.Pods = append(j.Pods, pods.Items...)

	return nil
}

func (j *Job) ListKnownPods() []workloads.WorkloadPod {
	workloadPods := make([]workloads.WorkloadPod, len(j.Pods))
	for i, pod := range j.Pods {
		workloadPods[i] = workloads.WorkloadPod{
			Pod: pod,
		}
	}
	return workloadPods
}

func (j *Job) GetStatus() string {
	return "N/A (TODO)"
}
