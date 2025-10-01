/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package workloadjob

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/silogen/kaiwo/pkg/workloads/utils"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetDefaultRayJobSpec(config common.KaiwoConfigContext, dangerous bool) rayv1.RayJobSpec {
	return rayv1.RayJobSpec{
		ShutdownAfterJobFinishes: true,
		RayClusterSpec:           utils.GetRayClusterTemplate(config, dangerous),
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
		rayJobSpec = GetDefaultRayJobSpec(config, spec.Dangerous)
	} else {
		rayJobSpec = spec.RayJob.Spec
	}

	// Ensure backoff limit is 0
	if baseutils.ValueOrDefault(rayJobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		rayJobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	// Convert entrypoint
	if spec.EntryPoint != "" {
		rayJobSpec.Entrypoint = baseutils.ConvertMultilineEntrypoint(spec.EntryPoint, true).(string)
	}

	// Update ray cluster specs
	utils.UpdateRayClusterSpec(ctx, clusterCtx, handler.KaiwoJob, rayJobSpec.RayClusterSpec)

	rayJob := handler.GetInitializedObject().(*rayv1.RayJob)
	rayJob.Labels = rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels
	rayJob.Spec = rayJobSpec

	common.UpdateLabels(handler.KaiwoJob, &rayJob.ObjectMeta)

	rayJob.Labels[common.QueueLabel] = common.GetClusterQueueName(ctx, handler)
	if priorityclass := handler.GetCommonSpec().WorkloadPriorityClass; priorityclass != "" {
		rayJob.Labels[common.WorkloaddPriorityClassLabel] = priorityclass
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
