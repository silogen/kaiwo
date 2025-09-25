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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

const (
	downloadJobImage = "kserve/storage-initializer:v0.15.2"
)

func (r *ModelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("modelcache", req.String())
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

	// Emit events based on observed state prior to apply, gated by prior status
	prevStorage := meta.FindStatusCondition(mc.Status.Conditions, kaiwo.ConditionStorageReady)
	prevReady := meta.FindStatusCondition(mc.Status.Conditions, kaiwo.ConditionReady)
	if !ob.pvcFound && (prevStorage == nil || prevStorage.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "PVCEnsured", fmt.Sprintf("Ensured PVC %s", r.pvcName(&mc)))
		logger.V(1).Info("PVC ensured (SSA)", "pvc", r.pvcName(&mc))
	}
	if ob.storageReady && (prevStorage == nil || prevStorage.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "PVCBound", fmt.Sprintf("PVC %s is Bound", r.pvcName(&mc)))
		logger.Info("PVC bound", "pvc", r.pvcName(&mc))
	}
	if ob.storageReady && !ob.jobFound && (prevReady == nil || prevReady.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "DownloadJobEnsured", fmt.Sprintf("Ensured download Job %s", r.jobName(&mc)))
		logger.V(1).Info("Download job ensured (SSA)", "job", r.jobName(&mc))
	}
	if ob.jobSucceeded && (prevReady == nil || prevReady.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeNormal, "DownloadJobCompleted", fmt.Sprintf("Job %s completed successfully", r.jobName(&mc)))
		logger.Info("Download job completed", "job", r.jobName(&mc))
	}
	if ob.jobFailed && (prevReady == nil || prevReady.Status != metav1.ConditionTrue) {
		r.Recorder.Event(&mc, corev1.EventTypeWarning, "DownloadJobFailed", fmt.Sprintf("Job %s failed", r.jobName(&mc)))
		logger.Info("Download job failed", "job", r.jobName(&mc))
	}

	// Status projection and patch
	newStatus := r.projectStatus(&mc, ob)
	changed := !conditionsEqual(newStatus.Conditions, mc.Status.Conditions) ||
		newStatus.Status != mc.Status.Status ||
		newStatus.PersistentVolumeClaim != mc.Status.PersistentVolumeClaim ||
		newStatus.ObservedGeneration != mc.Status.ObservedGeneration

	if changed {
		summary := summarizeConditionTransitions(mc.Status.Conditions, newStatus.Conditions)
		if summary != "" {
			evtType := corev1.EventTypeNormal
			if cond := meta.FindStatusCondition(newStatus.Conditions, kaiwo.ConditionFailure); cond != nil && cond.Status == metav1.ConditionTrue {
				evtType = corev1.EventTypeWarning
			}
			logger.Info("status transitioned", "changes", summary)
			r.Recorder.Event(&mc, evtType, "StatusChanged", summary)
		}
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
	pvcFound          bool
	pvc               corev1.PersistentVolumeClaim
	storageClass      storagev1.StorageClass
	storageClassFound bool
	jobFound          bool
	job               batchv1.Job

	// derived
	pvcBound             bool
	waitForFirstConsumer bool
	storageReady         bool
	storageLost          bool
	jobSucceeded         bool
	jobFailed            bool
	jobPendingOrRunning  bool

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

		// Get StorageClass to check binding mode
		if ob.pvc.Spec.StorageClassName != nil && *ob.pvc.Spec.StorageClassName != "" {
			if err := r.Get(ctx, types.NamespacedName{Name: *ob.pvc.Spec.StorageClassName}, &ob.storageClass); err != nil {
				if client.IgnoreNotFound(err) != nil {
					return ob, fmt.Errorf("get storageclass: %w", err)
				}
				ob.storageClassFound = false
			} else {
				ob.storageClassFound = true
				if ob.storageClass.VolumeBindingMode != nil {
					ob.waitForFirstConsumer = *ob.storageClass.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
				}
			}
		}
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
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pvc := r.buildPVC(mc, r.pvcName(mc))
		if err := ctrl.SetControllerReference(mc, pvc, r.Scheme); err != nil {
			return fmt.Errorf("owner pvc: %w", err)
		}
		if err := r.Patch(ctx, pvc, client.Apply, client.FieldOwner("modelcache-controller")); err != nil {
			return fmt.Errorf("apply pvc: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Apply Job when storage is ready OR when PVC is pending with WaitForFirstConsumer
	canCreateJob := ob.storageReady || (ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimPending && ob.waitForFirstConsumer)
	if canCreateJob && !ob.jobFound {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			job := r.buildDownloadJob(mc, r.jobName(mc), r.pvcName(mc))
			if err := ctrl.SetControllerReference(mc, job, r.Scheme); err != nil {
				return fmt.Errorf("owner job: %w", err)
			}
			if err := r.Patch(ctx, job, client.Apply, client.FieldOwner("modelcache-controller")); err != nil {
				return fmt.Errorf("apply job: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
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
	if strings.Contains(lower, "immutable") ||
		strings.Contains(lower, "may not be changed") ||
		(strings.Contains(lower, "forbidden") && strings.Contains(lower, "immutable")) {
		return "StorageImmutable", msg, true
	}
	return "ApplyError", msg, false
}

type stateFlags struct {
	failure      bool
	ready        bool
	canCreateJob bool
	progressing  bool
}

func (r *ModelCacheReconciler) calculateStateFlags(ob observation) stateFlags {
	failure := ob.storageLost || ob.jobFailed || ob.applyErrorReason != ""
	ready := ob.storageReady && ob.jobSucceeded && ob.applyErrorReason == ""
	canCreateJob := ob.storageReady || (ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimPending && ob.waitForFirstConsumer)
	progressing := !ready && !failure && (!ob.storageReady || ob.jobPendingOrRunning || (!ob.jobFound && canCreateJob))

	return stateFlags{
		failure:      failure,
		ready:        ready,
		canCreateJob: canCreateJob,
		progressing:  progressing,
	}
}

func (r *ModelCacheReconciler) projectStatus(mc *kaiwo.ModelCache, ob observation) kaiwo.ModelCacheStatus {
	status := mc.Status
	status.PersistentVolumeClaim = r.pvcName(mc)
	status.ObservedGeneration = mc.GetGeneration()

	stateFlags := r.calculateStateFlags(ob)

	storageCond := r.buildStorageReadyCondition(ob)
	readyCond := r.buildReadyCondition(stateFlags)
	progressingCond := r.buildProgressingCondition(ob, stateFlags)
	failureCond := r.buildFailureCondition(ob, stateFlags)

	status.Conditions = mergeConditions(status.Conditions, []metav1.Condition{storageCond, readyCond, progressingCond, failureCond})
	status.Status = r.determineOverallStatus(stateFlags, ob)

	return status
}

func (r *ModelCacheReconciler) buildStorageReadyCondition(ob observation) metav1.Condition {
	cond := metav1.Condition{Type: kaiwo.ConditionStorageReady}

	switch {
	case !ob.pvcFound:
		cond.Status = metav1.ConditionFalse
		cond.Reason = kaiwo.ReasonPVCPending
		cond.Message = "PVC not created yet"
	case ob.pvc.Status.Phase == corev1.ClaimBound:
		cond.Status = metav1.ConditionTrue
		cond.Reason = kaiwo.ReasonPVCBound
	case ob.pvc.Status.Phase == corev1.ClaimPending:
		cond.Status = metav1.ConditionFalse
		cond.Reason = kaiwo.ReasonPVCProvisioning
		cond.Message = "PVC is provisioning"
	case ob.pvc.Status.Phase == corev1.ClaimLost:
		cond.Status = metav1.ConditionFalse
		cond.Reason = kaiwo.ReasonPVCLost
		cond.Message = "PVC lost"
	default:
		cond.Status = metav1.ConditionUnknown
		cond.Reason = string(ob.pvc.Status.Phase)
	}

	return cond
}

func (r *ModelCacheReconciler) buildReadyCondition(sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: kaiwo.ConditionReady}
	if sf.ready {
		cond.Status = metav1.ConditionTrue
		cond.Reason = kaiwo.ReasonWarm
	} else {
		cond.Status = metav1.ConditionFalse
		if !sf.canCreateJob {
			cond.Reason = kaiwo.ReasonWaitingForPVC
		} else {
			cond.Reason = kaiwo.ReasonDownloading
		}
	}

	return cond
}

func (r *ModelCacheReconciler) buildProgressingCondition(ob observation, sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: kaiwo.ConditionProgressing}
	cond.Status = boolToCondition(sf.progressing)

	if !ob.storageReady && !sf.canCreateJob {
		cond.Reason = kaiwo.ReasonWaitingForPVC
	} else if ob.jobPendingOrRunning || (!ob.jobFound && sf.canCreateJob) {
		cond.Reason = kaiwo.ReasonDownloading
	} else {
		cond.Reason = kaiwo.ReasonRetryBackoff
	}

	return cond
}

func (r *ModelCacheReconciler) buildFailureCondition(ob observation, sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: kaiwo.ConditionFailure}
	cond.Status = boolToCondition(sf.failure)
	cond.Reason = "NoFailure" // Ensure Reason is always non-empty to satisfy schema

	if ob.applyErrorReason != "" {
		cond.Reason = ob.applyErrorReason
		cond.Message = ob.applyErrorMessage
	} else if ob.storageLost {
		cond.Reason = kaiwo.ReasonPVCLost
	} else if ob.jobFailed {
		cond.Reason = kaiwo.ReasonDownloadFailed
	}

	return cond
}

func (r *ModelCacheReconciler) determineOverallStatus(sf stateFlags, ob observation) kaiwo.ModelCacheStatusEnum {
	switch {
	case sf.failure && (ob.applyTerminal || ob.jobFailed):
		return kaiwo.ModelCacheStatusFailed
	case sf.ready:
		return kaiwo.ModelCacheStatusAvailable
	case sf.progressing:
		return kaiwo.ModelCacheStatusProgressing
	default:
		return kaiwo.ModelCacheStatusPending
	}
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
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
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
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: batchv1.SchemeGroupVersion.String(),
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: mc.Namespace,
			Labels: mergeStringMap(nil, map[string]string{
				"app.kubernetes.io/managed-by": "modelcache-controller",
				"kaiwo.silogen.ai/modelcache":  mc.Name,
			}),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            baseutils.Pointer(int32(0)),
			TTLSecondsAfterFinished: baseutils.Pointer(int32(30)), // Cleanup after 30s to allow status observation
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    baseutils.Pointer(int64(1000)), // kserve storage-initializer user
						RunAsGroup:   baseutils.Pointer(int64(1000)),
						FSGroup:      baseutils.Pointer(int64(1000)), // Ensures volume ownership matches user
						RunAsNonRoot: baseutils.Pointer(true),
					},
					Volumes: []corev1.Volume{
						{
							Name: "cache",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: baseutils.Pointer(resource.MustParse("500Mi")), // Small temp space for system operations
								},
							},
						},
					},
					// TODO find out how to reduce the duplicate cache size during download
					Containers: []corev1.Container{
						{
							Name:            "model-download",
							Image:           downloadJobImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  baseutils.Pointer(int64(1000)),
								RunAsGroup: baseutils.Pointer(int64(1000)),
							},
							Env: append(mc.Spec.Env, []corev1.EnvVar{
								{Name: "HF_HOME", Value: mountPath + "/.hf"},
								{Name: "UMASK", Value: "0022"}, // Create files with 644 permissions (readable by others)
							}...),
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								fmt.Sprintf(`
# Download the model
python /storage-initializer/scripts/initializer-entrypoint %s %s

# Clean up HF xet cache to save space (keeps only final model files)
echo "Cleaning up HF cache to save space..."
rm -rf %s/.hf/xet/*/chunk-cache 2>/dev/null || true
rm -rf %s/.hf/xet/*/staging 2>/dev/null || true

# Report final sizes
echo "Final storage usage:"
du -sh %s
du -sh %s/.hf 2>/dev/null || true
				`, mc.Spec.SourceURI, mountPath, mountPath, mountPath, mountPath, mountPath),
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "cache", MountPath: mountPath},
								{Name: "tmp", MountPath: "/tmp"},
							},
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
	return baseutils.FormatNameWithPostfix(mc.Name, "cache-download")
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

// summarizeConditionTransitions produces a compact, human-friendly diff of condition changes
func summarizeConditionTransitions(oldC, newC []metav1.Condition) string {
	idx := func(cs []metav1.Condition) map[string]metav1.Condition {
		m := map[string]metav1.Condition{}
		for _, c := range cs {
			m[c.Type] = c
		}
		return m
	}
	o, n := idx(oldC), idx(newC)
	var parts []string
	for t, nc := range n {
		oc, ok := o[t]
		if !ok || oc.Status != nc.Status || oc.Reason != nc.Reason {
			parts = append(parts, fmt.Sprintf("%s: %s→%s (%s)", t, string(oc.Status), string(nc.Status), nc.Reason))
		}
	}
	return strings.Join(parts, "; ")
}
