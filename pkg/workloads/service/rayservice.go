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
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/silogen/kaiwo/pkg/workloads/utils"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	return &appwrapperv1beta2.AppWrapper{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appwrapperv1beta2.GroupVersion.String(),
			Kind:       "AppWrapper",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoService.Name,
			Namespace: handler.KaiwoService.Namespace,
			Labels:    map[string]string{},
		},
	}
}

func (handler *RayServiceHandler) BuildDesired(ctx context.Context, clusterCtx common.ClusterContext) (client.Object, error) {
	rayService := handler.buildRayService(ctx, clusterCtx)

	// Wrap the RayService with an AppWrapper
	resourceConfig := common.CalculateResourceConfig(ctx, clusterCtx, handler.KaiwoService, true)

	rayServiceSpecBytes, err := json.Marshal(rayService.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RayServiceSpec: %w", err)
	}

	labelsBytes, err := json.Marshal(rayService.Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal labels: %w", err)
	}

	appWrapper := handler.GetInitializedObject().(*appwrapperv1beta2.AppWrapper)
	appWrapper.Labels = map[string]string{
		common.QueueLabel: common.GetClusterQueueName(ctx, handler),
	}
	if priorityclass := handler.GetCommonSpec().WorkloadPriorityClass; priorityclass != "" {
		appWrapper.Labels[common.WorkloaddPriorityClassLabel] = priorityclass
	}
	appWrapper.Spec = appwrapperv1beta2.AppWrapperSpec{
		Components: []appwrapperv1beta2.AppWrapperComponent{
			{
				DeclaredPodSets: []appwrapperv1beta2.AppWrapperPodSet{
					{Replicas: baseutils.Pointer(int32(1)), Path: "template.spec.rayClusterConfig.headGroupSpec.template"},
					{Replicas: baseutils.Pointer(int32(resourceConfig.Replicas)), Path: "template.spec.rayClusterConfig.workerGroupSpecs[0].template"},
				},
				Template: runtime.RawExtension{
					Raw: []byte(fmt.Sprintf(`{
				    "apiVersion": "ray.io/v1",
				    "kind": "RayService",
				    "metadata": {
					"name": "%s",
					"namespace": "%s",
					"labels": %s
				    },
				    "spec": %s
				}`, handler.KaiwoService.Name, handler.KaiwoService.Namespace, labelsBytes, rayServiceSpecBytes)),
				},
			},
		},
	}

	common.UpdateLabels(handler.KaiwoService, &appWrapper.ObjectMeta)

	return appWrapper, nil
}

func (handler *RayServiceHandler) buildRayService(ctx context.Context, clusterCtx common.ClusterContext) rayv1.RayService {
	config := common.ConfigFromContext(ctx)

	spec := handler.KaiwoService.Spec

	var rayServiceSpec rayv1.RayServiceSpec

	if spec.Ray.Spec == nil {
		rayServiceSpec = GetDefaultRayServiceSpec(
			config,
			spec.Dangerous,
		)
	} else {
		rayServiceSpec = *spec.Ray.Spec
	}

	// Allow overriding ServeConfigV2 via spec.ray.serveConfigV2
	if spec.Ray != nil && spec.Ray.ServeConfigV2 != "" {
		rayServiceSpec.ServeConfigV2 = spec.Ray.ServeConfigV2
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

func (handler *RayServiceHandler) GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error) {
	appWrapper := &appwrapperv1beta2.AppWrapper{}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(handler.KaiwoService), appWrapper); err != nil {
		return nil, fmt.Errorf("failed to get app wrapper: %w", err)
	}

	workload, err := common.GetKueueWorkload(ctx, k8sClient, appWrapper.GetNamespace(), string(appWrapper.GetUID()))
	if err != nil {
		return nil, fmt.Errorf("failed to extract workload from handler: %w", err)
	}
	if workload == nil {
		return []kueuev1beta1.Workload{}, nil
	}
	return []kueuev1beta1.Workload{*workload}, nil
}

func (handler *RayServiceHandler) HandleStatusChange(ctx context.Context, k8sClient client.Client, obj client.Object, newStatus kaiwo.WorkloadStatus) error {
	return nil
}

func GetDefaultRayServiceSpec(config common.KaiwoConfigContext, dangerous bool) rayv1.RayServiceSpec {
	return rayv1.RayServiceSpec{
		RayClusterSpec: *utils.GetRayClusterTemplate(config, dangerous),
	}
}
