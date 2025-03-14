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
	common "github.com/silogen/kaiwo/pkg/workloads/common"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

var (
	enforceKaiwoOnWorkloads = baseutils.GetEnv("ENFORCE_KAIWO_ON_GPU_WORKLOADS", "false")
	gpuKeys                 = []corev1.ResourceName{"amd.com/gpu", "nvidia.com/gpu"}
)

type JobWebhook struct {
	Client  client.Client
	decoder *admission.Decoder
}

// +kubebuilder:webhook:path=/mutate-batch-v1-job,mutating=true,failurePolicy=fail,sideEffects=None,groups=batch,resources=jobs,verbs=create,versions=v1,name=mjob.kb.io,admissionReviewVersions=v1
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

	logger := logf.FromContext(ctx)

	if job.DeletionTimestamp != nil {
		return nil
	}

	authenticatedUser := getAuthenticatedUser(ctx)

	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}

	if strings.ToLower(enforceKaiwoOnWorkloads) == "true" { //nolint:goconst
		for _, container := range job.Spec.Template.Spec.Containers {
			if CheckGPUReservation(container) {
				job.Labels[baseutils.KaiwoManagedLabel] = "true" //nolint:goconst
			}
		}
	}

	if kaiwoManages(job) {
		if job.Labels[kaiwov1alpha1.QueueLabel] == "" {
			job.Labels[kaiwov1alpha1.QueueLabel] = controllerutils.DefaultClusterQueueName
		}
		if job.Spec.Template.Spec.TerminationGracePeriodSeconds == nil {
			job.Spec.Template.Spec.TerminationGracePeriodSeconds = baseutils.Pointer(int64(0))
			baseutils.Debug(logger, "Set terminationGracePeriodSeconds to 0", "JobName", job.Name)
		}
		if err := j.ensureKaiwoJob(ctx, job, authenticatedUser); err != nil {
			return fmt.Errorf("failed to create KaiwoJob: %w", err)
		}
		if !baseutils.ContainsString(job.Finalizers, common.Finalizer) {
			job.Finalizers = append(job.Finalizers, common.Finalizer)
			logger.Info("Added finalizer to Job", "JobName", job.Name)
		}

	}

	return nil
}

func kaiwoManages(job *batchv1.Job) bool {
	return job.Labels[baseutils.KaiwoManagedLabel] == "true" //nolint:goconst
}

func getAuthenticatedUser(ctx context.Context) string {
	logger := logf.FromContext(ctx)
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		logger.Error(nil, "Failed to extract admission request from context")
		return "unknown-user"
	}
	return req.UserInfo.Username
}

func (j *JobWebhook) ensureKaiwoJob(ctx context.Context, job *batchv1.Job, authenticatedUser string) error {
	logger := logf.FromContext(ctx)
	logger.Info("Ensuring KaiwoJob exists for Job", "JobName", job.Name)

	kaiwoJob := &kaiwov1alpha1.KaiwoJob{}

	err := j.Client.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, kaiwoJob)
	if err == nil {
		logger.Info("KaiwoJob already exists", "KaiwoJob", kaiwoJob.Name)
		return nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Unexpected error retrieving KaiwoJob")
		return fmt.Errorf("unexpected error retrieving KaiwoJob: %w", err)
	}

	kaiwoJobLabels := make(map[string]string)
	for key, value := range job.Labels {
		kaiwoJobLabels[key] = value
	}

	if _, exists := kaiwoJobLabels[kaiwov1alpha1.QueueLabel]; !exists {
		kaiwoJobLabels[kaiwov1alpha1.QueueLabel] = controllerutils.DefaultClusterQueueName
	}

	kaiwoJob = &kaiwov1alpha1.KaiwoJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    kaiwoJobLabels,
		},
		Spec: kaiwov1alpha1.KaiwoJobSpec{
			Job: job,
			CommonMetaSpec: kaiwov1alpha1.CommonMetaSpec{
				User: baseutils.Pointer(authenticatedUser),
			},
		},
	}

	labelContext := baseutils.GetKaiwoLabelContext(kaiwoJob)
	// Set Kaiwo system labels on the Kaiwo job
	baseutils.SetKaiwoSystemLabels(labelContext, &kaiwoJob.ObjectMeta)
	// Set Kaiwo system labels on the original job
	baseutils.SetKaiwoSystemLabels(labelContext, &job.ObjectMeta)

	if err := controllerutils.CreateLocalQueue(ctx, j.Client, kaiwoJobLabels[kaiwov1alpha1.QueueLabel], kaiwoJob.Namespace); err != nil {
		return fmt.Errorf("failed to create local queue: %w", err)
	}

	if err := j.Client.Create(ctx, kaiwoJob); err != nil {
		logger.Error(err, "Failed to create KaiwoJob")
		return fmt.Errorf("failed to create KaiwoJob: %w", err)
	}

	logger.Info("Successfully created KaiwoJob", "KaiwoJob", kaiwoJob.Name, "Labels", kaiwoJob.Labels)
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
				if _, exists := job.Labels[baseutils.KaiwoManagedLabel]; !exists {
					return nil, fmt.Errorf("all jobs must have 'kaiwo.silogen.ai/managed' label")
				}
			}
		}
	}

	return nil, nil
}

func (j *JobWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (j *JobWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := logf.FromContext(ctx)

	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("expected a Job object but got %T", obj)
	}

	// **Ensure the job has a finalizer before deletion starts**
	if !baseutils.ContainsString(job.Finalizers, common.Finalizer) {
		return nil, nil
	}

	logger.Info("Removing finalizer from Job", "JobName", job.Name)

	// **Retry removing finalizer if there's a conflict**
	retryAttempts := 3
	for i := 0; i < retryAttempts; i++ {
		// **Reload the latest version of the Job**
		if err := j.Client.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, job); err != nil {
			return nil, fmt.Errorf("failed to refresh Job before removing finalizer: %w", err)
		}

		job.Finalizers = baseutils.RemoveString(job.Finalizers, common.Finalizer)
		if err := j.Client.Update(ctx, job); err != nil {
			if errors.IsConflict(err) {
				continue // Retry
			}
			logger.Error(err, "Failed to remove finalizer from Job", "JobName", job.Name)
			return nil, fmt.Errorf("failed to remove finalizer: %w", err)
		}

		logger.Info("Finalizer successfully removed, Job can now be deleted", "JobName", job.Name)
		break
	}

	kaiwoJob := &kaiwov1alpha1.KaiwoJob{}
	err := j.Client.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, kaiwoJob)
	if err == nil {
		logger.Info("Deleting associated KaiwoJob", "KaiwoJob", kaiwoJob.Name)

		// Remove finalizer from KaiwoJob
		if baseutils.ContainsString(kaiwoJob.Finalizers, common.Finalizer) {
			kaiwoJob.Finalizers = baseutils.RemoveString(kaiwoJob.Finalizers, common.Finalizer)
			if err := j.Client.Update(ctx, kaiwoJob); err != nil {
				return nil, fmt.Errorf("failed to remove finalizer from KaiwoJob: %w", err)
			}
		}

		if err := j.Client.Delete(ctx, kaiwoJob); err != nil {
			return nil, fmt.Errorf("failed to delete KaiwoJob: %w", err)
		}
		logger.Info("KaiwoJob successfully deleted", "KaiwoJob", kaiwoJob.Name)
	} else if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get KaiwoJob: %w", err)
	}

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
