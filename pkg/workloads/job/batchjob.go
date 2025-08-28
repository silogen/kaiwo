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

package workloads

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/kube/podspec"

	"github.com/silogen/kaiwo/pkg/platform/kueue"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/silogen/kaiwo/pkg/platform/cluster"

	"github.com/silogen/kaiwo/pkg/runtime/config"

	common2 "github.com/silogen/kaiwo/pkg/runtime/common"

	"github.com/silogen/kaiwo/pkg/api"

	"k8s.io/apimachinery/pkg/runtime"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
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

func (handler *BatchJobHandler) MutateActual(ctx context.Context, clusterCtx api.ClusterContext, actual client.Object) error {
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

func (handler *BatchJobHandler) BuildDesired(ctx context.Context, clusterCtx api.ClusterContext) (client.Object, error) {
	logger := log.FromContext(ctx)
	config := config.ConfigFromContext(ctx)

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

	if err := podspec.AddEntrypoint(spec.EntryPoint, &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	jobSpec.Suspend = baseutils.Pointer(true)

	gpuSchedulingResult, err := cluster.CalculateGpuRequirements(ctx, clusterCtx, handler.KaiwoJob.Spec.GpuResources, baseutils.Pointer(1))
	if err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to calculate gpu requirements: %v", err)
	}

	podspec.UpdatePodTemplateSpecNonRay(config, handler, gpuSchedulingResult, &jobSpec.Template)

	// IndexedCompletion is required by Kueue TAS
	jobSpec.CompletionMode = baseutils.Pointer(batchv1.IndexedCompletion)

	batchJob := handler.GetInitializedObject().(*batchv1.Job)
	batchJob.Spec = jobSpec

	common2.UpdateLabels(handler.KaiwoJob, &batchJob.ObjectMeta)
	common2.UpdateLabels(handler.KaiwoJob, &batchJob.Spec.Template.ObjectMeta)

	batchJob.Labels[common2.QueueLabel] = api.GetClusterQueueName(ctx, handler)

	return batchJob, nil
}

func (handler *BatchJobHandler) GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error) {
	logger := log.FromContext(ctx).WithName("GetKueueWorkloads")

	job := handler.GetInitializedObject().(*batchv1.Job)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Use the Job UID to match Kueue Workload ownerReference

	workload, err := kueue.GetKueueWorkload(ctx, k8sClient, job.GetNamespace(), string(job.GetUID()))
	if err != nil {
		logger.Error(err, "failed to get Kueue Workload", "namespace", job.GetNamespace(), "jobUID", string(job.GetUID()))
		return nil, fmt.Errorf("failed to extract workload from handler: %w", err)
	}
	if workload == nil {
		return []kueuev1beta1.Workload{}, nil
	}

	return []kueuev1beta1.Workload{*workload}, nil
}

func GetDefaultJobSpec(config config.KaiwoConfigContext, dangerous bool) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(defaultTTLSecondsAfterFinished),
		BackoffLimit:            baseutils.Pointer(int32(0)),
		Template:                podspec.GetPodTemplate(config, *resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous, "workload"),
		// Just to be explicit about the values
		Completions: baseutils.Pointer(int32(1)),
		Parallelism: baseutils.Pointer(int32(1)),
	}
}
