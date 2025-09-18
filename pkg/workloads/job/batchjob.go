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

	batchv1 "k8s.io/api/batch/v1"
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

	var jobSpec batchv1.JobSpec

	if spec.Job == nil {
		jobSpec = GetDefaultJobSpec(config, spec.Dangerous)
	} else {
		jobSpec = spec.Job.Spec
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

	resourceConfig := common.CalculateResourceConfig(ctx, clusterCtx, handler.KaiwoJob, true)
	common.UpdatePodSpec(config, handler.KaiwoJob, resourceConfig, &jobSpec.Template, false)

	batchJob := handler.GetInitializedObject().(*batchv1.Job)
	batchJob.Spec = jobSpec

	common.UpdateLabels(handler.KaiwoJob, &batchJob.ObjectMeta)
	common.UpdateLabels(handler.KaiwoJob, &batchJob.Spec.Template.ObjectMeta)

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

func GetDefaultJobSpec(config common.KaiwoConfigContext, dangerous bool) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(defaultTTLSecondsAfterFinished),
		BackoffLimit:            baseutils.Pointer(int32(0)),
		Template:                common.GetPodTemplate(config, *resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous, "workload"),
	}
}
