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

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/silogen/kaiwo/pkg/workloads/utils"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

type RayServiceHandler struct {
	KaiwoService *kaiwo.KaiwoService
	Scheme       *runtime.Scheme
}

func (handler *RayServiceHandler) GetKaiwoWorkloadObject() client.Object {
	return handler.KaiwoService
}

func (handler *RayServiceHandler) GetCommonSpec() kaiwo.CommonMetaSpec {
	return handler.KaiwoService.Spec.CommonMetaSpec
}

func (handler *RayServiceHandler) GetCommonStatusSpec() *kaiwo.CommonStatusSpec {
	return &handler.KaiwoService.Status.CommonStatusSpec
}

func (handler *RayServiceHandler) GetInitializedObject() client.Object {
	return &rayv1.RayService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rayv1.SchemeGroupVersion.String(),
			Kind:       "RayService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoService.Name,
			Namespace: handler.KaiwoService.Namespace,
			Labels:    map[string]string{},
		},
	}
}

func (handler *RayServiceHandler) BuildDesired(ctx context.Context, clusterCtx common.ClusterContext) (client.Object, error) {
	// Compute resource config so Ray worker replicas match desired
	_ = common.CalculateResourceConfig(ctx, clusterCtx, handler.KaiwoService, true)

	rayService := handler.buildRayService(ctx, clusterCtx)
	common.UpdateLabels(handler.KaiwoService, &rayService.ObjectMeta)
	return &rayService, nil
}

func (handler *RayServiceHandler) buildRayService(ctx context.Context, clusterCtx common.ClusterContext) rayv1.RayService {
	config := common.ConfigFromContext(ctx)

	spec := handler.KaiwoService.Spec

	var rayServiceSpec rayv1.RayServiceSpec

	if spec.RayService == nil {
		rayServiceSpec = GetDefaultRayServiceSpec(
			config,
			spec.Dangerous,
		)
	} else {
		rayServiceSpec = spec.RayService.Spec
	}

	if spec.ServeConfigV2 != "" {
		rayServiceSpec.ServeConfigV2 = spec.ServeConfigV2
	}

	// Update ray cluster specs
	utils.UpdateRayClusterSpec(ctx, clusterCtx, handler.KaiwoService, &rayServiceSpec.RayClusterSpec)

	rayService := rayv1.RayService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rayv1.SchemeGroupVersion.String(),
			Kind:       "RayService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoService.Name,
			Namespace: handler.KaiwoService.Namespace,
			Labels:    rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template.Labels,
		},
		Spec: rayServiceSpec,
	}

	common.UpdateLabels(handler.KaiwoService, &rayService.ObjectMeta)
	return rayService
}

func (handler *RayServiceHandler) MutateActual(ctx context.Context, clusterCtx common.ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

func (handler *RayServiceHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	rayService := &rayv1.RayService{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), rayService); err != nil {
		if !errors.IsNotFound(err) {
			return nil, nil, fmt.Errorf("failed to get rayService: %w", err)
		}
		return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
	}

	if meta.IsStatusConditionTrue(rayService.Status.Conditions, string(rayv1.RayServiceReady)) {
		return baseutils.Pointer(kaiwo.WorkloadStatusRunning), nil, nil
	}

	for _, appStat := range rayService.Status.ActiveServiceStatus.Applications {
		if appStat.Status == "UNHEALTHY" || appStat.Status == "DEPLOY_FAILED" {
			return baseutils.Pointer(kaiwo.WorkloadStatusFailed), nil, nil
		}
	}

	return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
}

func (handler *RayServiceHandler) HandleStatusChange(ctx context.Context, k8sClient client.Client, obj client.Object, newStatus kaiwo.WorkloadStatus) error {
	return nil
}

func GetDefaultRayServiceSpec(config common.KaiwoConfigContext, dangerous bool) rayv1.RayServiceSpec {
	return rayv1.RayServiceSpec{
		RayClusterSpec: *utils.GetRayClusterTemplate(config, dangerous),
	}
}
