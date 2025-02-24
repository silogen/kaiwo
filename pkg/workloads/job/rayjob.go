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
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"
)

func GetDefaultRayJobSpec(dangerous bool) rayv1.RayJobSpec {
	return rayv1.RayJobSpec{
		ShutdownAfterJobFinishes: true,
		RayClusterSpec: &rayv1.RayClusterSpec{
			EnableInTreeAutoscaling: baseutils.Pointer(false),
			HeadGroupSpec: rayv1.HeadGroupSpec{
				RayStartParams: map[string]string{},
				Template:       controllerutils.GetPodTemplate(resource.MustParse("1Gi"), dangerous),
			},
			WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
				{
					GroupName:      "default-worker-group",
					Replicas:       baseutils.Pointer(int32(1)),
					MinReplicas:    baseutils.Pointer(int32(1)),
					MaxReplicas:    baseutils.Pointer(int32(1)),
					RayStartParams: map[string]string{},
					Template:       controllerutils.GetPodTemplate(resource.MustParse("200Gi"), dangerous), // TODO: add to CRD as configurable field
				},
			},
		},
	}
}

type RayJobReconciler struct {
	workloadutils.ResourceReconcilerBase[*rayv1.RayJob]
	KaiwoJob *v1alpha1.KaiwoJob
}

func NewRayJobReconciler(kaiwoJob *v1alpha1.KaiwoJob) *RayJobReconciler {
	reconciler := &RayJobReconciler{
		ResourceReconcilerBase: workloadutils.ResourceReconcilerBase[*rayv1.RayJob]{
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
		// logger.Info("RayJobSpec is nil, using DefaultRayJobSpec", "KaiwoJob", kaiwoJob.Name)
		rayJobSpec = GetDefaultRayJobSpec(baseutils.ValueOrDefault(spec.Dangerous))
	} else {
		rayJobSpec = spec.RayJob.Spec
	}

	if rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels == nil {
		rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels = make(map[string]string)
	}
	rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels["job-name"] = r.ObjectKey.Name

	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Labels == nil {
			rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Labels = make(map[string]string)
		}
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Labels["job-name"] = r.ObjectKey.Name
	}

	if baseutils.ValueOrDefault(spec.EntryPoint) != "" {
		rayJobSpec.Entrypoint = *spec.EntryPoint
	}

	replicas := baseutils.ValueOrDefault(spec.Replicas)
	gpusPerReplica := baseutils.ValueOrDefault(spec.GpusPerReplica)

	if baseutils.ValueOrDefault(spec.Gpus) > 0 || gpusPerReplica > 0 {
		// Only calculate if it is needed, otherwise avoid interacting with the cluster
		var err error
		replicas, gpusPerReplica, err = controllerutils.CalculateNumberOfReplicas(
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

	// Adjust resource requests & limits
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(replicas))
		if err := controllerutils.AdjustResourceRequestsAndLimits(
			ctx,
			baseutils.ValueOrDefault(spec.GpuVendor),
			baseutils.ValueOrDefault(spec.Gpus),
			baseutils.ValueOrDefault(spec.Replicas),
			baseutils.ValueOrDefault(spec.GpusPerReplica),
			&rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template,
		); err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to adjust ray job spec", err)
		}
	}

	// Add environment variables
	if err := controllerutils.AddEnvVars(ctx, baseutils.ValueOrDefault(spec.EnvVars), &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add env vars to head", err)
	}
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if err := controllerutils.AddEnvVars(ctx, baseutils.ValueOrDefault(spec.EnvVars), &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template); err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to add env vars to worker group", err)
		}
	}

	if spec.Storage != nil && spec.Storage.StorageEnabled {
		// Attach storage to head pod
		if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Spec, *spec.Storage, r.ObjectKey.Name); err != nil {
			return nil, err
		}

		// Attach storage to worker pods
		for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
			if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec, *spec.Storage, r.ObjectKey.Name); err != nil {
				return nil, err
			}
		}
	}

	// Construct the RayJob object
	rayJob := &rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    r.KaiwoJob.Labels,
		},
		Spec: rayJobSpec,
	}

	return rayJob, nil
}

func (r *RayJobReconciler) GetEmptyObject() *rayv1.RayJob {
	return &rayv1.RayJob{}
}
