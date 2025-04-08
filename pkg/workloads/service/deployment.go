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

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	ctrl "sigs.k8s.io/controller-runtime"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

func GetDefaultDeploymentSpec(dangerous bool, resourceRequirements corev1.ResourceRequirements) appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: baseutils.Pointer(int32(1)),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{},
		},
		Template: workloadutils.GetPodTemplate(
			*resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
			dangerous,
			resourceRequirements,
			"workload",
		),
	}
}

type DeploymentReconciler struct {
	common.ResourceReconcilerBase[*appsv1.Deployment]
	KaiwoService *v1alpha1.KaiwoService
}

func NewDeploymentReconciler(svc *v1alpha1.KaiwoService) *DeploymentReconciler {
	reconciler := &DeploymentReconciler{
		ResourceReconcilerBase: common.ResourceReconcilerBase[*appsv1.Deployment]{
			ObjectKey: client.ObjectKeyFromObject(svc),
		},
		KaiwoService: svc,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *DeploymentReconciler) Build(ctx context.Context, _ client.Client) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx)

	svcSpec := r.KaiwoService.Spec
	labelContext := common.GetKaiwoLabelContext(r.KaiwoService)

	var depSpec appsv1.DeploymentSpec
	var overrideDefaults bool

	if svcSpec.Deployment == nil {
		depSpec = GetDefaultDeploymentSpec(
			svcSpec.Dangerous,
			baseutils.ValueOrDefault(svcSpec.Resources),
		)
		if r.KaiwoService.Spec.CommonMetaSpec.Gpus > 0 {
			overrideDefaults = true
		}
		if r.KaiwoService.Spec.CommonMetaSpec.Resources != nil {
			overrideDefaults = false
		}
	} else {
		depSpec = svcSpec.Deployment.Spec
		overrideDefaults = false
	}

	depSpec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways

	if depSpec.Template.ObjectMeta.Labels == nil {
		depSpec.Template.ObjectMeta.Labels = map[string]string{}
	}

	depSpec.Selector.MatchLabels["app"] = r.ObjectKey.Name
	depSpec.Template.ObjectMeta.Labels["app"] = r.ObjectKey.Name

	depSpec.Template.ObjectMeta.Labels[common.QueueLabel] = r.KaiwoService.Labels[common.QueueLabel]
	if r.KaiwoService.Spec.PriorityClass != "" {
		depSpec.Template.ObjectMeta.Labels[common.WorkloadPriorityClassLabel] = r.KaiwoService.Spec.PriorityClass
	}

	if svcSpec.Replicas != nil {
		depSpec.Replicas = baseutils.Pointer(int32(*svcSpec.Replicas))
	}

	gpus := r.KaiwoService.Spec.CommonMetaSpec.Gpus

	if err := workloadutils.UpdatePodSpec(
		r.KaiwoService.Spec.CommonMetaSpec,
		labelContext,
		&depSpec.Template,
		r.KaiwoService.Name,
		int(*depSpec.Replicas),
		gpus,
		overrideDefaults,
	); err != nil {
		return nil, fmt.Errorf("failed to update deployment template: %w", err)
	}

	if err := workloadutils.AddEntrypoint(
		svcSpec.EntryPoint,
		&depSpec.Template,
	); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    depSpec.Template.ObjectMeta.Labels,
		},
		Spec: depSpec,
	}

	common.CopyLabels(r.KaiwoService.GetLabels(), &dep.ObjectMeta)
	common.SetKaiwoSystemLabels(labelContext, &dep.ObjectMeta)

	logger.Info("Building Deployment for KaiwoService", "name", r.ObjectKey.Name)
	return dep, nil
}

func (r *DeploymentReconciler) GetEmptyObject() *appsv1.Deployment {
	return &appsv1.Deployment{}
}

func (r *DeploymentReconciler) ValidateBeforeCreateOrUpdate(ctx context.Context, actual *appsv1.Deployment) (*ctrl.Result, error) {
	// Abort reconciliation the managed label is set and actual doesn't exist, as the deployment is managed by the webhook
	// This is to avoid trying to create the deployment that is going to be created once the webhook completes
	return workloadutils.ValidateKaiwoResourceBeforeCreateOrUpdate(ctx, actual, r.KaiwoService.ObjectMeta)
}
