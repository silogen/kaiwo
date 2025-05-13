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

package workloadjob

import (
	"context"
	"fmt"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	ctrl "sigs.k8s.io/controller-runtime"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

const (
	defaultTTLSecondsAfterFinished = int32(3600)
)

func GetDefaultJobSpec(config controllerutils.KaiwoConfigContext, dangerous bool, resourceRequirements corev1.ResourceRequirements) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(defaultTTLSecondsAfterFinished),
		BackoffLimit:            baseutils.Pointer(int32(0)),
		Template:                workloadutils.GetPodTemplate(config, *resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous, resourceRequirements, "workload"),
	}
}

type BatchJobReconciler struct {
	common.ResourceReconcilerBase[*batchv1.Job]
	KaiwoJob *kaiwo.KaiwoJob
}

func NewBatchJobReconciler(kaiwoJob *kaiwo.KaiwoJob) *BatchJobReconciler {
	reconciler := &BatchJobReconciler{
		ResourceReconcilerBase: common.ResourceReconcilerBase[*batchv1.Job]{
			ObjectKey: client.ObjectKeyFromObject(kaiwoJob),
		},
		KaiwoJob: kaiwoJob,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *BatchJobReconciler) Build(ctx context.Context, _ client.Client) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	config := controllerutils.ConfigFromContext(ctx)

	spec := r.KaiwoJob.Spec
	labelContext := common.GetKaiwoLabelContext(r.KaiwoJob)

	var jobSpec batchv1.JobSpec
	jobSpec.Template.ObjectMeta.Labels = r.KaiwoJob.ObjectMeta.Labels

	var overrideDefaults bool

	if spec.Job == nil {
		jobSpec = GetDefaultJobSpec(config, spec.Dangerous, baseutils.ValueOrDefault(spec.Resources))
		if r.KaiwoJob.Spec.CommonMetaSpec.Gpus > 0 {
			overrideDefaults = true
		}
		if r.KaiwoJob.Spec.CommonMetaSpec.Resources != nil {
			overrideDefaults = false
		}
	} else {
		jobSpec = spec.Job.Spec
		overrideDefaults = false
		workloadutils.SyncGpuMetaFromPodSpec(jobSpec.Template.Spec, &r.KaiwoJob.Spec.CommonMetaSpec)
	}

	if err := workloadutils.UpdatePodSpec(config, r.KaiwoJob.Spec.CommonMetaSpec, labelContext, &jobSpec.Template, r.KaiwoJob.Name, 1, r.KaiwoJob.Spec.CommonMetaSpec.Gpus, overrideDefaults, false); err != nil {
		return nil, fmt.Errorf("failed to update job spec: %w", err)
	}

	if baseutils.ValueOrDefault(jobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		jobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	if jobSpec.TTLSecondsAfterFinished == nil {
		jobSpec.TTLSecondsAfterFinished = baseutils.Pointer(defaultTTLSecondsAfterFinished)
	}

	if err := workloadutils.AddEntrypoint(spec.EntryPoint, &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	jobSpec.Suspend = baseutils.Pointer(true)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    jobSpec.Template.ObjectMeta.Labels,
		},
		Spec: jobSpec,
	}

	common.CopyLabels(r.KaiwoJob.ObjectMeta.Labels, &job.ObjectMeta)
	common.SetKaiwoSystemLabels(labelContext, &job.ObjectMeta)

	job.ObjectMeta.Labels[kaiwo.QueueLabel] = r.KaiwoJob.Labels[kaiwo.QueueLabel]
	if r.KaiwoJob.Spec.PriorityClass != "" {
		job.Spec.Template.Spec.PriorityClassName = r.KaiwoJob.Spec.PriorityClass
	}

	return job, nil
}

func (r *BatchJobReconciler) GetEmptyObject() *batchv1.Job {
	return &batchv1.Job{}
}

func (r *BatchJobReconciler) ValidateBeforeCreateOrUpdate(ctx context.Context, actual *batchv1.Job) (*ctrl.Result, error) {
	// Abort reconciliation the managed label is set and actual doesn't exist, as the job is managed by the webhook
	// This is to avoid trying to create the job that is going to be created once the webhook completes
	return workloadutils.ValidateKaiwoResourceBeforeCreateOrUpdate(ctx, actual, r.KaiwoJob.ObjectMeta)
}
