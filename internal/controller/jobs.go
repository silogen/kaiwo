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

package controller

import (
	"context"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var DefaultJobSpec = batchv1.JobSpec{
	TTLSecondsAfterFinished: func(i int32) *int32 { return &i }(3600),
	Template:                controllerutils.DefaultPodTemplateSpec,
}

func (r *KaiwoJobReconciler) reconcileK8sJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var existingJob batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingJob)
	if err == nil {
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing Job")
		return ctrl.Result{}, err
	}

	var jobSpec batchv1.JobSpec

	if kaiwoJob.Spec.Job == nil {
		logger.Info("JobSpec is nil, using default JobSpec", "KaiwoJob", kaiwoJob.Name)
		jobSpec = DefaultJobSpec

	} else {
		jobSpec = kaiwoJob.Spec.Job.Spec
	}

	if jobSpec.Template.ObjectMeta.Labels == nil {
		jobSpec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	jobSpec.Template.ObjectMeta.Labels["job-name"] = kaiwoJob.Name

	if err := controllerutils.AddEntrypoint(ctx, kaiwoJob.Spec.EntryPoint, &jobSpec.Template); err != nil {
		return ctrl.Result{}, err
	}

	if kaiwoJob.Spec.Gpus == 0 {
		gpuResourceKey := corev1.ResourceName(controllerutils.GetGpuResourceKey(kaiwoJob.Spec.GpuVendor))
		if gpuQuantity, exists := jobSpec.Template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			kaiwoJob.Spec.Gpus = int(gpuQuantity.Value())
		}
	}

	if err := controllerutils.AdjustResourceRequestsAndLimits(ctx, kaiwoJob.Spec.GpuVendor, kaiwoJob.Spec.Gpus, 1, kaiwoJob.Spec.Gpus, &jobSpec.Template); err != nil {
		return ctrl.Result{}, err
	}

	if err := controllerutils.AddEnvVars(ctx, kaiwoJob.Spec.EnvVars, &jobSpec.Template); err != nil {
		return ctrl.Result{}, err
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: jobSpec,
	}

	if err := controllerutils.UpdatePodSpecStorage(ctx, &job.Spec.Template.Spec, kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
		return ctrl.Result{}, err
	}

	if err := ctrl.SetControllerReference(kaiwoJob, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, job); err != nil {
		logger.Error(err, "Failed to create Job")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created Kubernetes Job", "Job", job.Name)
	return ctrl.Result{}, nil
}
