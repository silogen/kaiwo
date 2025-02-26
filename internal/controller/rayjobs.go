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

package controller

import (
	"context"
	"strings"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

var DefaultRayJobSpec = rayv1.RayJobSpec{
	ShutdownAfterJobFinishes: true,
	RayClusterSpec: &rayv1.RayClusterSpec{
		EnableInTreeAutoscaling: controllerutils.BoolPtr(false),
		HeadGroupSpec: rayv1.HeadGroupSpec{
			RayStartParams: map[string]string{},
			Template:       controllerutils.GetPodTemplate(resource.MustParse("1Gi")),
		},
		WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
			{
				GroupName:      "default-worker-group",
				Replicas:       controllerutils.Int32Ptr(1),
				MinReplicas:    controllerutils.Int32Ptr(1),
				MaxReplicas:    controllerutils.Int32Ptr(1),
				RayStartParams: map[string]string{},
				Template:       controllerutils.GetPodTemplate(resource.MustParse("200Gi")), // TODO: add to CRD as configurable field
			},
		},
	},
}

func (r *KaiwoJobReconciler) reconcileRayJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var existingRayJob rayv1.RayJob
	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingRayJob)
	if err == nil {
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing RayJob")
		return ctrl.Result{}, err
	}

	kaiwoJob.Spec.Ray = true

	// Use DefaultRayJobSpec if none is provided
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

	// Apply entrypoint
	if kaiwoJob.Spec.EntryPoint != "" {
		rayJobSpec.Entrypoint = kaiwoJob.Spec.EntryPoint
	}

	replicas, gpusPerReplica, err := controllerutils.CalculateNumberOfReplicas(ctx, r.Client, strings.ToLower(kaiwoJob.Spec.GpuVendor), kaiwoJob.Spec.Gpus, kaiwoJob.Spec.Replicas, kaiwoJob.Spec.GpusPerReplica, true)
	if err != nil {
		return ctrl.Result{}, err
	}

	kaiwoJob.Spec.Replicas = replicas
	kaiwoJob.Spec.GpusPerReplica = gpusPerReplica

	// Adjust resource requests & limits
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Replicas = controllerutils.Int32Ptr(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MinReplicas = controllerutils.Int32Ptr(int32(replicas))
		rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].MaxReplicas = controllerutils.Int32Ptr(int32(replicas))
		if err := controllerutils.AdjustResourceRequestsAndLimits(ctx, kaiwoJob.Spec.GpuVendor, kaiwoJob.Spec.Gpus, kaiwoJob.Spec.Replicas, kaiwoJob.Spec.GpusPerReplica, &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Add environment variables
	if err := controllerutils.AddEnvVars(ctx, kaiwoJob.Spec.Env, &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template); err != nil {
		return ctrl.Result{}, err
	}
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if err := controllerutils.AddEnvVars(ctx, kaiwoJob.Spec.Env, &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Attach storage to head pod
	if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJobSpec.RayClusterSpec.HeadGroupSpec.Template.Spec, kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
		return ctrl.Result{}, err
	}

	// Attach storage to worker pods
	for i := range rayJobSpec.RayClusterSpec.WorkerGroupSpecs {
		if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJobSpec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec, kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Construct the RayJob object
	rayJob := &rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Labels,
		},
		Spec: *rayJobSpec.DeepCopy(),
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(kaiwoJob, rayJob, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	// Create the RayJob
	if err := r.Create(ctx, rayJob); err != nil {
		logger.Error(err, "Failed to create RayJob")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created RayJob", "RayJob", rayJob.Name)
	return ctrl.Result{}, nil
}
