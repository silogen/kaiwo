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

	"k8s.io/apimachinery/pkg/runtime"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

const (
	defaultTTLSecondsAfterFinished = int32(3600)
)

type BatchJobHandler struct {
	KaiwoJob *kaiwo.KaiwoJob
	Scheme   *runtime.Scheme
}

func (handler *BatchJobHandler) GetInitializedObject() client.Object {
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoJob.Name,
			Namespace: handler.KaiwoJob.Namespace,
			Labels:    map[string]string{},
		},
	}
}

func (handler *BatchJobHandler) MutateActual(ctx context.Context, clusterCtx common.ClusterContext, actual client.Object) error {
	return nil
}

func (handler *BatchJobHandler) GetKaiwoWorkloadObject() client.Object {
	return handler.KaiwoJob
}

func (handler *BatchJobHandler) GetCommonSpec() kaiwo.CommonMetaSpec {
	return handler.KaiwoJob.Spec.CommonMetaSpec
}

func (handler *BatchJobHandler) GetCommonStatusSpec() *kaiwo.CommonStatusSpec {
	return &handler.KaiwoJob.Status.CommonStatusSpec
}

func (handler *BatchJobHandler) BuildDesired(ctx context.Context, clusterCtx common.ClusterContext) (client.Object, error) {
	logger := log.FromContext(ctx)
	config := common.ConfigFromContext(ctx)

	spec := handler.KaiwoJob.Spec
	labelContext := common.GetKaiwoLabelContext(handler.KaiwoJob)

	var jobSpec batchv1.JobSpec
	jobSpec.Template.ObjectMeta.Labels = handler.KaiwoJob.ObjectMeta.Labels

	commonSpec := handler.KaiwoJob.Spec.CommonMetaSpec

	var overrideDefaults bool

	if spec.Job == nil {
		jobSpec = GetDefaultJobSpec(config, spec.Dangerous, baseutils.ValueOrDefault(spec.Resources))
		if commonSpec.Gpus > 0 {
			overrideDefaults = true
		}
		if commonSpec.Resources != nil {
			overrideDefaults = false
		}
	} else {
		jobSpec = spec.Job.Spec
		overrideDefaults = false
		common.SyncGpuMetaFromPodSpec(jobSpec.Template.Spec, &commonSpec)
	}

	if err := common.UpdatePodSpec(config, commonSpec, labelContext, &jobSpec.Template, handler.KaiwoJob.Name, 1, commonSpec.Gpus, overrideDefaults, false); err != nil {
		return nil, fmt.Errorf("failed to update handler spec: %w", err)
	}

	if baseutils.ValueOrDefault(jobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		jobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	if jobSpec.TTLSecondsAfterFinished == nil {
		jobSpec.TTLSecondsAfterFinished = baseutils.Pointer(defaultTTLSecondsAfterFinished)
	}

	if err := common.AddEntrypoint(spec.EntryPoint, &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	jobSpec.Suspend = baseutils.Pointer(true)

	batchJob := handler.GetInitializedObject().(*batchv1.Job)

	batchJob.Spec = jobSpec

	common.CopyLabels(handler.KaiwoJob.ObjectMeta.Labels, &batchJob.ObjectMeta)
	common.SetKaiwoSystemLabels(labelContext, &batchJob.ObjectMeta)

	batchJob.ObjectMeta.Labels[common.QueueLabel] = common.GetClusterQueueName(ctx, handler)
	if commonSpec.PriorityClass != "" {
		batchJob.Spec.Template.Spec.PriorityClassName = commonSpec.PriorityClass
	}

	return batchJob, nil
}

func (handler *BatchJobHandler) ObserveStatus(_ context.Context, k8sClient client.Client, obj client.Object, previousStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	actualJob := obj.(*batchv1.Job)
	status, err := common.ObserveBatchJob(actualJob, previousStatus)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to observe batch job status: %w", err)
	}

	return &status, []metav1.Condition{}, nil
}

func (handler *BatchJobHandler) GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error) {
	job := handler.GetInitializedObject().(*batchv1.Job)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	workload, err := common.GetKueueWorkload(ctx, k8sClient, job.GetNamespace(), string(job.GetUID()))
	if err != nil {
		return nil, fmt.Errorf("failed to extract workload from handler: %w", err)
	}
	if workload == nil {
		return []kueuev1beta1.Workload{}, nil
	}
	return []kueuev1beta1.Workload{*workload}, nil
}

func GetDefaultJobSpec(config common.KaiwoConfigContext, dangerous bool, resourceRequirements corev1.ResourceRequirements) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(defaultTTLSecondsAfterFinished),
		BackoffLimit:            baseutils.Pointer(int32(0)),
		Template:                common.GetPodTemplate(config, *resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous, resourceRequirements, "workload"),
	}
}

//type BatchJobReconciler struct {
//	common.ResourceReconcilerBase[*batchv1.Job]
//	KaiwoJob *kaiwo.KaiwoJob
//}
//
//func NewBatchJobReconciler(kaiwoJob *kaiwo.KaiwoJob) *BatchJobReconciler {
//	reconciler := &BatchJobReconciler{
//		ResourceReconcilerBase: common.ResourceReconcilerBase[*batchv1.Job]{
//			ObjectKey: client.ObjectKeyFromObject(kaiwoJob),
//		},
//		KaiwoJob: kaiwoJob,
//	}
//	reconciler.Self = reconciler
//	return reconciler
//}
//
//func (r *BatchJobReconciler) Build(ctx context.Context, _ client.Client) (*batchv1.Job, error) {
//	logger := log.FromContext(ctx)
//	config := controllerutils.ConfigFromContext(ctx)
//
//	spec := r.KaiwoJob.Spec
//	labelContext := common.GetKaiwoLabelContext(r.KaiwoJob)
//
//	var jobSpec batchv1.JobSpec
//	jobSpec.Template.ObjectMeta.Labels = r.KaiwoJob.ObjectMeta.Labels
//
//	var overrideDefaults bool
//
//	if spec.Job == nil {
//		jobSpec = GetDefaultJobSpec(config, spec.Dangerous, baseutils.ValueOrDefault(spec.Resources))
//		if r.KaiwoJob.Spec.CommonMetaSpec.Gpus > 0 {
//			overrideDefaults = true
//		}
//		if r.KaiwoJob.Spec.CommonMetaSpec.Resources != nil {
//			overrideDefaults = false
//		}
//	} else {
//		jobSpec = spec.Job.Spec
//		overrideDefaults = false
//		common.SyncGpuMetaFromPodSpec(jobSpec.Template.Spec, &r.KaiwoJob.Spec.CommonMetaSpec)
//	}
//
//	if err := common.UpdatePodSpec(config, r.KaiwoJob.Spec.CommonMetaSpec, labelContext, &jobSpec.Template, r.KaiwoJob.Name, 1, r.KaiwoJob.Spec.CommonMetaSpec.Gpus, overrideDefaults, false); err != nil {
//		return nil, fmt.Errorf("failed to update job spec: %w", err)
//	}
//
//	if baseutils.ValueOrDefault(jobSpec.BackoffLimit) > 0 {
//		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
//		jobSpec.BackoffLimit = baseutils.Pointer(int32(0))
//	}
//
//	if jobSpec.TTLSecondsAfterFinished == nil {
//		jobSpec.TTLSecondsAfterFinished = baseutils.Pointer(defaultTTLSecondsAfterFinished)
//	}
//
//	if err := common.AddEntrypoint(spec.EntryPoint, &jobSpec.Template); err != nil {
//		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
//	}
//
//	jobSpec.Suspend = baseutils.Pointer(true)
//
//	job := &batchv1.Job{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      r.ObjectKey.Name,
//			Namespace: r.ObjectKey.Namespace,
//			Labels:    jobSpec.Template.ObjectMeta.Labels,
//		},
//		Spec: jobSpec,
//	}
//
//	common.CopyLabels(r.KaiwoJob.ObjectMeta.Labels, &job.ObjectMeta)
//	common.SetKaiwoSystemLabels(labelContext, &job.ObjectMeta)
//
//	job.ObjectMeta.Labels[common.QueueLabel] = r.KaiwoJob.Labels[common.QueueLabel]
//	if r.KaiwoJob.Spec.PriorityClass != "" {
//		job.Spec.Template.Spec.PriorityClassName = r.KaiwoJob.Spec.PriorityClass
//	}
//
//	return job, nil
//}
//
//func (r *BatchJobReconciler) GetEmptyObject() *batchv1.Job {
//	return &batchv1.Job{}
//}
//
//func (r *BatchJobReconciler) ValidateBeforeCreateOrUpdate(ctx context.Context, actual *batchv1.Job) (*ctrl.Result, error) {
//	// Abort reconciliation the managed label is set and actual doesn't exist, as the job is managed by the webhook
//	// This is to avoid trying to create the job that is going to be created once the webhook completes
//	return common.ValidateKaiwoResourceBeforeCreateOrUpdate(ctx, actual, r.KaiwoJob.ObjectMeta)
//}
