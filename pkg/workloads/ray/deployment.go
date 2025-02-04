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

package ray

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/workloads"
)

type Deployment struct{}

type DeploymentFlags struct {
	Serveconfig string
}

//go:embed deployment.yaml.tmpl
var DeploymentTemplate []byte

const ServeconfigFilename = "serveconfig"

func (deployment Deployment) GenerateTemplateContext(execFlags workloads.ExecFlags) (any, error) {
	contents, err := os.ReadFile(execFlags.WorkloadFiles[ServeconfigFilename])
	if err != nil {
		return nil, fmt.Errorf("failed to read serveconfig file: %w", err)
	}

	return DeploymentFlags{Serveconfig: strings.TrimSpace(string(contents))}, nil
}

//func (deployment Deployment) BuildObject(flags workloads.WorkloadTemplateConfig) (client.Object, error) {
//	obj := &rayv1.RayService{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      flags.Meta.Name,
//			Namespace: flags.Meta.Namespace,
//			Labels: map[string]string{
//				"kaiwo-cli/username": flags.Meta.User,
//			},
//		},
//		Spec: rayv1.RayServiceSpec{
//			ServeConfigV2: "",
//			RayClusterSpec: rayv1.RayClusterSpec{
//				// EnableInTreeAutoscaling: true,
//				HeadGroupSpec: rayv1.HeadGroupSpec{
//					RayStartParams: map[string]string{
//						"dashboard-host": "0.0.0.0",
//					},
//					Template: corev1.PodTemplateSpec{},
//				},
//			},
//		},
//	}
//	return obj, nil
//}
//
//func buildPodSpec(metaFlags workloads.MetaFlags, ports []corev1.ContainerPort) corev1.PodSpec {
//	return corev1.PodSpec{}
//}

func (deployment Deployment) ConvertObject(object runtime.Object) (client.Object, bool) {
	obj, ok := object.(*rayv1.RayService)

	return obj, ok
}

func (deployment Deployment) DefaultTemplate() ([]byte, error) {
	if DeploymentTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return DeploymentTemplate, nil
}

//func (service Deployment) GenerateName() string {
//	return utils.BuildWorkloadName(service.Shared.Name, service.Shared.Path, service.Deployment.Image)
//}

func (deployment Deployment) IgnoreFiles() []string {
	return []string{ServeconfigFilename, workloads.KaiwoconfigFilename, workloads.EnvFilename, workloads.TemplateFileName}
}

func (deployment Deployment) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (deployment Deployment) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (deployment Deployment) GenerateAdditionalResourceManifests(_ client.Client, _ workloads.WorkloadTemplateConfig) ([]runtime.Object, error) {
	return []runtime.Object{}, nil
}

func (deployment Deployment) BuildReference(ctx context.Context, k8sClient client.Client, key client.ObjectKey) (workloads.WorkloadReference, error) {
	obj := &rayv1.RayService{}
	if err := k8sClient.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("could not get job: %w", err)
	}
	deploymentRef := &ServiceReference{
		RayService: *obj,
		WorkloadReferenceBase: workloads.WorkloadReferenceBase{
			WorkloadObject: obj,
		},
	}
	if err := deploymentRef.Load(ctx, k8sClient); err != nil {
		return nil, fmt.Errorf("could not load service: %w", err)
	}
	return deploymentRef, nil
}

type ServiceReference struct {
	workloads.WorkloadReferenceBase
	RayService rayv1.RayService
	RayCluster *rayv1.RayCluster
	HeadPod    *corev1.Pod
	WorkerPods []*corev1.Pod
}

func (serviceRef *ServiceReference) Load(ctx context.Context, k8sClient client.Client) error {
	// Fetch ray cluster

	clusterLabelSelector := client.MatchingLabels{
		"ray.io/originated-from-cr-name": serviceRef.RayService.Name,
		"ray.io/originated-from-crd":     "RayService",
	}

	logrus.Debugln(clusterLabelSelector)

	clusterList := &rayv1.RayClusterList{}
	if err := k8sClient.List(ctx, clusterList, clusterLabelSelector); err != nil {
		return fmt.Errorf("could not list clusters: %w", err)
	}
	if len(clusterList.Items) == 0 {
		return fmt.Errorf("no clusters found")
	}
	if len(clusterList.Items) > 1 {
		return fmt.Errorf("more than one clusters found")
	}
	serviceRef.RayCluster = &clusterList.Items[0]

	clusterPodLabelSelector := client.MatchingLabels{
		"ray.io/cluster": serviceRef.RayCluster.Name,
	}

	clusterPodList := &corev1.PodList{}
	if err := k8sClient.List(ctx, clusterPodList, clusterPodLabelSelector); err != nil {
		return fmt.Errorf("could not list cluster pods: %w", err)
	}

	// Clear
	serviceRef.HeadPod = nil
	serviceRef.WorkerPods = []*corev1.Pod{}

	for _, pod := range clusterPodList.Items {
		nodeType := pod.Labels["ray.io/node-type"]
		if nodeType == "worker" {
			serviceRef.WorkerPods = append(serviceRef.WorkerPods, &pod)
		} else if nodeType == "head" {
			if serviceRef.HeadPod == nil {
				serviceRef.HeadPod = &pod
			} else {
				logrus.Warn("More than one head pod encountered")
			}
		} else {
			logrus.Warnf("Encountered unknown node type: %s", nodeType)
		}
	}

	return nil
}

func (serviceRef *ServiceReference) GetPods() []workloads.WorkloadPod {
	var pods []workloads.WorkloadPod

	if serviceRef.HeadPod != nil {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *serviceRef.HeadPod,
			LogicalGroup: "head",
		})
	} else {
		logrus.Debug("No head pod")
	}

	for _, pod := range serviceRef.WorkerPods {
		pods = append(pods, workloads.WorkloadPod{
			Pod:          *pod,
			LogicalGroup: "worker",
		})
	}
	return pods
}

func (serviceRef *ServiceReference) GetStatus() string {
	return "N/A (TODO)"
}

func (serviceRef *ServiceReference) GetServices(ctx context.Context, k8sClient client.Client) ([]corev1.Service, error) {
	if serviceRef.RayCluster == nil {
		return []corev1.Service{}, fmt.Errorf("no ray cluster set, run ServiceReference.Load() first")
	}

	serviceLabelSelector := client.MatchingLabels{
		"ray.io/cluster": serviceRef.RayCluster.Name,
	}
	serviceList := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, serviceList, serviceLabelSelector); err != nil {
		return nil, fmt.Errorf("could not list services: %w", err)
	}

	return serviceList.Items, nil
}
