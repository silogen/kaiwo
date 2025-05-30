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
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/silogen/kaiwo/pkg/workloads/utils"

	"k8s.io/apimachinery/pkg/api/resource"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	v1 "k8s.io/api/core/v1"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetDefaultRayJobSpec(config common.KaiwoConfigContext, dangerous bool, resourceRequirements v1.ResourceRequirements) rayv1.RayJobSpec {
	return rayv1.RayJobSpec{
		ShutdownAfterJobFinishes: true,
		RayClusterSpec:           utils.GetRayClusterTemplate(config, dangerous, resourceRequirements),
	}
}

type RayJobHandler struct {
	KaiwoJob *kaiwo.KaiwoJob
	Scheme   *runtime.Scheme
}

func (handler *RayJobHandler) GetInitializedObject() client.Object {
	return &rayv1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RayJob",
			APIVersion: rayv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoJob.Name,
			Namespace: handler.KaiwoJob.Namespace,
			Labels:    map[string]string{},
		},
	}
}

func (handler *RayJobHandler) GetKaiwoWorkloadObject() client.Object {
	return handler.KaiwoJob
}

func (handler *RayJobHandler) GetCommonSpec() kaiwo.CommonMetaSpec {
	return handler.KaiwoJob.Spec.CommonMetaSpec
}

func (handler *RayJobHandler) GetCommonStatusSpec() *kaiwo.CommonStatusSpec {
	return &handler.KaiwoJob.Status.CommonStatusSpec
}

func (handler *RayJobHandler) BuildDesired(ctx context.Context, clusterCtx common.ClusterContext) (client.Object, error) {
	logger := log.FromContext(ctx)
	config := common.ConfigFromContext(ctx)

	spec := handler.KaiwoJob.Spec

	var rayJobSpec rayv1.RayJobSpec

	if spec.RayJob == nil {
		rayJobSpec = GetDefaultRayJobSpec(config, spec.Dangerous, baseutils.ValueOrDefault(spec.Resources))
	} else {
		rayJobSpec = spec.RayJob.Spec
		for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
			common.SyncGpuMetaFromPodSpec(rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec, &handler.KaiwoJob.Spec.CommonMetaSpec)
		}
	}

	if baseutils.ValueOrDefault(rayJobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		rayJobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	if headMemoryOverride := resource.MustParse(config.Ray.HeadPodMemory); headMemoryOverride.Value() > 0 {
		common.FillPodResources(&rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Spec, &v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceMemory: headMemoryOverride,
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: headMemoryOverride,
			},
		}, true)
	}

	labelContext := common.GetKaiwoLabelContext(handler.KaiwoJob)

	schedulingConfig := common.CalculateSchedulingConfig(ctx, clusterCtx, handler.KaiwoJob, true)
	spec.Replicas = &schedulingConfig.Replicas
	spec.GpusPerReplica = schedulingConfig.GpusPerReplica

	var overrideDefaults bool

	if schedulingConfig.TotalGpus > 0 && spec.RayJob == nil {
		overrideDefaults = true
	} else {
		overrideDefaults = false
	}

	if err := common.UpdatePodSpec(
		config,
		handler.KaiwoJob.Spec.CommonMetaSpec,
		labelContext,
		&rayJobSpec.RayClusterSpec.HeadGroupSpec.Template,
		handler.KaiwoJob.Name,
		schedulingConfig.Replicas,
		schedulingConfig.GpusPerReplica,
		false,
		true,
	); err != nil {
		return nil, fmt.Errorf("failed to update job spec: %w", err)
	}

	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if err := common.UpdatePodSpec(
			config,
			handler.KaiwoJob.Spec.CommonMetaSpec,
			labelContext,
			&rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template,
			handler.KaiwoJob.Name,
			schedulingConfig.Replicas,
			schedulingConfig.GpusPerReplica,
			overrideDefaults,
			false,
		); err != nil {
			return nil, fmt.Errorf("failed to update job spec for container %d: %w", i, err)
		}
	}

	if spec.EntryPoint != "" {
		rayJobSpec.Entrypoint = baseutils.ConvertMultilineEntrypoint(spec.EntryPoint, true).(string)
	}

	// Adjust resource requests & limits
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(schedulingConfig.Replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(schedulingConfig.Replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(schedulingConfig.Replicas))
	}

	rayJob := handler.GetInitializedObject().(*rayv1.RayJob)
	rayJob.ObjectMeta.Labels = rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.ObjectMeta.Labels
	rayJob.Spec = rayJobSpec

	common.CopyLabels(handler.KaiwoJob.ObjectMeta.Labels, &rayJob.ObjectMeta)
	common.SetKaiwoSystemLabels(labelContext, &rayJob.ObjectMeta)

	// TODO set in update pod spec
	rayJob.ObjectMeta.Labels[common.QueueLabel] = common.GetClusterQueueName(ctx, handler)
	if handler.KaiwoJob.Spec.PriorityClass != "" {
		rayJob.Spec.RayClusterSpec.HeadGroupSpec.Template.Spec.PriorityClassName = handler.KaiwoJob.Spec.PriorityClass
		for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
			rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec.PriorityClassName = handler.KaiwoJob.Spec.PriorityClass
		}
	}

	return rayJob, nil
}

func (handler *RayJobHandler) MutateActual(ctx context.Context, clusterCtx common.ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

func (handler *RayJobHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	job := obj.(*rayv1.RayJob)

	switch job.Status.JobStatus {
	case rayv1.JobStatusNew, rayv1.JobStatusPending:
		// Kaiwo status Starting means that the job has been admitted, and is starting up
		return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
	case rayv1.JobStatusRunning:
		return baseutils.Pointer(kaiwo.WorkloadStatusRunning), nil, nil
	case rayv1.JobStatusFailed:
		return baseutils.Pointer(kaiwo.WorkloadStatusFailed), nil, nil
	case rayv1.JobStatusSucceeded:
		return baseutils.Pointer(kaiwo.WorkloadStatusComplete), nil, nil
	case rayv1.JobStatusStopped:
		return baseutils.Pointer(kaiwo.WorkloadStatusTerminated), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected job status: %s", job.Status.JobStatus)
	}
}

func (handler *RayJobHandler) GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error) {
	rayJob := &rayv1.RayJob{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(handler.KaiwoJob), rayJob); err != nil {
		return nil, fmt.Errorf("failed to get rayJob: %w", err)
	}
	workload, err := common.GetKueueWorkload(ctx, k8sClient, rayJob.GetNamespace(), string(rayJob.GetUID()))
	if err != nil {
		return nil, fmt.Errorf("failed to extract workload from handler: %w", err)
	}
	if workload == nil {
		return []kueuev1beta1.Workload{}, nil
	}
	return []kueuev1beta1.Workload{*workload}, nil
}
