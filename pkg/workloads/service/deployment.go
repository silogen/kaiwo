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

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"k8s.io/apimachinery/pkg/runtime"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

func GetDefaultDeploymentSpec(config common.KaiwoConfigContext, dangerous bool) appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: baseutils.Pointer(int32(1)),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{},
		},
		Template: common.GetPodTemplate(
			config,
			*resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
			dangerous,
			"workload",
		),
	}
}

type DeploymentHandler struct {
	KaiwoService *kaiwo.KaiwoService
	Scheme       *runtime.Scheme
}

func (handler *DeploymentHandler) GetKaiwoWorkloadObject() client.Object {
	return handler.KaiwoService
}

func (handler *DeploymentHandler) GetCommonSpec() kaiwo.CommonMetaSpec {
	return handler.KaiwoService.Spec.CommonMetaSpec
}

func (handler *DeploymentHandler) GetCommonStatusSpec() *kaiwo.CommonStatusSpec {
	return &handler.KaiwoService.Status.CommonStatusSpec
}

func (handler *DeploymentHandler) GetInitializedObject() client.Object {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      handler.KaiwoService.Name,
			Namespace: handler.KaiwoService.Namespace,
			Labels:    map[string]string{},
		},
	}
}

func (handler *DeploymentHandler) BuildDesired(ctx context.Context, clusterCtx common.ClusterContext) (client.Object, error) {
	logger := log.FromContext(ctx)
	config := common.ConfigFromContext(ctx)

	svc := handler.KaiwoService
	svcSpec := svc.Spec

	var depSpec appsv1.DeploymentSpec

	if svcSpec.Deployment == nil {
		depSpec = GetDefaultDeploymentSpec(
			config,
			svcSpec.Dangerous,
		)
	} else {
		depSpec = svcSpec.Deployment.Spec
	}

	depSpec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways

	depSpec.Selector.MatchLabels["app"] = svc.Name

	resourceConfig := common.CalculateResourceConfig(ctx, clusterCtx, handler.KaiwoService, true)

	if svcSpec.Replicas != nil {
		depSpec.Replicas = baseutils.Pointer(int32(resourceConfig.Replicas))
	}

	if err := common.AddEntrypoint(
		func() string {
			if svcSpec.Deployment != nil {
				return svcSpec.Deployment.EntryPoint
			}
			return ""
		}(),
		&depSpec.Template,
	); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	common.UpdatePodSpec(config, handler.KaiwoService, resourceConfig, &depSpec.Template, false)

	depSpec.Template.Labels["app"] = svc.Name
	depSpec.Template.Labels[common.QueueLabel] = common.GetClusterQueueName(ctx, handler)
	if priorityclass := handler.GetCommonSpec().WorkloadPriorityClass; priorityclass != "" {
		depSpec.Template.Labels[common.WorkloaddPriorityClassLabel] = priorityclass
	}

	dep := handler.GetInitializedObject().(*appsv1.Deployment)
	dep.Labels = depSpec.Template.Labels
	dep.Spec = depSpec

	common.UpdateLabels(handler.KaiwoService, &dep.ObjectMeta)

	return dep, nil
}

func (handler *DeploymentHandler) MutateActual(ctx context.Context, clusterCtx common.ClusterContext, actual client.Object) error {
	// TODO
	return nil
}

func (handler *DeploymentHandler) GetActual(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	objectKey := client.ObjectKeyFromObject(handler.KaiwoService)
	batchJob := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, objectKey, batchJob); err != nil {
		return nil, err
	}
	return batchJob, nil
}

func (handler *DeploymentHandler) ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	deployment := obj.(*appsv1.Deployment)
	// Check for a “Progressing=False / ProgressDeadlineExceeded”
	for _, c := range deployment.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing &&
			c.Status == corev1.ConditionFalse &&
			c.Reason == "ProgressDeadlineExceeded" {
			return baseutils.Pointer(kaiwo.WorkloadStatusError), nil, nil
		}
	}

	// None of the desired replicas have been observed yet (the controller hasn’t rolled out the latest spec)
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
	}

	// If we see fewer AvailableReplicas than we asked for
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if deployment.Status.AvailableReplicas < desired {
		return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
	}

	// Deployment exists and is running
	return baseutils.Pointer(kaiwo.WorkloadStatusRunning), nil, nil
}

func (handler *DeploymentHandler) GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error) {
	podList := &corev1.PodList{}

	if err := k8sClient.List(ctx, podList, client.MatchingLabels{
		common.KaiwoRunIdLabel: string(handler.KaiwoService.UID),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var workloads []kueuev1beta1.Workload

	for _, pod := range podList.Items {
		workload, err := common.GetKueueWorkload(ctx, k8sClient, pod.Namespace, string(pod.UID))
		if err != nil {
			return nil, fmt.Errorf("failed to get workload: %w", err)
		}
		if workload == nil {
			continue
		}
		workloads = append(workloads, *workload)
	}
	return workloads, nil
}
