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

func GetDefaultJobSpec(dangerous bool, resourceRequirements corev1.ResourceRequirements) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(int32(3600)),
		Template:                workloadcommon.GetPodTemplate(*resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous, resourceRequirements),
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
	labelContext := r.KaiwoJob.GetLabelContext()

	var jobSpec batchv1.JobSpec
	jobSpec.Template.ObjectMeta.Labels = r.KaiwoJob.ObjectMeta.Labels

	if spec.Job == nil {
		jobSpec = GetDefaultJobSpec(baseutils.ValueOrDefault(spec.Dangerous), baseutils.ValueOrDefault(spec.Resources))
	} else {
		jobSpec = spec.Job.Spec
	}

	if err := workloadcommon.UpdatePodSpec(r.KaiwoJob.Spec.CommonMetaSpec, r.KaiwoJob.GetLabelContext(), &jobSpec.Template, r.KaiwoJob.Name, true); err != nil {
		return nil, fmt.Errorf("failed to update job spec: %w", err)
	}

	if baseutils.ValueOrDefault(jobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		jobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	if err := workloadcommon.AddEntrypoint(baseutils.ValueOrDefault(spec.EntryPoint), &jobSpec.Template); err != nil {
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

	// Update the job-level labels
	baseutils.CopyLabels(r.KaiwoJob.ObjectMeta.Labels, &job.ObjectMeta)
	baseutils.SetKaiwoSystemLabels(labelContext, &job.ObjectMeta)

	return job, nil
}

func (r *BatchJobReconciler) GetEmptyObject() *batchv1.Job {
	return &batchv1.Job{}
}
