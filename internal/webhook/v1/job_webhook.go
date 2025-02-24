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

package v1

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1 "k8s.io/api/core/v1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

var (
	enforceKaiwoOnWorkloads = baseutils.GetEnv("ENFORCE_KAIWO_ON_GPU_WORKLOADS", "true")
	joblog                  = logf.Log.WithName("job-webhook")
	gpuKeys                 = []corev1.ResourceName{"nvidia.com/gpu", "amd.com/gpu"}
)

type JobWebhook struct {
	Client  client.Client
	decoder *admission.Decoder
}

// +kubebuilder:webhook:path=/mutate-batch-v1-job,mutating=true,failurePolicy=fail,sideEffects=None,groups=batch,resources=jobs,verbs=create;update,versions=v1,name=mjob.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-batch-v1-job,validating=true,failurePolicy=fail,sideEffects=None,groups=batch,resources=jobs,verbs=create;update;delete,versions=v1,name=vjob.kb.io,admissionReviewVersions=v1

var (
	_ admission.CustomDefaulter = &JobWebhook{}
	_ admission.CustomValidator = &JobWebhook{}
)

func (j *JobWebhook) Default(ctx context.Context, obj runtime.Object) error {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return fmt.Errorf("expected a Job object but got %T", obj)
	}

	authenticatedUser := getAuthenticatedUser(ctx)

	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}

	if strings.ToLower(enforceKaiwoOnWorkloads) == "true" { //nolint:goconst
		for _, container := range job.Spec.Template.Spec.Containers {
			if CheckGPUReservation(container) {
				job.Labels["kaiwo.silogen.ai/managed"] = "true" //nolint:goconst
			}
		}
	}

	if kaiwoManages(job) {
		job.Labels[kaiwov1alpha1.QueueLabel] = controllerutils.DefaultKaiwoQueueConfigName
		if job.Spec.Template.Spec.TerminationGracePeriodSeconds == nil {
			job.Spec.Template.Spec.TerminationGracePeriodSeconds = baseutils.Pointer(int64(0))
		}
		joblog.Info("Set terminationGracePeriodSeconds to 0", "JobName", job.Name)
		if err := j.ensureKaiwoJob(ctx, job, authenticatedUser); err != nil {
			return fmt.Errorf("failed to create KaiwoJob: %w", err)
		}
	}

	return nil
}

func kaiwoManages(job *batchv1.Job) bool {
	return job.Labels["kaiwo.silogen.ai/managed"] == "true" //nolint:goconst
}

func getAuthenticatedUser(ctx context.Context) string {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		joblog.Error(nil, "Failed to extract admission request from context")
		return "unknown-user"
	}
	return req.UserInfo.Username
}

func (j *JobWebhook) ensureKaiwoJob(ctx context.Context, job *batchv1.Job, authenticatedUser string) error {
	kaiwoJob := &kaiwov1alpha1.KaiwoJob{}

	err := j.Client.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, kaiwoJob)
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		joblog.Error(err, "Unexpected error retrieving KaiwoJob")
		return fmt.Errorf("unexpected error retrieving KaiwoJob: %w", err)
	}

	kaiwoJob = &kaiwov1alpha1.KaiwoJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    make(map[string]string),
		},
		Spec: kaiwov1alpha1.KaiwoJobSpec{
			Job:  job,
			User: baseutils.Pointer(authenticatedUser),
		},
	}

	kaiwoJob.Labels[kaiwov1alpha1.QueueLabel] = controllerutils.DefaultKaiwoQueueConfigName

	if err := controllerutils.CreateLocalQueue(ctx, j.Client, kaiwoJob.Labels[kaiwov1alpha1.QueueLabel], kaiwoJob.ObjectMeta.Namespace); err != nil {
		return fmt.Errorf("failed to create local queue: %w", err)
	}

	// Create the KaiwoJob in the cluster
	if err := j.Client.Create(ctx, kaiwoJob); err != nil {
		joblog.Error(err, "Failed to create KaiwoJob")
		return fmt.Errorf("failed to create KaiwoJob: %w", err)
	}

	joblog.Info("Successfully created KaiwoJob", "KaiwoJob", kaiwoJob.Name)
	return nil
}

func CheckGPUReservation(container corev1.Container) bool {
	for _, gpuKey := range gpuKeys {
		if _, exists := container.Resources.Limits[gpuKey]; exists {
			return true
		}
		if _, exists := container.Resources.Requests[gpuKey]; exists {
			return true
		}
	}
	return false
}

func (j *JobWebhook) InjectDecoder(d *admission.Decoder) error {
	j.decoder = d
	return nil
}

func (j *JobWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("expected a Job object but got %T", obj)
	}

	if strings.ToLower(enforceKaiwoOnWorkloads) == "true" {
		for _, container := range job.Spec.Template.Spec.Containers {
			if CheckGPUReservation(container) {
				if _, exists := job.Labels["kaiwo.silogen.ai/managed"]; !exists {
					return nil, fmt.Errorf("all jobs must have 'kaiwo.silogen.ai/managed' label")
				}
			}
		}
	}

	joblog.Info("Job validation passed", "JobName", job.Name)
	return nil, nil
}

func (j *JobWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newJob, ok := newObj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("expected a Job object but got %T", newObj)
	}
	oldJob, ok := oldObj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("expected a Job object but got %T", oldObj)
	}

	// Allow modification only if the update is from Kueue
	req, err := admission.RequestFromContext(ctx)
	if err == nil {
		if strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:kueue-") {
			return nil, nil
		}
	}

	if len(newJob.Spec.Template.Spec.Containers) != len(oldJob.Spec.Template.Spec.Containers) {
		return nil, fmt.Errorf("changing the number of containers is not allowed")
	}
	// Prevent increasing GPU requests/limits
	for i, newContainer := range newJob.Spec.Template.Spec.Containers {
		oldContainer := oldJob.Spec.Template.Spec.Containers[i]

		for _, gpuKey := range gpuKeys {
			if newLimit, newExists := newContainer.Resources.Limits[gpuKey]; newExists {
				if oldLimit, oldExists := oldContainer.Resources.Limits[gpuKey]; oldExists {
					if newLimit.Cmp(oldLimit) != 0 {
						return nil, fmt.Errorf("increasing/decreasing GPU limits is not allowed")
					}
				}
			}

			if newRequest, newExists := newContainer.Resources.Requests[gpuKey]; newExists {
				if oldRequest, oldExists := oldContainer.Resources.Requests[gpuKey]; oldExists {
					if newRequest.Cmp(oldRequest) != 0 {
						return nil, fmt.Errorf("increasing/decreasing GPU requests is not allowed")
					}
				}
			}
		}
	}

	joblog.Info("Job update validation passed", "JobName", newJob.Name)
	return nil, nil
}

func (j *JobWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (j *JobWebhook) SetupJobWebhookWithManager(mgr ctrl.Manager) error {
	j.Client = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(&batchv1.Job{}).
		WithDefaulter(j).
		WithValidator(j).
		Complete()
}
