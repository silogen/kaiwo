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

package workloadservice

import (
	"context"
	"fmt"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	workloadcommon "github.com/silogen/kaiwo/pkg/workloads/common"
)

func GetDefaultRayServiceSpec(dangerous bool, resourceRequirements v1.ResourceRequirements) rayv1.RayServiceSpec {
	return rayv1.RayServiceSpec{
		RayClusterSpec: *workloadcommon.GetRayClusterTemplate(dangerous, resourceRequirements),
	}
}

type RayServiceReconciler struct {
	workloadcommon.ResourceReconcilerBase[*rayv1.RayService]
	KaiwoService *v1alpha1.KaiwoService
}

func NewRayServiceReconciler(kaiwoService *v1alpha1.KaiwoService) *RayServiceReconciler {
	r := &RayServiceReconciler{
		ResourceReconcilerBase: workloadcommon.ResourceReconcilerBase[*rayv1.RayService]{
			ObjectKey: client.ObjectKeyFromObject(kaiwoService),
		},
		KaiwoService: kaiwoService,
	}
	r.Self = r
	return r
}

func (r *RayServiceReconciler) Build(ctx context.Context, k8sClient client.Client) (*rayv1.RayService, error) {
	logger := log.FromContext(ctx)

	spec := r.KaiwoService.Spec

	var rayServiceSpec rayv1.RayServiceSpec

	if spec.RayService == nil {
		rayServiceSpec = GetDefaultRayServiceSpec(
			baseutils.ValueOrDefault(spec.Dangerous),
			baseutils.ValueOrDefault(spec.Resources),
		)
	} else {
		rayServiceSpec = spec.RayService.Spec
	}

	if headMemoryOverride := os.Getenv("RAY_HEAD_POD_MEMORY"); len(headMemoryOverride) > 0 {
		quantity, err := resource.ParseQuantity(headMemoryOverride)
		if err != nil {
			msg := fmt.Sprintf("Failed to parse HEAD_POD_MEMORY %s", headMemoryOverride)
			logger.Error(err, msg)
			panic(msg)
		}
		workloadcommon.FillPodResources(
			&rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template.Spec,
			&v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: quantity,
				},
				Requests: v1.ResourceList{
					v1.ResourceMemory: quantity,
				},
			},
			true,
		)
	}

	labelContext := baseutils.GetKaiwoLabelContext(r.KaiwoService)

	replicas := baseutils.ValueOrDefault(spec.Replicas)
	gpusPerReplica := baseutils.ValueOrDefault(spec.GpusPerReplica)

	calculatedGpus := baseutils.ValueOrDefault(r.KaiwoService.Spec.CommonMetaSpec.Gpus)

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
	r.KaiwoService.Spec.CommonMetaSpec.Gpus = &calculatedGpus

	var overrideDefaults bool
	if calculatedGpus > 0 && spec.RayService == nil {
		overrideDefaults = true
	} else {
		overrideDefaults = false
	}

	if spec.RayService == nil {
		if err := workloadcommon.UpdatePodSpec(
			r.KaiwoService.Spec.CommonMetaSpec,
			labelContext,
			&rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template,
			r.KaiwoService.Name,
			replicas,
			gpusPerReplica,
			false,
		); err != nil {
			return nil, fmt.Errorf("failed to update head group spec: %w", err)
		}

		for i := range rayServiceSpec.RayClusterSpec.WorkerGroupSpecs {
			if err := workloadcommon.UpdatePodSpec(
				r.KaiwoService.Spec.CommonMetaSpec,
				labelContext,
				&rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].Template,
				r.KaiwoService.Name,
				replicas,
				gpusPerReplica,
				overrideDefaults,
			); err != nil {
				return nil, fmt.Errorf("failed to update worker group spec for index %d: %w", i, err)
			}
		}

		for i := range rayServiceSpec.RayClusterSpec.WorkerGroupSpecs {
			rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].Replicas = baseutils.Pointer(int32(replicas))
			rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].MinReplicas = baseutils.Pointer(int32(replicas))
			rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = baseutils.Pointer(int32(replicas))
		}
	}

	if baseutils.ValueOrDefault(spec.ServeConfigV2) != "" {
		rayServiceSpec.ServeConfigV2 = *spec.ServeConfigV2
	}

	rayService := &rayv1.RayService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template.ObjectMeta.Labels,
		},
		Spec: rayServiceSpec,
	}

	baseutils.CopyLabels(r.KaiwoService.GetLabels(), &rayService.ObjectMeta)
	baseutils.SetKaiwoSystemLabels(labelContext, &rayService.ObjectMeta)

	return rayService, nil
}

func (r *RayServiceReconciler) GetEmptyObject() *rayv1.RayService {
	return &rayv1.RayService{}
}
