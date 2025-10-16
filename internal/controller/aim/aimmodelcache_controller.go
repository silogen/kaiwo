/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package aim

import (
	"context"
	"fmt"

	inf "gopkg.in/inf.v0"

	"k8s.io/client-go/tools/record"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

// AIMModelCacheReconciler reconciles a AIMModelCache object
type AIMModelCacheReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Clientset kubernetes.Interface
}

// RBAC markers
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimmodelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aim.silogen.ai,resources=aimmodelcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

const (
	modelCacheFieldOwner = "modelcache-controller"
)

func (r *AIMModelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch CR
	var mc aimv1alpha1.AIMModelCache
	if err := r.Get(ctx, req.NamespacedName, &mc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return controllerutils.Reconcile(ctx, controllerutils.ReconcileSpec[*aimv1alpha1.AIMModelCache, aimv1alpha1.AIMModelCacheStatus]{
		Client:   r.Client,
		Scheme:   r.Scheme,
		Object:   &mc,
		Recorder: r.Recorder,
		ObserveFn: func(ctx context.Context) (any, error) {
			return r.observe(ctx, &mc)
		},
		FieldOwner: modelCacheFieldOwner,
		PlanFn: func(ctx context.Context, obs any) ([]client.Object, error) {
			var o *observation
			if obs != nil {
				var ok bool
				o, ok = obs.(*observation)
				if !ok {
					return nil, fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.plan(ctx, &mc, o)
		},
		ProjectFn: func(ctx context.Context, obs any, errs controllerutils.ReconcileErrors) error {
			var o *observation
			if obs != nil {
				var ok bool
				o, ok = obs.(*observation)
				if !ok {
					return fmt.Errorf("unexpected observation type %T", obs)
				}
			}
			return r.projectStatus(ctx, &mc, o, errs)
		},
		FinalizeFn: nil,
	})

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

func (r *AIMModelCacheReconciler) observe(ctx context.Context, mc *aimv1alpha1.AIMModelCache) (*observation, error) {
	ob := &observation{}
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

func (r *AIMModelCacheReconciler) plan(_ context.Context, mc *aimv1alpha1.AIMModelCache, ob *observation) ([]client.Object, error) {
	var desired []client.Object

	if ob == nil {
		return desired, nil
	}
	// Include PVC on every reconcile
	pvc := r.buildPVC(mc, r.pvcName(mc))
	if err := ctrl.SetControllerReference(mc, pvc, r.Scheme); err != nil {
		return desired, fmt.Errorf("owner pvc: %w", err)
	}
	desired = append(desired, pvc)

	// Include Job when storage is ready OR when PVC is pending with WaitForFirstConsumer
	canCreateJob := ob.storageReady || (ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimPending && ob.waitForFirstConsumer)
	if canCreateJob && !ob.jobFound {

		job := r.buildDownloadJob(mc, r.jobName(mc), r.pvcName(mc))
		if err := ctrl.SetControllerReference(mc, job, r.Scheme); err != nil {
			return desired, fmt.Errorf("owner job: %w", err)
		}
		desired = append(desired, job)

	}

	return desired, nil
}

type stateFlags struct {
	failure      bool
	ready        bool
	canCreateJob bool
	progressing  bool
}

func (r *AIMModelCacheReconciler) calculateStateFlags(ob observation) stateFlags {
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

func (r *AIMModelCacheReconciler) projectStatus(_ context.Context, mc *aimv1alpha1.AIMModelCache, ob *observation, errs controllerutils.ReconcileErrors) error {
	status := mc.Status
	var conditions []metav1.Condition

	//Report any outstanding errors to report from previous controller actions
	if errs.HasError() {

		if errs.ObserveErr != nil {
			conditions = append(conditions, controllerutils.NewCondition(
				controllerutils.ConditionTypeFailure,
				metav1.ConditionTrue,
				controllerutils.ReasonFailed,
				fmt.Sprintf("We have observation errors: %v", errs.ObserveErr),
			))
		}

		if errs.ApplyErr != nil {
			conditions = append(conditions, controllerutils.NewCondition(
				controllerutils.ConditionTypeFailure,
				metav1.ConditionTrue,
				controllerutils.ReasonFailed,
				fmt.Sprintf("Apply failed: %v", errs.ApplyErr),
			))
		}

		mc.Status.Status = aimv1alpha1.AIMModelCacheStatusFailed
		for _, cond := range conditions {
			meta.SetStatusCondition(&mc.Status.Conditions, cond)
		}
		return nil
	}

	//Check dependencies (pvc & job)
	mc.Status.PersistentVolumeClaim = r.pvcName(mc)
	mc.Status.ObservedGeneration = mc.GetGeneration()

	stateFlags := r.calculateStateFlags(*ob)

	storageCond := r.buildStorageReadyCondition(*ob)
	readyCond := r.buildReadyCondition(stateFlags)
	progressingCond := r.buildProgressingCondition(*ob, stateFlags)
	failureCond := r.buildFailureCondition(*ob, stateFlags)

	status.Conditions = mergeConditions(status.Conditions, []metav1.Condition{storageCond, readyCond, progressingCond, failureCond})
	for _, cond := range status.Conditions {
		meta.SetStatusCondition(&mc.Status.Conditions, cond)
	}
	mc.Status.Status = r.determineOverallStatus(stateFlags, *ob)

	//fmt.Printf("%v\n", newStatus)
	//r.Recorder

	return nil
}

func (r *AIMModelCacheReconciler) buildStorageReadyCondition(ob observation) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.ConditionStorageReady}

	switch {
	case !ob.pvcFound:
		cond.Status = metav1.ConditionFalse
		cond.Reason = aimv1alpha1.ReasonPVCPending
		cond.Message = "PVC not created yet"
	case ob.pvc.Status.Phase == corev1.ClaimBound:
		cond.Status = metav1.ConditionTrue
		cond.Reason = aimv1alpha1.ReasonPVCBound
	case ob.pvc.Status.Phase == corev1.ClaimPending:
		cond.Status = metav1.ConditionFalse
		cond.Reason = aimv1alpha1.ReasonPVCProvisioning
		cond.Message = "PVC is provisioning"
	case ob.pvc.Status.Phase == corev1.ClaimLost:
		cond.Status = metav1.ConditionFalse
		cond.Reason = aimv1alpha1.ReasonPVCLost
		cond.Message = "PVC lost"
	default:
		cond.Status = metav1.ConditionUnknown
		cond.Reason = string(ob.pvc.Status.Phase)
	}

	return cond
}

func (r *AIMModelCacheReconciler) buildReadyCondition(sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.ConditionReady}
	if sf.ready {
		cond.Status = metav1.ConditionTrue
		cond.Reason = aimv1alpha1.ReasonWarm
	} else {
		cond.Status = metav1.ConditionFalse
		if !sf.canCreateJob {
			cond.Reason = aimv1alpha1.ReasonWaitingForPVC
		} else {
			cond.Reason = aimv1alpha1.ReasonDownloading
		}
	}

	return cond
}

func (r *AIMModelCacheReconciler) buildProgressingCondition(ob observation, sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.ConditionProgressing}
	cond.Status = boolToCondition(sf.progressing)

	if !ob.storageReady && !sf.canCreateJob {
		cond.Reason = aimv1alpha1.ReasonWaitingForPVC
	} else if ob.jobPendingOrRunning || (!ob.jobFound && sf.canCreateJob) {
		cond.Reason = aimv1alpha1.ReasonDownloading
	} else {
		cond.Reason = aimv1alpha1.ReasonRetryBackoff
	}

	return cond
}

func (r *AIMModelCacheReconciler) buildFailureCondition(ob observation, sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.ConditionFailure}
	cond.Status = boolToCondition(sf.failure)
	cond.Reason = "NoFailure" // Ensure Reason is always non-empty to satisfy schema

	if ob.applyErrorReason != "" {
		cond.Reason = ob.applyErrorReason
		cond.Message = ob.applyErrorMessage
	} else if ob.storageLost {
		cond.Reason = aimv1alpha1.ReasonPVCLost
	} else if ob.jobFailed {
		cond.Reason = aimv1alpha1.ReasonDownloadFailed
	}

	return cond
}

func (r *AIMModelCacheReconciler) determineOverallStatus(sf stateFlags, ob observation) aimv1alpha1.AIMModelCacheStatusEnum {
	switch {
	case sf.failure && (ob.applyTerminal || ob.jobFailed):
		return aimv1alpha1.AIMModelCacheStatusFailed
	case sf.ready:
		return aimv1alpha1.AIMModelCacheStatusAvailable
	case sf.progressing:
		return aimv1alpha1.AIMModelCacheStatusProgressing
	default:
		return aimv1alpha1.AIMModelCacheStatusPending
	}
}

func (r *AIMModelCacheReconciler) buildPVC(mc *aimv1alpha1.AIMModelCache, pvcName string) *corev1.PersistentVolumeClaim {
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
				"aim.silogen.ai/modelcache":    mc.Name,
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

func MultiplyQuantityByFloatExact(q resource.Quantity, factor float64) resource.Quantity {
	dec := q.AsDec()
	factorDec := inf.NewDec(int64(factor*1e6), 6) // represent float as decimal
	dec.Mul(dec, factorDec)
	return *resource.NewDecimalQuantity(*dec, q.Format)
}

func (r *AIMModelCacheReconciler) buildDownloadJob(mc *aimv1alpha1.AIMModelCache, jobName string, pvcName string) *batchv1.Job {
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
				"aim.silogen.ai/modelcache":    mc.Name,
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
					ImagePullSecrets: mc.Spec.ImagePullSecrets,
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
					Containers: []corev1.Container{
						{
							Name:            "model-download",
							Image:           mc.Spec.ModelDownloadImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  baseutils.Pointer(int64(1000)),
								RunAsGroup: baseutils.Pointer(int64(1000)),
							},
							Env: append(mc.Spec.Env, []corev1.EnvVar{
								{Name: "HF_HUB_DISABLE_XET", Value: "1"},
								{Name: "HF_HOME", Value: mountPath + "/.hf"},
								{Name: "UMASK", Value: "0022"}, // Create files with 644 permissions (readable by others)
							}...),
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								fmt.Sprintf(`
# Download the model
python /storage-initializer/scripts/initializer-entrypoint %s %s &&
(
# Clean up HF xet cache to save space (keeps only final model files)
echo "Cleaning up HF cache to save space..."
rm -rf %s/.hf/xet/*/chunk-cache 2>/dev/null || true
rm -rf %s/.hf/xet/*/staging 2>/dev/null || true

# Report final sizes
echo "Final storage usage:"
du -sh %s
du -sh %s/.hf 2>/dev/null || true
)
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

func (r *AIMModelCacheReconciler) pvcName(mc *aimv1alpha1.AIMModelCache) string {
	return baseutils.FormatNameWithPostfix(mc.Name, "cache")
}

func (r *AIMModelCacheReconciler) jobName(mc *aimv1alpha1.AIMModelCache) string {
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

func (r *AIMModelCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("modelcache-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimv1alpha1.AIMModelCache{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Named("modelcache-controller").
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
