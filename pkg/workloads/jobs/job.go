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

package jobs

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/silogen/kaiwo/pkg/workloads"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"

	"github.com/sirupsen/logrus"
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

	if contents == nil {
		logrus.Warnln("No entrypoint file found. Expecting entrypoint in image")
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	entrypoint := string(contents)

	entrypoint = strings.ReplaceAll(entrypoint, "\n", " ")    // Flatten multiline string
	entrypoint = strings.ReplaceAll(entrypoint, "\"", "\\\"") // Escape double quotes
	entrypoint = fmt.Sprintf("\"%s\"", entrypoint)            // Wrap the entire command in quotes

	return JobFlags{Entrypoint: entrypoint}, nil

}

func (job Job) DefaultTemplate() ([]byte, error) {
	if JobTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return JobTemplate, nil
}

func (job Job) ConvertObject(object runtime.Object) (runtime.Object, bool) {
	obj, ok := object.(*batchv1.Job)
	return obj, ok
}

func (job Job) IgnoreFiles() []string {
	return []string{EntrypointFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename}
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

func (job Job) BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (workloads.WorkloadReference2, error) {
	obj := &batchv1.Job{}
	if err := k8sClient.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("could not get job: %w", err)
	}
	jobRef := &JobReference{
		Job: *obj,
	}
	return jobRef, nil
}

type JobReference struct {
	Job  batchv1.Job
	Pods []corev1.Pod
}

func (jobRef *JobReference) Load(ctx context.Context, k8sClient client.Client) error {
	logrus.Debugf("loading job reference %s", jobRef.Job.Name)

	pods := &corev1.PodList{}

	controllerUID, exists := jobRef.Job.Spec.Template.Labels["batch.kubernetes.io/controller-uid"]

	if !exists {
		return fmt.Errorf("controller-uid label is missing in the Job's pod template")
	}

	logrus.Infof("Using controller uid: %s", controllerUID)

	labelSelector := client.MatchingLabels{
		"batch.kubernetes.io/controller-uid": controllerUID,
		"controller-uid":                     controllerUID,
		"batch.kubernetes.io/job-name":       jobRef.Job.GetName(),
		"job-name":                           jobRef.Job.GetName(),
	}
	if err := k8sClient.List(ctx, pods, client.InNamespace(jobRef.Job.Namespace), labelSelector); err != nil {
		return fmt.Errorf("could not list pods: %w", err)
	}

	logrus.Debugf("Found %d pods", len(pods.Items))

	// Clear existing pods and append the new ones
	jobRef.Pods = nil
	for _, pod := range pods.Items {
		jobRef.Pods = append(jobRef.Pods, pod)
	}

	return nil
}

func (jobRef *JobReference) GetPods() []workloads.WorkloadPod {
	var workloadPods = make([]workloads.WorkloadPod, len(jobRef.Pods))
	for i, pod := range jobRef.Pods {
		workloadPods[i] = workloads.WorkloadPod{
			Pod: pod,
		}
	}
	return workloadPods
}
