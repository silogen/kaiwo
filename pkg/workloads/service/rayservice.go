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
	"strings"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	ctrl "sigs.k8s.io/controller-runtime"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

func GetDefaultRayServiceSpec(config controllerutils.KaiwoConfigContext, dangerous bool, resourceRequirements v1.ResourceRequirements) rayv1.RayServiceSpec {
	return rayv1.RayServiceSpec{
		RayClusterSpec: *workloadutils.GetRayClusterTemplate(config, dangerous, resourceRequirements),
	}
}

type RayServiceReconciler struct {
	common.ResourceReconcilerBase[*appwrapperv1beta2.AppWrapper]
	KaiwoService *kaiwo.KaiwoService
}

func NewRayServiceReconciler(kaiwoService *kaiwo.KaiwoService) *RayServiceReconciler {
	r := &RayServiceReconciler{
		ResourceReconcilerBase: common.ResourceReconcilerBase[*appwrapperv1beta2.AppWrapper]{
			ObjectKey: client.ObjectKeyFromObject(kaiwoService),
		},
		KaiwoService: kaiwoService,
	}
	r.Self = r
	return r
}

func (r *RayServiceReconciler) Build(ctx context.Context, k8sClient client.Client) (*appwrapperv1beta2.AppWrapper, error) {
	logger := log.FromContext(ctx)
	config := controllerutils.ConfigFromContext(ctx)

	spec := r.KaiwoService.Spec

	var rayServiceSpec rayv1.RayServiceSpec

	if spec.RayService == nil {
		rayServiceSpec = GetDefaultRayServiceSpec(
			config,
			spec.Dangerous,
			baseutils.ValueOrDefault(spec.Resources),
		)
	} else {
		rayServiceSpec = spec.RayService.Spec
		for i := range rayServiceSpec.RayClusterSpec.WorkerGroupSpecs {
			workloadutils.SyncGpuMetaFromPodSpec(rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec, &r.KaiwoService.Spec.CommonMetaSpec)
		}
	}

	if headMemoryOverride := config.Ray.HeadPodMemory; headMemoryOverride.Value() > 0 {
		workloadutils.FillPodResources(
			&rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template.Spec,
			&v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: headMemoryOverride,
				},
				Requests: v1.ResourceList{
					v1.ResourceMemory: headMemoryOverride,
				},
			},
			true,
		)
	}

	labelContext := common.GetKaiwoLabelContext(r.KaiwoService)

	replicas := baseutils.ValueOrDefault(spec.Replicas)
	gpusPerReplica := spec.GpusPerReplica

	calculatedGpus := r.KaiwoService.Spec.CommonMetaSpec.Gpus

	if spec.Gpus > 0 || gpusPerReplica > 0 {
		var err error
		calculatedGpus, replicas, gpusPerReplica, err = controllerutils.CalculateNumberOfReplicas(
			ctx,
			k8sClient,
			strings.ToLower(spec.GpuVendor),
			spec.Gpus,
			baseutils.ValueOrDefault(spec.Replicas),
			spec.GpusPerReplica,
			true,
		)
		if err != nil {
			return nil, baseutils.LogErrorf(logger, "failed to calculate number of replicas", err)
		}
	}

	spec.Replicas = &replicas
	spec.GpusPerReplica = gpusPerReplica
	r.KaiwoService.Spec.CommonMetaSpec.Gpus = calculatedGpus

	var overrideDefaults bool
	if calculatedGpus > 0 && spec.RayService == nil {
		overrideDefaults = true
	} else {
		overrideDefaults = false
	}

	if spec.RayService == nil {
		if err := workloadutils.UpdatePodSpec(
			config,
			r.KaiwoService.Spec.CommonMetaSpec,
			labelContext,
			&rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template,
			r.KaiwoService.Name,
			replicas,
			gpusPerReplica,
			false,
			true,
		); err != nil {
			return nil, fmt.Errorf("failed to update head group spec: %w", err)
		}

		for i := range rayServiceSpec.RayClusterSpec.WorkerGroupSpecs {
			if err := workloadutils.UpdatePodSpec(
				config,
				r.KaiwoService.Spec.CommonMetaSpec,
				labelContext,
				&rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].Template,
				r.KaiwoService.Name,
				replicas,
				gpusPerReplica,
				overrideDefaults,
				false,
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

	if spec.ServeConfigV2 != "" {
		rayServiceSpec.ServeConfigV2 = spec.ServeConfigV2
	}

	rayService := &rayv1.RayService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template.ObjectMeta.Labels,
		},
		Spec: rayServiceSpec,
	}

	common.CopyLabels(r.KaiwoService.GetLabels(), &rayService.ObjectMeta)
	common.SetKaiwoSystemLabels(labelContext, &rayService.ObjectMeta)

	if r.KaiwoService.Spec.PriorityClass != "" {
		rayServiceSpec.RayClusterSpec.HeadGroupSpec.Template.Spec.PriorityClassName = r.KaiwoService.Spec.PriorityClass
		for i := range rayServiceSpec.RayClusterSpec.WorkerGroupSpecs {
			rayServiceSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec.PriorityClassName = r.KaiwoService.Spec.PriorityClass
		}
	}

	rayServiceSpecBytes, err := json.Marshal(rayServiceSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RayServiceSpec: %w", err)
	}

	appWrapper := &appwrapperv1beta2.AppWrapper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.KaiwoService.Name,
			Namespace: r.KaiwoService.Namespace,
			Labels: map[string]string{
				common.QueueLabel: r.KaiwoService.Labels[common.QueueLabel],
			},
		},
		Spec: appwrapperv1beta2.AppWrapperSpec{
			Components: []appwrapperv1beta2.AppWrapperComponent{
				{
					DeclaredPodSets: []appwrapperv1beta2.AppWrapperPodSet{
						{Replicas: baseutils.Pointer(int32(1)), Path: "template.spec.rayClusterConfig.headGroupSpec.template"},
						{Replicas: baseutils.Pointer(int32(replicas)), Path: "template.spec.rayClusterConfig.workerGroupSpecs[0].template"},
					},
					Template: runtime.RawExtension{
						Raw: []byte(fmt.Sprintf(`{
				    "apiVersion": "ray.io/v1",
				    "kind": "RayService",
				    "metadata": {
					"name": "%s",
					"namespace": "%s"
				    },
				    "spec": %s
				}`, r.KaiwoService.Name, r.KaiwoService.Namespace, rayServiceSpecBytes)),
					},
				},
			},
		},
	}

	return appWrapper, nil
}

func (r *RayServiceReconciler) GetEmptyObject() *appwrapperv1beta2.AppWrapper {
	return &appwrapperv1beta2.AppWrapper{}
}

func (r *RayServiceReconciler) ValidateBeforeCreateOrUpdate(ctx context.Context, actual *appwrapperv1beta2.AppWrapper) (*ctrl.Result, error) {
	// Abort reconciliation the managed label is set and actual doesn't exist, as the service is managed by the webhook
	// This is to avoid trying to create the service that is going to be created once the webhook completes
	return workloadutils.ValidateKaiwoResourceBeforeCreateOrUpdate(ctx, actual, r.KaiwoService.ObjectMeta)
}
