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

	appwrapperv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"k8s.io/apimachinery/pkg/runtime"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (handler *DeploymentHandler) BuildDesired(ctx context.Context, clusterCtx common.ClusterContext) (client.Object, error) {
	desiredDeployment, err := handler.buildDeploymentTemplate(ctx, clusterCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to build desired deployment: %w", err)
	}

	specBytes, err := json.Marshal(desiredDeployment.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Deployment spec: %w", err)
	}
	labelsBytes, err := json.Marshal(desiredDeployment.ObjectMeta.Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Deployment labels: %w", err)
	}

	// 3) initialize the AppWrapper
	applicationWrapper := handler.GetInitializedObject().(*appwrapperv1beta2.AppWrapper)
	applicationWrapper.Labels = map[string]string{
		common.QueueLabel: common.GetClusterQueueName(ctx, handler),
	}
	if priorityClass := handler.GetCommonSpec().WorkloadPriorityClass; priorityClass != "" {
		applicationWrapper.Labels[common.WorkloaddPriorityClassLabel] = priorityClass
	}

	// 4) configure the wrapper to suspend until Kueue admits all replicas
	applicationWrapper.Spec = appwrapperv1beta2.AppWrapperSpec{
		Suspend: true,
		Components: []appwrapperv1beta2.AppWrapperComponent{{
			DeclaredPodSets: []appwrapperv1beta2.AppWrapperPodSet{{
				Replicas: desiredDeployment.Spec.Replicas,
				Path:     "template.spec.template",
			}},
			Template: runtime.RawExtension{
				Raw: []byte(fmt.Sprintf(`{
  "apiVersion": "apps/v1",
  "kind":       "Deployment",
  "metadata": {
    "name":      %q,
    "namespace": %q,
    "labels":    %s
  },
  "spec": %s
}`, desiredDeployment.Name,
					desiredDeployment.Namespace,
					labelsBytes,
					specBytes)),
			},
		}},
	}

	// 5) propagate KaiwoService labels onto the AppWrapper itself
	common.UpdateLabels(handler.KaiwoService, &applicationWrapper.ObjectMeta)

	return applicationWrapper, nil
}

func (handler *DeploymentHandler) buildDeploymentTemplate(
	ctx context.Context,
	clusterContext common.ClusterContext,
) (*appsv1.Deployment, error) {
	kaiwoService := handler.KaiwoService
	configuration := common.ConfigFromContext(ctx)

	var deploymentSpec appsv1.DeploymentSpec
	if kaiwoService.Spec.Deployment == nil {
		deploymentSpec = GetDefaultDeploymentSpec(
			configuration,
			kaiwoService.Spec.Dangerous,
		)
	} else {
		deploymentSpec = kaiwoService.Spec.Deployment.Spec
	}

	deploymentSpec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways

	if deploymentSpec.Selector == nil {
		deploymentSpec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{},
		}
	}
	deploymentSpec.Selector.MatchLabels["app"] = kaiwoService.Name

	gpuSchedulingResult, err := common.CalculateGpuRequirements(
		ctx, clusterContext,
		kaiwoService.Spec.GpuResources,
		kaiwoService.Spec.Replicas,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate GPU requirements: %w", err)
	}
	if kaiwoService.Spec.Replicas != nil {
		deploymentSpec.Replicas = baseutils.Pointer(int32(*gpuSchedulingResult.Replicas))
	}

	if err := common.AddEntrypoint(
		kaiwoService.Spec.EntryPoint,
		&deploymentSpec.Template,
	); err != nil {
		return nil, fmt.Errorf("failed to add entrypoint: %w", err)
	}

	common.UpdatePodTemplateSpecNonRay(configuration, handler, gpuSchedulingResult, &deploymentSpec.Template)

	deploymentSpec.Template.ObjectMeta.Labels["app"] = kaiwoService.Name
	deploymentSpec.Template.ObjectMeta.Labels[common.KaiwoRunIdLabel] = string(kaiwoService.UID)
	deploymentSpec.Template.ObjectMeta.Labels[common.QueueLabel] = common.GetClusterQueueName(ctx, handler)

	desiredDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoService.Name,
			Namespace: kaiwoService.Namespace,
			Labels:    deploymentSpec.Template.ObjectMeta.Labels,
		},
		Spec: deploymentSpec,
	}

	return desiredDeployment, nil
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
	deployment := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), deployment); err != nil {
		if !errors.IsNotFound(err) {
			return nil, nil, fmt.Errorf("failed to get deployment: %w", err)
		}
		return baseutils.Pointer(kaiwo.WorkloadStatusStarting), nil, nil
	}
	// Check for a “Progressing=False / ProgressDeadlineExceeded”
	for _, c := range deployment.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing &&
			c.Status == corev1.ConditionFalse &&
			c.Reason == "ProgressDeadlineExceeded" {
			return baseutils.Pointer(kaiwo.WorkloadStatusFailed), nil, nil
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
