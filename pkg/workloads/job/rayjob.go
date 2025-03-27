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
	"os"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"

	v1 "k8s.io/api/core/v1"

	workloadcommon "github.com/silogen/kaiwo/pkg/workloads/common"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetDefaultRayJobSpec(dangerous bool, resourceRequirements v1.ResourceRequirements) rayv1.RayJobSpec {
	return rayv1.RayJobSpec{
		ShutdownAfterJobFinishes: true,
		RayClusterSpec:           workloadcommon.GetRayClusterTemplate(dangerous, resourceRequirements),
	}
}

type RayJobReconciler struct {
	workloadcommon.ResourceReconcilerBase[*rayv1.RayJob]
	KaiwoJob *v1alpha1.KaiwoJob
}

func NewRayJobReconciler(kaiwoJob *v1alpha1.KaiwoJob) *RayJobReconciler {
	reconciler := &RayJobReconciler{
		ResourceReconcilerBase: workloadcommon.ResourceReconcilerBase[*rayv1.RayJob]{
			ObjectKey: client.ObjectKeyFromObject(kaiwoJob),
		},
		KaiwoJob: kaiwoJob,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *RayJobReconciler) Build(ctx context.Context, k8sClient client.Client) (*rayv1.RayJob, error) {
	logger := log.FromContext(ctx)

	spec := r.KaiwoJob.Spec

	var rayJobSpec rayv1.RayJobSpec

	if spec.RayJob == nil {
		rayJobSpec = GetDefaultRayJobSpec(baseutils.ValueOrDefault(spec.Dangerous), baseutils.ValueOrDefault(spec.Resources))
	} else {
		rayJobSpec = spec.RayJob.Spec
	}

	if baseutils.ValueOrDefault(rayJobSpec.BackoffLimit) > 0 {
		logger.Info("Warning! BackOffLimit can currently only be 0, overriding the given value")
		rayJobSpec.BackoffLimit = baseutils.Pointer(int32(0))
	}

	// Override ray head pod memory, if the env var is set
	if headMemoryOverride := os.Getenv("RAY_HEAD_POD_MEMORY"); len(headMemoryOverride) > 0 {
		quantity, err := resource.ParseQuantity(headMemoryOverride)
		if err != nil {
			msg := fmt.Sprintf("Failed to parse HEAD_POD_MEMORY %s", headMemoryOverride)
			logger.Error(err, msg)
			panic(msg)
		}
		workloadcommon.FillPodResources(&rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Spec, &v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceMemory: quantity,
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: quantity,
			},
		}, true)
	}

	labelContext := baseutils.GetKaiwoLabelContext(r.KaiwoJob)

	replicas := baseutils.ValueOrDefault(spec.Replicas)
	gpusPerReplica := baseutils.ValueOrDefault(spec.GpusPerReplica)

	calculatedGpus := baseutils.ValueOrDefault(r.KaiwoJob.Spec.CommonMetaSpec.Gpus)

	if baseutils.ValueOrDefault(spec.Gpus) > 0 || gpusPerReplica > 0 {
		var err error
		calculatedGpus, replicas, gpusPerReplica, err = controllerutils.CalculateNumberOfReplicas(
			ctx,
			k8sClient,
			strings.ToLower(baseutils.ValueOrDefault(spec.GpuVendor)),
			baseutils.ValueOrDefault(spec.Gpus),
			baseutils.ValueOrDefault(spec.Replicas),
			baseutils.ValueOrDefault(spec.GpusPerReplica),
			true,
		)
		if err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to calculate number of replicas", err)
		}
	}

	spec.Replicas = &replicas
	spec.GpusPerReplica = &gpusPerReplica
	r.KaiwoJob.Spec.CommonMetaSpec.Gpus = &calculatedGpus

	var overrideDefaults bool

	if calculatedGpus > 0 && spec.RayJob == nil {
		overrideDefaults = true
	} else {
		overrideDefaults = false
	}

	if err := workloadcommon.UpdatePodSpec(r.KaiwoJob.Spec.CommonMetaSpec, labelContext, &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template, r.KaiwoJob.Name, replicas, gpusPerReplica, false); err != nil {
		return nil, fmt.Errorf("failed to update job spec: %w", err)
	}

	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if err := workloadcommon.UpdatePodSpec(r.KaiwoJob.Spec.CommonMetaSpec, labelContext, &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template, r.KaiwoJob.Name, replicas, gpusPerReplica, overrideDefaults); err != nil {
			return nil, fmt.Errorf("failed to update job spec for container %d: %w", i, err)
		}
	}

	if baseutils.ValueOrDefault(spec.EntryPoint) != "" {
		rayJobSpec.Entrypoint = baseutils.ConvertMultilineEntrypoint(*spec.EntryPoint, true).(string)
	}

	// Adjust resource requests & limits
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(replicas))
	}

	rayJob := &rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.ObjectMeta.Labels,
		},
		Spec: rayJobSpec,
	}

	baseutils.CopyLabels(r.KaiwoJob.ObjectMeta.Labels, &rayJob.ObjectMeta)
	baseutils.SetKaiwoSystemLabels(labelContext, &rayJob.ObjectMeta)

	rayJob.ObjectMeta.Labels[v1alpha1.QueueLabel] = r.KaiwoJob.Labels[v1alpha1.QueueLabel]

	return rayJob, nil
}

func (r *RayJobReconciler) GetEmptyObject() *rayv1.RayJob {
	return &rayv1.RayJob{}
}

func (r *RayJobReconciler) ValidateBeforeCreateOrUpdate(ctx context.Context, actual *rayv1.RayJob) (*ctrl.Result, error) {
	// Abort reconciliation the managed label is set and actual doesn't exist, as the job is managed by the webhook
	// This is to avoid trying to create the job that is going to be created once the webhook completes
	return workloadcommon.ValidateKaiwoResourceBeforeCreateOrUpdate(ctx, actual, r.KaiwoJob.ObjectMeta)
}
