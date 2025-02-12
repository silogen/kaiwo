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

package controller

import (
	"context"
	"fmt"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// KaiwoJobReconciler reconciles a KaiwoJob object
type KaiwoJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs/finalizers,verbs=update

func (r *KaiwoJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the KaiwoJob instance
	var kaiwoJob kaiwov1alpha1.KaiwoJob
	if err := r.Get(ctx, req.NamespacedName, &kaiwoJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get KaiwoJob")
			return ctrl.Result{}, err
		}
		logger.Info("Job does not exist, it might have been deleted")
		return ctrl.Result{}, nil
	}

	if err := controllerutils.ReconcileStorage(r.Client, r.Scheme, ctx, &kaiwoJob, &kaiwoJob.Spec.Storage); err != nil {
		return ctrl.Result{}, err
	}

	result, err := controllerutils.ReconcileDownloadJob(r.Client, r.Scheme, ctx, &kaiwoJob, &kaiwoJob.Spec.Storage)
	if err != nil {
		return ctrl.Result{}, err
	}
	// If the download job has not completed, a re-reconciliation request may be returned
	if result != nil {
		return *result, nil
	}

	if kaiwoJob.Spec.Image == "" {
		kaiwoJob.Spec.Image = baseutils.DefaultRayImage
		if err := r.Update(ctx, &kaiwoJob); err != nil {
			logger.Error(err, "Failed to update KaiwoJob with default image")
			return ctrl.Result{}, err
		}
	}

	if kaiwoJob.Spec.RayClusterSpec == nil || !kaiwoJob.Spec.Ray {
		return r.reconcileK8sJob(ctx, &kaiwoJob)
	} else if kaiwoJob.Spec.RayClusterSpec != nil || kaiwoJob.Spec.Ray {
		return r.reconcileRayJob(ctx, &kaiwoJob)
	}

	jobInvalidErr := fmt.Errorf("KaiwoJob does not specify a valid Job or RayJob")
	logger.Error(jobInvalidErr, "KaiwoJob is misconfigured", "KaiwoJob", kaiwoJob.Name)
	return ctrl.Result{}, jobInvalidErr
}

func (r *KaiwoJobReconciler) reconcileK8sJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var existingJob batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingJob)
	if err == nil {
		logger.Info("Kubernetes Job already exists", "Job", existingJob.Name)
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing Job")
		return ctrl.Result{}, err
	}

	jobSpec := kaiwoJob.Spec.JobSpec
	if jobSpec == nil {
		logger.Info("JobSpec is nil, using default JobSpec", "KaiwoJob", kaiwoJob.Name)

		jobSpec = &batchv1.JobSpec{
			TTLSecondsAfterFinished: Int32Ptr(43200),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		}

		if err := FillPodSpec(kaiwoJob, &jobSpec.Template.Spec); err != nil {
			logger.Error(err, "Failed to fill PodSpec")
			return ctrl.Result{}, err
		}
	}

	if kaiwoJob.Labels == nil {
		kaiwoJob.Labels = make(map[string]string)
	}

	kaiwoJob.Labels[kaiwov1alpha1.UserLabel] = baseutils.SanitizeStringForKubernetes(kaiwoJob.Spec.User)
	kaiwoJob.Labels[kaiwov1alpha1.QueueLabel] = kaiwoJob.Spec.ClusterQueue

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: *jobSpec,
	}

	if err := controllerutils.UpdatePodSpecStorage(ctx, &job.Spec.Template.Spec, kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
		return ctrl.Result{}, err
	}

	if err := ctrl.SetControllerReference(kaiwoJob, job, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, job); err != nil {
		logger.Error(err, "Failed to create Job")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created Kubernetes Job", "Job", job.Name)
	return ctrl.Result{}, nil
}

func (r *KaiwoJobReconciler) reconcileRayJob(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var existingRayJob rayv1.RayJob
	err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &existingRayJob)
	if err == nil {
		logger.Info("RayJob already exists", "RayJob", existingRayJob.Name)
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing RayJob")
		return ctrl.Result{}, err
	}

	rayCluster := kaiwoJob.Spec.RayClusterSpec
	if rayCluster == nil {
		logger.Info("RayClusterSpec is nil, using default RayClusterSpec", "KaiwoJob", kaiwoJob.Name)
		rayCluster = &rayv1.RayClusterSpec{
			RayVersion:              "2.9.0",
			EnableInTreeAutoscaling: BoolPtr(false),
			HeadGroupSpec: rayv1.HeadGroupSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyAlways,
					},
				},
			},
			WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
				{
					Replicas:  Int32Ptr(2),
					GroupName: "default-worker-group",
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyAlways,
						},
					},
				},
			},
		}
	}

	if err := FillRayClusterPodSpec(kaiwoJob, rayCluster); err != nil {
		logger.Error(err, "Failed to fill RayClusterPodSpec")
		return ctrl.Result{}, err
	}
	rayJob := &rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: rayv1.RayJobSpec{
			Entrypoint:     kaiwoJob.Spec.EntryPoint,
			RayClusterSpec: rayCluster,
		},
	}

	// Attach storage to head pod
	if err := controllerutils.UpdatePodSpecStorage(ctx, &rayJob.Spec.RayClusterSpec.HeadGroupSpec.Template.Spec, kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
		return ctrl.Result{}, err
	}

	// Attach storage to worker pods
	for i := range rayJob.Spec.RayClusterSpec.WorkerGroupSpecs {
		workerSpec := rayJob.Spec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec
		if err := controllerutils.UpdatePodSpecStorage(ctx, &workerSpec, kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
			return ctrl.Result{}, err
		}
		rayJob.Spec.RayClusterSpec.WorkerGroupSpecs[i].Template.Spec = workerSpec
	}

	if err := ctrl.SetControllerReference(kaiwoJob, rayJob, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, rayJob); err != nil {
		logger.Error(err, "Failed to create RayJob")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created RayJob", "RayJob", rayJob.Name)
	return ctrl.Result{}, nil
}

func (r *KaiwoJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwov1alpha1.KaiwoJob{}).
		Named("kaiwojob").
		Complete(r)
}
