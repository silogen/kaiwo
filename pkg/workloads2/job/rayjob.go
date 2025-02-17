package workloadjob

import (

	//"context"
	//"fmt"
	//
	//workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"
	//
	//controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	//
	//rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	//corev1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/api/errors"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//ctrl "sigs.k8s.io/controller-runtime"
	//"sigs.k8s.io/controller-runtime/pkg/client"
	//"sigs.k8s.io/controller-runtime/pkg/log"
	//
	//kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/*
Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

type RayJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
}

func (k *RayJobCommand) Build() (client.Object, error) {
	return nil, nil
}

//
//func (r *KaiwoJobReconciler) reconcileRayJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
//	logger := log.FromContext(ctx)
//
//	var existingRayJob rayv1.RayJob
//	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingRayJob)
//	if err == nil {
//		logger.Info("RayJob already exists", "RayJob", existingRayJob.Name)
//		return ctrl.Result{}, nil
//	} else if !errors.IsNotFound(err) {
//		logger.Error(err, "Failed to get existing RayJob")
//		return ctrl.Result{}, err
//	}
//
//	rayCluster := kaiwoJob.Spec.RayClusterSpec
//	if rayCluster == nil {
//		logger.Info("RayClusterSpec is nil, using default RayClusterSpec", "KaiwoJob", kaiwoJob.Name)
//		rayCluster = &rayv1.RayClusterSpec{
//			RayVersion:              "2.9.0",
//			EnableInTreeAutoscaling: BoolPtr(false),
//			HeadGroupSpec: rayv1.HeadGroupSpec{
//				Template: corev1.PodTemplateSpec{
//					Spec: corev1.PodSpec{
//						RestartPolicy: corev1.RestartPolicyAlways,
//					},
//				},
//			},
//			WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
//				{
//					Replicas:  Int32Ptr(2),
//					GroupName: "default-worker-group",
//					Template: corev1.PodTemplateSpec{
//						Spec: corev1.PodSpec{
//							RestartPolicy: corev1.RestartPolicyAlways,
//						},
//					},
//				},
//			},
//		}
//	}
//
//	if err := FillRayClusterPodSpec(kaiwoJob, rayCluster); err != nil {
//		logger.Error(err, "Failed to fill RayClusterPodSpec")
//		return ctrl.Result{}, err
//	}
//
//	if kaiwoJob.Spec.EntryPoint == nil || *kaiwoJob.Spec.EntryPoint == "" {
//		return ctrl.Result{}, fmt.Errorf("KaiwoJob does not specify an EntryPoint")
//	}
//
//	rayJob := &rayv1.RayJob{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      kaiwoJob.Name,
//			Namespace: kaiwoJob.Namespace,
//			Labels:    kaiwoJob.Spec.Labels,
//		},
//		Spec: rayv1.RayJobSpec{
//			Entrypoint:     *kaiwoJob.Spec.EntryPoint,
//			RayClusterSpec: rayCluster,
//		},
//	}
//
//	// Attach storage to head pod
//	if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJob.Spec.RayClusterSpec.HeadGroupSpec.Template.Spec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
//		return ctrl.Result{}, err
//	}
//
//	// Attach storage to worker pods
//	for i := range rayJob.Spec.RayClusterSpec.WorkerGroupSpecs {
//		workerSpec := rayJob.Spec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec
//		if err := controllerutils.UpdatePodSpecStorage(ctx, &workerSpec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
//			return ctrl.Result{}, err
//		}
//		rayJob.Spec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec = workerSpec
//	}
//
//	if err := ctrl.SetControllerReference(kaiwoJob, rayJob, r.Scheme); err != nil {
//		logger.Error(err, "Failed to set controller reference")
//		return ctrl.Result{}, err
//	}
//
//	if err := r.Create(ctx, rayJob); err != nil {
//		logger.Error(err, "Failed to create RayJob")
//		return ctrl.Result{}, err
//	}
//
//	logger.Info("Successfully created RayJob", "RayJob", rayJob.Name)
//	return ctrl.Result{}, nil
//}
