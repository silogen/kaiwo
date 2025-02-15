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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		return ctrl.Result{}, nil
	}

	// **Update Status based on RayJob, Kubernetes Job, or Pods**
	if err := r.updateKaiwoJobStatus(ctx, &kaiwoJob); err != nil {
		logger.Error(err, "Failed to update KaiwoJob status")
		return ctrl.Result{}, err
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

	if kaiwoJob.Spec.Labels == nil {
		kaiwoJob.Spec.Labels = make(map[string]string)
	}

	kaiwoJob.Spec.Labels[kaiwov1alpha1.UserLabel] = baseutils.SanitizeStringForKubernetes(kaiwoJob.Spec.User)
	if kaiwoJob.Spec.ClusterQueue == "" {
		kaiwoJob.Spec.Labels[kaiwov1alpha1.QueueLabel] = DefaultKaiwoQueueConfigName
	} else {
		kaiwoJob.Spec.Labels[kaiwov1alpha1.QueueLabel] = kaiwoJob.Spec.ClusterQueue
	}

	if err := controllerutils.CreateLocalQueue(ctx, r.Client, kaiwoJob.Spec.Labels[kaiwov1alpha1.QueueLabel], kaiwoJob.ObjectMeta.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	if kaiwoJob.Spec.RayJob == nil || !kaiwoJob.Spec.Ray {
		return r.reconcileK8sJob(ctx, &kaiwoJob)
	} else if kaiwoJob.Spec.RayJob != nil || kaiwoJob.Spec.Ray {
		return r.reconcileRayJob(ctx, &kaiwoJob)
	}

	jobInvalidErr := fmt.Errorf("KaiwoJob does not specify a valid Job or RayJob")
	logger.Error(jobInvalidErr, "KaiwoJob is misconfigured", "KaiwoJob", kaiwoJob.Name)
	return ctrl.Result{}, jobInvalidErr
}

func (r *KaiwoJobReconciler) updateKaiwoJobStatus(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) error {
	logger := log.FromContext(ctx)

	jobSucceeded, jobFailed, err := r.checkJobCompletion(ctx, kaiwoJob)
	if err != nil {
		return err
	}

	if kaiwoJob.Status.StartTime == nil {
		if startTime := r.getEarliestPodStartTime(ctx, kaiwoJob); startTime != nil {
			kaiwoJob.Status.StartTime = startTime
			logger.Info("Set KaiwoJob StartTime", "KaiwoJob", kaiwoJob.Name, "StartTime", startTime)
		}
	}

	if jobSucceeded || jobFailed {
		return r.handleJobCompletion(ctx, kaiwoJob, jobSucceeded)
	}

	startTimeUpdated, err := r.checkPodStatus(ctx, kaiwoJob)
	if err != nil {
		return err
	}

	if startTimeUpdated && kaiwoJob.Status.CompletionTime != nil {
		kaiwoJob.Status.Duration = int64(kaiwoJob.Status.CompletionTime.Time.Sub(kaiwoJob.Status.StartTime.Time).Seconds())
	}

	if err := r.Status().Update(ctx, kaiwoJob); err != nil {
		logger.Error(err, "Failed to update KaiwoJob status")
		return err
	}

	logger.Info("Updated KaiwoJob status", "KaiwoJob", kaiwoJob.Name, "Status", kaiwoJob.Status.Status)
	return nil
}

func (r *KaiwoJobReconciler) checkJobCompletion(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (bool, bool, error) {
	logger := log.FromContext(ctx)
	var jobSucceeded, jobFailed bool

	if !kaiwoJob.Spec.Ray {
		var job batchv1.Job
		err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &job)
		if err == nil {
			jobFailed = job.Status.Failed > 0
			jobSucceeded = job.Status.Succeeded > 0
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get Kubernetes Job status")
			return false, false, err
		}
	} else {
		var rayJob rayv1.RayJob
		err := r.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &rayJob)
		if err == nil {
			switch rayJob.Status.JobStatus {
			case rayv1.JobStatusFailed:
				jobFailed = true
			case rayv1.JobStatusSucceeded:
				jobSucceeded = true
			}
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get RayJob status")
			return false, false, err
		}
	}

	return jobSucceeded, jobFailed, nil
}

func (r *KaiwoJobReconciler) handleJobCompletion(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob, jobSucceeded bool) error {
	logger := log.FromContext(ctx)
	now := metav1.Now()

	if jobSucceeded {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusComplete
	} else {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusFailed
	}

	kaiwoJob.Status.CompletionTime = &now

	if kaiwoJob.Status.StartTime != nil {
		kaiwoJob.Status.Duration = int64(kaiwoJob.Status.CompletionTime.Time.Sub(kaiwoJob.Status.StartTime.Time).Seconds())
	}

	if err := r.Status().Update(ctx, kaiwoJob); err != nil {
		logger.Error(err, "Failed to update KaiwoJob status on completion")
		return err
	}

	logger.Info("Updated KaiwoJob on completion", "KaiwoJob", kaiwoJob.Name, "Status", kaiwoJob.Status.Status)
	return nil
}

func (r *KaiwoJobReconciler) checkPodStatus(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) (bool, error) {
	logger := log.FromContext(ctx)

	podList := &corev1.PodList{}
	err := r.List(ctx, podList, client.InNamespace(kaiwoJob.Namespace), client.MatchingLabels{"job-name": kaiwoJob.Name})
	if err != nil {
		logger.Error(err, "Failed to list pods for job", "KaiwoJob", kaiwoJob.Name)
		return false, err
	}

	var runningPods, pendingPods []corev1.Pod
	var earliestRunningTime *metav1.Time

	for _, pod := range podList.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			runningPods = append(runningPods, pod)
			if pod.Status.StartTime != nil {
				if earliestRunningTime == nil || pod.Status.StartTime.Before(earliestRunningTime) {
					earliestRunningTime = pod.Status.StartTime
				}
			}
		case corev1.PodPending:
			pendingPods = append(pendingPods, pod)
		}
	}

	if earliestRunningTime != nil && kaiwoJob.Status.StartTime == nil {
		kaiwoJob.Status.StartTime = earliestRunningTime
		logger.Info("Set KaiwoJob StartTime", "KaiwoJob", kaiwoJob.Name, "StartTime", earliestRunningTime)
		return true, nil
	}

	if len(runningPods) > 0 {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusRunning
	} else if len(pendingPods) > 0 {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusStarting
	} else {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusPending
	}

	return false, nil
}

func (r *KaiwoJobReconciler) getEarliestPodStartTime(ctx context.Context, kaiwoJob *kaiwov1alpha1.KaiwoJob) *metav1.Time {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(kaiwoJob.Namespace), client.MatchingLabels{"job-name": kaiwoJob.Name}); err != nil {
		return nil
	}

	var earliestStartTime *metav1.Time
	for _, pod := range podList.Items {
		if pod.Status.StartTime != nil {
			if earliestStartTime == nil || pod.Status.StartTime.Before(earliestStartTime) {
				earliestStartTime = pod.Status.StartTime
			}
		}
	}
	return earliestStartTime
}

func (r *KaiwoJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwov1alpha1.KaiwoJob{}).
		Watches(
			&batchv1.Job{}, // Watching batchv1.Job
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return r.mapJobToKaiwoJob(obj)
			}),
		).
		Watches(
			&rayv1.RayJob{}, // Watching RayJob
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return r.mapRayJobToKaiwoJob(obj)
			}),
		).
		Named("kaiwojob").
		Complete(r)
}

func (r *KaiwoJobReconciler) mapJobToKaiwoJob(obj client.Object) []reconcile.Request {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{
			Name:      job.Name,
			Namespace: job.Namespace,
		}},
	}
}

func (r *KaiwoJobReconciler) mapRayJobToKaiwoJob(obj client.Object) []reconcile.Request {
	rayJob, ok := obj.(*rayv1.RayJob)
	if !ok {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{
			Name:      rayJob.Name,
			Namespace: rayJob.Namespace,
		}},
	}
}
