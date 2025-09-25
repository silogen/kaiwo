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
	"strings"

	"k8s.io/client-go/tools/record"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ModelCacheReconciler reconciles a ModelCache object
type ModelCacheReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// RBAC markers
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=modelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=modelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=modelcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

const (
	// Placeholder image for the download job; replace in a future iteration
	downloadJobImage = "ghcr.io/example/model-downloader:latest"
)

func (r *ModelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	// Fetch CR
	var mc kaiwo.ModelCache
	if err := r.Get(ctx, req.NamespacedName, &mc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Observe
	ob, err := r.observe(ctx, &mc)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("observation failed: %w", err)
	}

	// Deletion guard: don't mutate children when deleting
	if !mc.DeletionTimestamp.IsZero() {
		st := r.projectStatus(&mc, ob)
		meta.SetStatusCondition(&st.Conditions, metav1.Condition{Type: kaiwo.ConditionReady, Status: metav1.ConditionFalse, Reason: "Finalizing", Message: "Resource is being deleted"})
		before := mc.DeepCopy()
		mc.Status = st
		_ = r.Status().Patch(ctx, &mc, client.MergeFrom(before))
		logger.Info("finalizing ModelCache; skipping child mutations", "name", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Apply (ensure PVC and Job)
	if err := r.apply(ctx, &mc, ob); err != nil {
		// classify and project failure, then return error for backoff
		reason, message, terminal := classifyApplyError(err)
		ob.applyErrorReason = reason
		ob.applyErrorMessage = message
		ob.applyTerminal = terminal
		st := r.projectStatus(&mc, ob)
		before := mc.DeepCopy()
		mc.Status = st
		_ = r.Status().Patch(ctx, &mc, client.MergeFrom(before))
		r.Recorder.Event(&mc, corev1.EventTypeWarning, reason, message)
		logger.Error(err, "apply failed", "reason", reason)
		return ctrl.Result{}, fmt.Errorf("apply failed: %w", err)
	}

	// Emit events based on observed state prior to apply
	prevStorage := meta.FindStatusCondition(mc.Status.Conditions, kaiwo.ConditionStorageReady)
	prevReady := meta.FindStatusCondition(mc.Status.Conditions, kaiwo.ConditionReady)
	if !ob.pvcFound {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "PVCEnsured", fmt.Sprintf("Ensured PVC %s", r.pvcName(&mc)))
		logger.Info("PVC ensured (SSA)", "pvc", r.pvcName(&mc))
	}
	if ob.storageReady && (prevStorage == nil || prevStorage.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "PVCBound", fmt.Sprintf("PVC %s is Bound", r.pvcName(&mc)))
		logger.Info("PVC bound", "pvc", r.pvcName(&mc))
	}
	if ob.storageReady && !ob.jobFound {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "DownloadJobEnsured", fmt.Sprintf("Ensured download Job %s", r.jobName(&mc)))
		logger.Info("Download job ensured (SSA)", "job", r.jobName(&mc))
	}
	if ob.jobSucceeded && (prevReady == nil || prevReady.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "DownloadJobCompleted", "Download job completed successfully")
		logger.Info("Download job completed")
	}
	if ob.jobFailed {
		r.Recorder.Event(&mc, corev1.EventTypeWarning, "DownloadJobFailed", "Download job failed")
		logger.Info("Download job failed")
	}

	// Status projection and patch
	newStatus := r.projectStatus(&mc, ob)
	changed := !conditionsEqual(newStatus.Conditions, mc.Status.Conditions) ||
		newStatus.Status != mc.Status.Status ||
		newStatus.PersistentVolumeClaim != mc.Status.PersistentVolumeClaim ||
		newStatus.ObservedGeneration != mc.Status.ObservedGeneration

	if changed {
		before := mc.DeepCopy()
		mc.Status = newStatus
		if err := r.Status().Patch(ctx, &mc, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch ModelCache status: %w", err)
		}
	}

	// Rely on watch events instead of periodic requeue
	return ctrl.Result{}, nil
}

// observation holds read-only snapshot of dependent resources and derived flags
type observation struct {
	pvcFound bool
	pvc      corev1.PersistentVolumeClaim
	jobFound bool
	job      batchv1.Job

	// derived
	pvcBound            bool
	storageReady        bool
	storageLost         bool
	jobSucceeded        bool
	jobFailed           bool
	jobPendingOrRunning bool

	// apply error projection
	applyErrorReason  string
	applyErrorMessage string
	applyTerminal     bool
}

func (r *ModelCacheReconciler) observe(ctx context.Context, mc *kaiwo.ModelCache) (ob observation, err error) {
	// PVC
	pvcName := r.pvcName(mc)
	if err := r.Get(ctx, types.NamespacedName{Namespace: mc.Namespace, Name: pvcName}, &ob.pvc); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ob, fmt.Errorf("get pvc: %w", err)
		}
		ob.pvcFound = false
	} else {
		ob.pvcFound = true
		ob.pvcBound = ob.pvc.Status.Phase == corev1.ClaimBound
	}

	// Job
	jobName := r.jobName(mc)
	if err := r.Get(ctx, types.NamespacedName{Namespace: mc.Namespace, Name: jobName}, &ob.job); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ob, fmt.Errorf("get job: %w", err)
		}
		ob.jobFound = false
	} else {
		ob.jobFound = true
		for _, c := range ob.job.Status.Conditions {
			if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
				ob.jobFailed = true
			}
			if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
				ob.jobSucceeded = true
			}
		}
		if ob.job.Status.Succeeded > 0 {
			ob.jobSucceeded = true
		}
		if ob.job.Status.Active > 0 || baseutils.ValueOrDefault(ob.job.Status.Ready) > 0 {
			ob.jobPendingOrRunning = true
		}
	}

	// derived storage flags
	ob.storageReady = ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimBound
	ob.storageLost = ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimLost
	return ob, nil
}

func (r *ModelCacheReconciler) apply(ctx context.Context, mc *kaiwo.ModelCache, ob observation) error {
	// Server-Side Apply desired PVC on every reconcile
	pvc := r.buildPVC(mc, r.pvcName(mc))
	if err := ctrl.SetControllerReference(mc, pvc, r.Scheme); err != nil {
		return fmt.Errorf("owner pvc: %w", err)
	}
	if err := r.Patch(ctx, pvc, client.Apply, client.FieldOwner("modelcache-controller")); err != nil {
		return fmt.Errorf("apply pvc: %w", err)
	}

	// Apply Job only when storage is ready
	if ob.storageReady {
		job := r.buildDownloadJob(mc, r.jobName(mc), r.pvcName(mc))
		if err := ctrl.SetControllerReference(mc, job, r.Scheme); err != nil {
			return fmt.Errorf("owner job: %w", err)
		}
		if err := r.Patch(ctx, job, client.Apply, client.FieldOwner("modelcache-controller")); err != nil {
			return fmt.Errorf("apply job: %w", err)
		}
	}
	return nil
}

// classifyApplyError inspects known immutable-field patterns to produce
// a stable reason/message for status projection.
func classifyApplyError(err error) (reason, message string, terminal bool) {
	if err == nil {
		return "", "", false
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	// Common immutability markers from API server validation
	if strings.Contains(lower, "immutable") || strings.Contains(lower, "may not be changed") || strings.Contains(lower, "forbidden") && strings.Contains(lower, "immutable") {
		return "StorageImmutable", msg, true
	}
	return "ApplyError", msg, false
}

func (r *ModelCacheReconciler) projectStatus(mc *kaiwo.ModelCache, ob observation) kaiwo.ModelCacheStatus {
	status := mc.Status
	status.PersistentVolumeClaim = r.pvcName(mc)
	status.ObservedGeneration = mc.GetGeneration()

	// StorageReady
	storageCond := metav1.Condition{Type: kaiwo.ConditionStorageReady}
	switch {
	case !ob.pvcFound:
		storageCond.Status = metav1.ConditionFalse
		storageCond.Reason = kaiwo.ReasonPVCPending
		storageCond.Message = "PVC not created yet"
	case ob.pvc.Status.Phase == corev1.ClaimBound:
		storageCond.Status = metav1.ConditionTrue
		storageCond.Reason = kaiwo.ReasonPVCBound
	case ob.pvc.Status.Phase == corev1.ClaimPending:
		storageCond.Status = metav1.ConditionFalse
		storageCond.Reason = kaiwo.ReasonPVCProvisioning
		storageCond.Message = "PVC is provisioning"
	case ob.pvc.Status.Phase == corev1.ClaimLost:
		storageCond.Status = metav1.ConditionFalse
		storageCond.Reason = kaiwo.ReasonPVCLost
		storageCond.Message = "PVC lost"
	default:
		storageCond.Status = metav1.ConditionUnknown
		storageCond.Reason = string(ob.pvc.Status.Phase)
	}

	// Aggregate flags
	failure := ob.storageLost || ob.jobFailed || ob.applyErrorReason != ""
	ready := ob.storageReady && ob.jobSucceeded && ob.applyErrorReason == ""
	progressing := !ready && !failure && (!ob.storageReady || ob.jobPendingOrRunning || (!ob.jobFound && ob.storageReady))

	readyCond := metav1.Condition{Type: kaiwo.ConditionReady}
	if ready {
		readyCond.Status = metav1.ConditionTrue
		readyCond.Reason = kaiwo.ReasonWarm
	} else {
		readyCond.Status = metav1.ConditionFalse
		if !ob.storageReady {
			readyCond.Reason = kaiwo.ReasonWaitingForPVC
		} else {
			readyCond.Reason = kaiwo.ReasonDownloading
		}
	}

	progressingCond := metav1.Condition{Type: kaiwo.ConditionProgressing}
	progressingCond.Status = boolToCondition(progressing)
	if !ob.storageReady {
		progressingCond.Reason = kaiwo.ReasonWaitingForPVC
	} else if ob.jobPendingOrRunning || (!ob.jobFound && ob.storageReady) {
		progressingCond.Reason = kaiwo.ReasonDownloading
	} else {
		progressingCond.Reason = kaiwo.ReasonRetryBackoff
	}

	failureCond := metav1.Condition{Type: kaiwo.ConditionFailure}
	failureCond.Status = boolToCondition(failure)
	if ob.applyErrorReason != "" {
		failureCond.Reason = ob.applyErrorReason
		failureCond.Message = ob.applyErrorMessage
	} else if ob.storageLost {
		failureCond.Reason = kaiwo.ReasonPVCLost
	} else if ob.jobFailed {
		failureCond.Reason = kaiwo.ReasonDownloadFailed
	}

	// Merge
	status.Conditions = mergeConditions(status.Conditions, []metav1.Condition{storageCond, readyCond, progressingCond, failureCond})

	// High-level enum
	switch {
	case failure && ob.applyTerminal:
		status.Status = kaiwo.ModelCacheStatusFailed
	case ready:
		status.Status = kaiwo.ModelCacheStatusAvailable
	case progressing:
		status.Status = kaiwo.ModelCacheStatusProgressing
	default:
		status.Status = kaiwo.ModelCacheStatusPending
	}
	return status
}

//func (r *ModelCacheReconciler) isProgressing(st kaiwo.ModelCacheStatus) bool {
//	cond := meta.FindStatusCondition(st.Conditions, kaiwo.ConditionProgressing)
//	return cond != nil && cond.Status == metav1.ConditionTrue
//}

func (r *ModelCacheReconciler) buildPVC(mc *kaiwo.ModelCache, pvcName string) *corev1.PersistentVolumeClaim {
	var sc *string
	if mc.Spec.StorageClassName != "" {
		v := mc.Spec.StorageClassName
		sc = &v
	}
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: mc.Namespace,
			Labels: mergeStringMap(nil, map[string]string{
				"app.kubernetes.io/managed-by": "modelcache-controller",
				"kaiwo.silogen.ai/modelcache":  mc.Name,
			}),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: mc.Spec.Size,
				},
			},
			StorageClassName: sc,
		},
	}
}

func (r *ModelCacheReconciler) buildDownloadJob(mc *kaiwo.ModelCache, jobName string, pvcName string) *batchv1.Job {
	mountPath := "/cache"
	sourceEnv := corev1.EnvVar{Name: "SOURCE_URI", Value: mc.Spec.SourceURI}
	env := append([]corev1.EnvVar{sourceEnv}, mc.Spec.Env...)
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: mc.Namespace,
			Labels: mergeStringMap(nil, map[string]string{
				"app.kubernetes.io/managed-by": "modelcache-controller",
				"kaiwo.silogen.ai/modelcache":  mc.Name,
			}),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: baseutils.Pointer(int32(0)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "cache",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "model-download",
							Image:           downloadJobImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             env,
							Command:         []string{"/bin/sh", "-c", "echo Downloading $SOURCE_URI into /cache && sleep 1 && exit 0"},
							VolumeMounts:    []corev1.VolumeMount{{Name: "cache", MountPath: mountPath}},
						},
					},
				},
			},
		},
	}
}

func (r *ModelCacheReconciler) pvcName(mc *kaiwo.ModelCache) string {
	return baseutils.FormatNameWithPostfix(mc.Name, "cache")
}

func (r *ModelCacheReconciler) jobName(mc *kaiwo.ModelCache) string {
	return baseutils.FormatNameWithPostfix(mc.Name, "download")
}

func boolToCondition(v bool) metav1.ConditionStatus {
	if v {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// mergeConditions inserts or updates conditions by type using meta.SetStatusCondition semantics
func mergeConditions(existing, updates []metav1.Condition) []metav1.Condition {
	out := make([]metav1.Condition, len(existing))
	copy(out, existing)
	for _, c := range updates {
		meta.SetStatusCondition(&out, c)
	}
	return out
}

// conditionsEqual compares two condition slices ignoring LastTransitionTime and ObservedGeneration
func conditionsEqual(a, b []metav1.Condition) bool {
	// naive length + per-type compare
	if len(a) != len(b) {
		return false
	}
	// index by type
	index := func(list []metav1.Condition) map[string]metav1.Condition {
		m := map[string]metav1.Condition{}
		for _, c := range list {
			m[c.Type] = c
		}
		return m
	}
	ma, mb := index(a), index(b)
	if len(ma) != len(mb) {
		return false
	}
	for t, ca := range ma {
		cb, ok := mb[t]
		if !ok {
			return false
		}
		if ca.Type != cb.Type || ca.Status != cb.Status || ca.Reason != cb.Reason || ca.Message != cb.Message {
			return false
		}
	}
	return true
}

func (r *ModelCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("modelcache-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwo.ModelCache{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}, builder.WithPredicates(JobStatusChangedPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Named("modelcache").
		Complete(r)
}

// mergeStringMap returns a new map with b merged into a
func mergeStringMap(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
