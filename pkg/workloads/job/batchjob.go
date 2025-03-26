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

	ctrl "sigs.k8s.io/controller-runtime"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	workloadcommon "github.com/silogen/kaiwo/pkg/workloads/common"
)

const (
	defaultTTLSecondsAfterFinished = int32(3600)
)

func GetDefaultJobSpec(dangerous bool, resourceRequirements corev1.ResourceRequirements) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(defaultTTLSecondsAfterFinished),
		BackoffLimit:            baseutils.Pointer(int32(0)),
		Template:                workloadcommon.GetPodTemplate(*resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous, resourceRequirements, "workload"),
	}
}

type BatchJobReconciler struct {
	workloadcommon.ResourceReconcilerBase[*batchv1.Job]
	KaiwoJob *v1alpha1.KaiwoJob
}

func NewBatchJobReconciler(kaiwoJob *v1alpha1.KaiwoJob) *BatchJobReconciler {
	reconciler := &BatchJobReconciler{
		ResourceReconcilerBase: workloadcommon.ResourceReconcilerBase[*batchv1.Job]{
			ObjectKey: client.ObjectKeyFromObject(kaiwoJob),
		},
		KaiwoJob: kaiwoJob,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *BatchJobReconciler) Build(ctx context.Context, _ client.Client) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	spec := r.KaiwoJob.Spec
	labelContext := workloadcommon.GetKaiwoLabelContext(r.KaiwoJob)

	var jobSpec batchv1.JobSpec
	jobSpec.Template.ObjectMeta.Labels = r.KaiwoJob.ObjectMeta.Labels

	var overrideDefaults bool

	if spec.Job == nil {
		jobSpec = GetDefaultJobSpec(spec.Dangerous, baseutils.ValueOrDefault(spec.Resources))
		if r.KaiwoJob.Spec.CommonMetaSpec.Gpus > 0 {
			overrideDefaults = true
		}
		if r.KaiwoJob.Spec.CommonMetaSpec.Resources != nil {
			overrideDefaults = false
		}
	} else {
		jobSpec = spec.Job.Spec
		overrideDefaults = false
	}

	if err := workloadcommon.UpdatePodSpec(r.KaiwoJob.Spec.CommonMetaSpec, labelContext, &jobSpec.Template, r.KaiwoJob.Name, 1, r.KaiwoJob.Spec.CommonMetaSpec.Gpus, overrideDefaults); err != nil {
		return nil, fmt.Errorf("failed to update job spec: %w", err)
	}

	if baseutils.ValueOrDefault(jobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		jobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	if jobSpec.TTLSecondsAfterFinished == nil {
		jobSpec.TTLSecondsAfterFinished = baseutils.Pointer(defaultTTLSecondsAfterFinished)
	}

	if err := workloadcommon.AddEntrypoint(spec.EntryPoint, &jobSpec.Template); err != nil {
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

	workloadcommon.CopyLabels(r.KaiwoJob.ObjectMeta.Labels, &job.ObjectMeta)
	workloadcommon.SetKaiwoSystemLabels(labelContext, &job.ObjectMeta)

	job.ObjectMeta.Labels[workloadcommon.QueueLabel] = r.KaiwoJob.Labels[workloadcommon.QueueLabel]

	return job, nil
}

func (r *BatchJobReconciler) GetEmptyObject() *batchv1.Job {
	return &batchv1.Job{}
}

func (r *BatchJobReconciler) ValidateBeforeCreateOrUpdate(ctx context.Context, actual *batchv1.Job) (*ctrl.Result, error) {
	// Abort reconciliation the managed label is set and actual doesn't exist, as the job is managed by the webhook
	// This is to avoid trying to create the job that is going to be created once the webhook completes
	return workloadcommon.ValidateKaiwoResourceBeforeCreateOrUpdate(ctx, actual, r.KaiwoJob.ObjectMeta)
}
