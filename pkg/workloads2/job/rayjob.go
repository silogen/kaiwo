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
	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"
)

var DefaultRayJobSpec = rayv1.RayJobSpec{
	ShutdownAfterJobFinishes: true,
	RayClusterSpec: &rayv1.RayClusterSpec{
		EnableInTreeAutoscaling: baseutils.Pointer(false),
		HeadGroupSpec: rayv1.HeadGroupSpec{
			RayStartParams: map[string]string{},
			Template:       controllerutils.GetPodTemplate(resource.MustParse("1Gi")),
		},
		WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
			{
				GroupName:      "default-worker-group",
				Replicas:       baseutils.Pointer(int32(1)),
				MinReplicas:    baseutils.Pointer(int32(1)),
				MaxReplicas:    baseutils.Pointer(int32(1)),
				RayStartParams: map[string]string{},
				Template:       controllerutils.GetPodTemplate(resource.MustParse("200Gi")), // TODO: add to CRD as configurable field
			},
		},
	},
}

type RayJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	KaiwoJob *v1alpha1.KaiwoJob
}

func NewRayJobCommand(base workloadutils.CommandBase[workloadutils.CommandStateBase], kaiwoJob *v1alpha1.KaiwoJob) *RayJobCommand {
	cmd := &RayJobCommand{
		CommandBase: base,
		KaiwoJob:    kaiwoJob,
	}
	cmd.Self = cmd
	return cmd
}

func (k *RayJobCommand) Build(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	logger := log.FromContext(ctx)

	kaiwoJob := k.KaiwoJob

	kaiwoJob.Spec.Ray = baseutils.Pointer(true)

	var rayJobSpec rayv1.RayJobSpec
	if kaiwoJob.Spec.RayJob == nil {
		logger.Info("RayJobSpec is nil, using DefaultRayJobSpec", "KaiwoJob", kaiwoJob.Name)
		rayJobSpec = *DefaultRayJobSpec.DeepCopy()
	} else {
		rayJobSpec = kaiwoJob.Spec.RayJob.Spec
	}

	if rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels == nil {
		rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels = make(map[string]string)
	}
	rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Labels["job-name"] = kaiwoJob.Name

	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Labels == nil {
			rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Labels = make(map[string]string)
		}
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Labels["job-name"] = kaiwoJob.Name
	}

	if baseutils.ValueOrDefault(kaiwoJob.Spec.EntryPoint) != "" {
		rayJobSpec.Entrypoint = *kaiwoJob.Spec.EntryPoint
	}

	replicas := baseutils.ValueOrDefault(kaiwoJob.Spec.Replicas)
	gpusPerReplica := baseutils.ValueOrDefault(kaiwoJob.Spec.GpusPerReplica)

	if replicas == 0 || gpusPerReplica == 0 {
		// Only calculate if it is needed, otherwise avoid interacting with the cluster
		var err error
		replicas, gpusPerReplica, err = controllerutils.CalculateNumberOfReplicas(
			ctx,
			k8sClient,
			strings.ToLower(baseutils.ValueOrDefault(kaiwoJob.Spec.GpuVendor)),
			baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus),
			baseutils.ValueOrDefault(kaiwoJob.Spec.Replicas),
			baseutils.ValueOrDefault(kaiwoJob.Spec.GpusPerReplica),
			true,
		)
		if err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to calculate number of replicas", err)
		}
	}

	kaiwoJob.Spec.Replicas = &replicas
	kaiwoJob.Spec.GpusPerReplica = &gpusPerReplica

	// Adjust resource requests & limits
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(replicas))
		if err := controllerutils.AdjustResourceRequestsAndLimits(
			ctx,
			baseutils.ValueOrDefault(kaiwoJob.Spec.GpuVendor),
			baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus),
			baseutils.ValueOrDefault(kaiwoJob.Spec.Replicas),
			baseutils.ValueOrDefault(kaiwoJob.Spec.GpusPerReplica),
			&rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template,
		); err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to adjust ray job spec", err)
		}
	}

	// Add environment variables
	if err := controllerutils.AddEnvVars(ctx, baseutils.ValueOrDefault(kaiwoJob.Spec.EnvVars), &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add env vars to head", err)
	}
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if err := controllerutils.AddEnvVars(ctx, baseutils.ValueOrDefault(kaiwoJob.Spec.EnvVars), &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template); err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to add env vars to worker group", err)
		}
	}

	if kaiwoJob.Spec.Storage != nil && kaiwoJob.Spec.Storage.StorageEnabled {
		// Attach storage to head pod
		if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Spec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
			return nil, err
		}

		// Attach storage to worker pods
		for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
			if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
				return nil, err
			}
		}
	}

	objectKey := k.GetObjectKey()

	// Construct the RayJob object
	rayJob := &rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: rayJobSpec,
	}

	return rayJob, nil
}

func (k *RayJobCommand) GetEmptyObject() client.Object {
	return &rayv1.RayJob{}
}

func (k *RayJobCommand) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: k.KaiwoJob.Namespace,
		Name:      k.KaiwoJob.Name,
	}
}
