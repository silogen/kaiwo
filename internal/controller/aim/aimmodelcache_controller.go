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
	"strings"

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
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/helpers"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
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
	logger := log.FromContext(ctx)

	// Fetch CR
	var mc aimv1alpha1.AIMModelCache
	if err := r.Get(ctx, req.NamespacedName, &mc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	baseutils.Debug(logger, "Reconciling AIMModelCache", "name", mc.Name, "namespace", mc.Namespace)

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

	// runtime config
	runtimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec

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
	logger := log.FromContext(ctx)
	ob := &observation{}
	// PVC
	pvcName := r.pvcName(mc)
	if err := r.Get(ctx, types.NamespacedName{Namespace: mc.Namespace, Name: pvcName}, &ob.pvc); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ob, fmt.Errorf("get pvc: %w", err)
		}
		ob.pvcFound = false
		baseutils.Debug(logger, "PVC not found", "pvcName", pvcName)
	} else {
		ob.pvcFound = true
		ob.pvcBound = ob.pvc.Status.Phase == corev1.ClaimBound
		baseutils.Debug(logger, "PVC observed", "pvcName", pvcName, "phase", ob.pvc.Status.Phase, "bound", ob.pvcBound)

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
		baseutils.Debug(logger, "Download job not found", "jobName", jobName)
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
		baseutils.Debug(logger, "Download job observed", "jobName", jobName,
			"succeeded", ob.jobSucceeded, "failed", ob.jobFailed, "running", ob.jobPendingOrRunning)
	}

	// derived storage flags
	ob.storageReady = ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimBound
	ob.storageLost = ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimLost

	// Resolve runtime config for PVC headroom and other settings
	runtimeConfig, err := shared.ResolveRuntimeConfig(ctx, r.Client, mc.Namespace, mc.Spec.RuntimeConfigName)
	if err != nil {
		// If runtime config resolution fails, use empty spec (will use defaults)
		baseutils.Debug(logger, "Failed to resolve runtime config, using defaults", "error", err, "runtimeConfigName", mc.Spec.RuntimeConfigName)
		ob.runtimeConfigSpec = aimv1alpha1.AIMRuntimeConfigSpec{}
	} else {
		ob.runtimeConfigSpec = runtimeConfig.EffectiveSpec
	}

	return ob, nil
}

func (r *AIMModelCacheReconciler) plan(ctx context.Context, mc *aimv1alpha1.AIMModelCache, ob *observation) ([]client.Object, error) {
	logger := log.FromContext(ctx)
	var desired []client.Object

	if ob == nil {
		return desired, nil
	}

	// Include PVC only if it doesn't exist yet
	// Once created, PVCs are immutable - we never modify them to avoid:
	// 1. StorageClassName mutation errors (forbidden by Kubernetes)
	// 2. Storage size shrinkage errors (forbidden by Kubernetes)
	// 3. Unexpected PVC expansion from runtime config changes
	if !ob.pvcFound {
		headroomPercent := shared.GetPVCHeadroomPercent(ob.runtimeConfigSpec)
		storageClassName := shared.ResolveStorageClass(mc.Spec.StorageClassName, ob.runtimeConfigSpec)
		pvcSize := shared.QuantityWithHeadroom(mc.Spec.Size.Value(), headroomPercent)

		pvc := r.buildPVC(mc, r.pvcName(mc), pvcSize, storageClassName)

		// Propagate labels from model cache to PVC based on runtime config
		shared.PropagateLabels(mc, pvc, &ob.runtimeConfigSpec.AIMRuntimeConfigCommon)

		if err := ctrl.SetControllerReference(mc, pvc, r.Scheme); err != nil {
			return desired, fmt.Errorf("owner pvc: %w", err)
		}
		desired = append(desired, pvc)
	}

	// Include Job when storage is ready OR when PVC is pending with WaitForFirstConsumer
	// Skip job creation if download already completed (status is Available, even if job was TTL'd)
	downloadCompleted := ob.jobSucceeded || mc.Status.Status == aimv1alpha1.AIMModelCacheStatusAvailable
	canCreateJob := ob.storageReady || (ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimPending && ob.waitForFirstConsumer)
	if canCreateJob && !ob.jobFound && !downloadCompleted {
		baseutils.Debug(logger, "Planning to create download job",
			"storageReady", ob.storageReady,
			"waitForFirstConsumer", ob.waitForFirstConsumer)

		job := r.buildDownloadJob(mc, r.jobName(mc), r.pvcName(mc), ob.runtimeConfigSpec)

		// Propagate labels from model cache to download job based on runtime config
		shared.PropagateLabels(mc, job, &ob.runtimeConfigSpec.AIMRuntimeConfigCommon)

		if err := ctrl.SetControllerReference(mc, job, r.Scheme); err != nil {
			return desired, fmt.Errorf("owner job: %w", err)
		}
		desired = append(desired, job)
		controllerutils.EmitNormalEvent(r.Recorder, mc, "DownloadJobCreated",
			fmt.Sprintf("Creating download job %s", r.jobName(mc)))
	} else if !canCreateJob && !downloadCompleted {
		baseutils.Debug(logger, "Waiting for storage to be ready before creating job",
			"pvcFound", ob.pvcFound,
			"pvcPhase", ob.pvc.Status.Phase)
	}

	return desired, nil
}

type stateFlags struct {
	failure      bool
	ready        bool
	canCreateJob bool
	progressing  bool
}

func (r *AIMModelCacheReconciler) calculateStateFlags(ob observation, currentStatus aimv1alpha1.AIMModelCacheStatusEnum) stateFlags {
	failure := ob.storageLost || ob.jobFailed || ob.applyErrorReason != ""
	// Consider ready if:
	// 1. Storage is ready AND job succeeded, OR
	// 2. Storage is ready AND we were previously Available (job may have been TTL'd)
	wasAvailable := currentStatus == aimv1alpha1.AIMModelCacheStatusAvailable
	ready := ob.storageReady && (ob.jobSucceeded || wasAvailable) && ob.applyErrorReason == ""
	canCreateJob := ob.storageReady || (ob.pvcFound && ob.pvc.Status.Phase == corev1.ClaimPending && ob.waitForFirstConsumer)
	progressing := !ready && !failure && (!ob.storageReady || ob.jobPendingOrRunning || (!ob.jobFound && canCreateJob))

	return stateFlags{
		failure:      failure,
		ready:        ready,
		canCreateJob: canCreateJob,
		progressing:  progressing,
	}
}

func (r *AIMModelCacheReconciler) projectStatus(ctx context.Context, mc *aimv1alpha1.AIMModelCache, ob *observation, errs controllerutils.ReconcileErrors) error {
	logger := log.FromContext(ctx)
	status := mc.Status
	var conditions []metav1.Condition

	if mc.Status.Status == "" {
		mc.Status.Status = aimv1alpha1.AIMModelCacheStatusPending
	}

	// Report any outstanding errors to report from previous controller actions
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
		logger.Error(nil, "Model cache reconciliation failed", "status", mc.Status.Status)
		controllerutils.EmitWarningEvent(r.Recorder, mc, "ReconcileFailed", "Failed to reconcile model cache")
		return nil
	}

	// Check dependencies (pvc & job)
	mc.Status.PersistentVolumeClaim = r.pvcName(mc)
	mc.Status.ObservedGeneration = mc.GetGeneration()

	stateFlags := r.calculateStateFlags(*ob, mc.Status.Status)

	storageCond := r.buildStorageReadyCondition(*ob)
	readyCond := r.buildReadyCondition(stateFlags)
	progressingCond := r.buildProgressingCondition(*ob, stateFlags)
	failureCond := r.buildFailureCondition(*ob, stateFlags)

	status.Conditions = mergeConditions(status.Conditions, []metav1.Condition{storageCond, readyCond, progressingCond, failureCond})
	for _, cond := range status.Conditions {
		meta.SetStatusCondition(&mc.Status.Conditions, cond)
	}
	newStatus := r.determineOverallStatus(stateFlags, *ob)

	// Log and emit events for status transitions
	if mc.Status.Status != newStatus {
		logger.Info("Model cache status changed",
			"previousStatus", mc.Status.Status,
			"newStatus", newStatus,
			"pvcName", mc.Status.PersistentVolumeClaim)

		switch newStatus {
		case aimv1alpha1.AIMModelCacheStatusAvailable:
			controllerutils.EmitNormalEvent(r.Recorder, mc, "CacheReady",
				fmt.Sprintf("Model cache is available at PVC %s", mc.Status.PersistentVolumeClaim))
		case aimv1alpha1.AIMModelCacheStatusFailed:
			var reason string
			if ob.storageLost {
				reason = "PVC lost"
			} else if ob.jobFailed {
				reason = "Download job failed"
			} else {
				reason = "Unknown failure"
			}
			controllerutils.EmitWarningEvent(r.Recorder, mc, "CacheFailed",
				fmt.Sprintf("Model cache failed: %s", reason))
		case aimv1alpha1.AIMModelCacheStatusProgressing:
			baseutils.Debug(logger, "Model cache progressing",
				"pvcBound", ob.pvcBound,
				"jobRunning", ob.jobPendingOrRunning)
		}
	}

	mc.Status.Status = newStatus

	return nil
}

func (r *AIMModelCacheReconciler) buildStorageReadyCondition(ob observation) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.AIMModelCacheConditionStorageReady}

	switch {
	case !ob.pvcFound:
		cond.Status = metav1.ConditionFalse
		cond.Reason = aimv1alpha1.AIMModelCacheReasonPVCPending
		cond.Message = "PVC not created yet"
	case ob.pvc.Status.Phase == corev1.ClaimBound:
		cond.Status = metav1.ConditionTrue
		cond.Reason = aimv1alpha1.AIMModelCacheReasonPVCBound
	case ob.pvc.Status.Phase == corev1.ClaimPending:
		cond.Status = metav1.ConditionFalse
		cond.Reason = aimv1alpha1.AIMModelCacheReasonPVCProvisioning
		cond.Message = "PVC is provisioning"
	case ob.pvc.Status.Phase == corev1.ClaimLost:
		cond.Status = metav1.ConditionFalse
		cond.Reason = aimv1alpha1.AIMModelCacheReasonPVCLost
		cond.Message = "PVC lost"
	default:
		cond.Status = metav1.ConditionUnknown
		cond.Reason = string(ob.pvc.Status.Phase)
	}

	return cond
}

func (r *AIMModelCacheReconciler) buildReadyCondition(sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.AIMModelCacheConditionReady}
	if sf.ready {
		cond.Status = metav1.ConditionTrue
		cond.Reason = aimv1alpha1.AIMModelCacheReasonWarm
	} else {
		cond.Status = metav1.ConditionFalse
		if !sf.canCreateJob {
			cond.Reason = aimv1alpha1.AIMModelCacheReasonWaitingForPVC
		} else {
			cond.Reason = aimv1alpha1.AIMModelCacheReasonDownloading
		}
	}

	return cond
}

func (r *AIMModelCacheReconciler) buildProgressingCondition(ob observation, sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.AIMModelCacheConditionProgressing}
	cond.Status = boolToCondition(sf.progressing)

	if !ob.storageReady && !sf.canCreateJob {
		cond.Reason = aimv1alpha1.AIMModelCacheReasonWaitingForPVC
	} else if ob.jobPendingOrRunning || (!ob.jobFound && sf.canCreateJob) {
		cond.Reason = aimv1alpha1.AIMModelCacheReasonDownloading
	} else {
		cond.Reason = aimv1alpha1.AIMModelCacheReasonRetryBackoff
	}

	return cond
}

func (r *AIMModelCacheReconciler) buildFailureCondition(ob observation, sf stateFlags) metav1.Condition {
	cond := metav1.Condition{Type: aimv1alpha1.AIMModelCacheConditionFailure}
	cond.Status = boolToCondition(sf.failure)
	cond.Reason = aimv1alpha1.AIMModelCacheReasonNoFailure // Ensure Reason is always non-empty to satisfy schema

	if ob.applyErrorReason != "" {
		cond.Reason = ob.applyErrorReason
		cond.Message = ob.applyErrorMessage
	} else if ob.storageLost {
		cond.Reason = aimv1alpha1.AIMModelCacheReasonPVCLost
	} else if ob.jobFailed {
		cond.Reason = aimv1alpha1.AIMModelCacheReasonDownloadFailed
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

// extractModelFromSourceURI extracts the model name from a sourceURI.
// Examples:
//   - "hf://amd/Llama-3.1-8B-Instruct" → "amd/Llama-3.1-8B-Instruct"
//   - "s3://bucket/model-v1" → "bucket/model-v1"
func extractModelFromSourceURI(sourceURI string) string {
	// Remove the scheme prefix (hf://, s3://, etc.)
	if idx := strings.Index(sourceURI, "://"); idx != -1 {
		return sourceURI[idx+3:]
	}
	return sourceURI
}

func (r *AIMModelCacheReconciler) buildPVC(mc *aimv1alpha1.AIMModelCache, pvcName string, pvcSize resource.Quantity, storageClassName string) *corev1.PersistentVolumeClaim {
	// Storage class: empty string means use cluster default
	var sc *string
	if storageClassName != "" {
		sc = &storageClassName
	}

	// Determine cache type based on whether this was created by a template cache
	cacheType := shared.LabelValueCacheTypeTemplateCache
	if mc.Labels == nil || mc.Labels["template-created"] != "true" {
		cacheType = "" // Standalone model cache (not template or service cache)
	}

	// Build labels with type and source information
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "modelcache-controller",
		shared.LabelKeyModelCache:      mc.Name,
	}

	// Add cache type if it's a template cache
	if cacheType != "" {
		labels[shared.LabelKeyCacheType] = cacheType
	}

	// Extract model name from sourceURI (e.g., "hf://amd/Llama-3.1-8B" → "amd/Llama-3.1-8B")
	if mc.Spec.SourceURI != "" {
		if modelName := extractModelFromSourceURI(mc.Spec.SourceURI); modelName != "" {
			labels[shared.LabelKeySourceModel] = shared.SanitizeLabelValue(modelName)
		}
	}

	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: mc.Namespace,
			Labels:    mergeStringMap(nil, labels),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: pvcSize,
				},
			},
			StorageClassName: sc,
		},
	}
}

func (r *AIMModelCacheReconciler) buildDownloadJob(mc *aimv1alpha1.AIMModelCache, jobName string, pvcName string, runtimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec) *batchv1.Job {
	mountPath := "/cache"
	downloadImage := aimv1alpha1.DefaultDownloadImage
	if len(mc.Spec.ModelDownloadImage) > 0 {
		downloadImage = mc.Spec.ModelDownloadImage
	}
	// Merge env vars with precedence: mc.Spec.Env > runtimeConfigSpec.Env > defaults
	newEnv := helpers.MergeEnvVars([]corev1.EnvVar{
		{Name: "HF_XET_CHUNK_CACHE_SIZE_BYTES", Value: "0"},
		{Name: "HF_XET_SHARD_CACHE_SIZE_BYTES", Value: "0"},
		{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
		{Name: "HF_HOME", Value: mountPath + "/.hf"},
		{Name: "UMASK", Value: "0022"},
	}, helpers.MergeEnvVars(runtimeConfigSpec.Env, mc.Spec.Env))

	// Expected size in bytes for progress calculation
	expectedSizeBytes := mc.Spec.Size.Value()

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
			BackoffLimit:            baseutils.Pointer(int32(200)),     // Since we kill the job after 5 minutes of no progress, we can set a high backoff limit
			TTLSecondsAfterFinished: baseutils.Pointer(int32(60 * 10)), // Cleanup after 10min to allow status observation
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:         corev1.RestartPolicyNever,
					ShareProcessNamespace: baseutils.Pointer(true), // Enable cross-container signaling
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
					// Native sidecar (Kubernetes 1.28+): init container with restartPolicy=Always
					// runs alongside main containers and is automatically terminated by kubelet
					// when all regular containers complete (success or failure)
					InitContainers: []corev1.Container{
						{
							Name:            "progress-monitor",
							Image:           "busybox:1.36",
							ImagePullPolicy: corev1.PullIfNotPresent,
							// restartPolicy: Always makes this a native sidecar that runs alongside main containers
							// Kubernetes automatically sends SIGTERM when all regular containers terminate
							RestartPolicy: baseutils.Pointer(corev1.ContainerRestartPolicyAlways),
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  baseutils.Pointer(int64(1000)),
								RunAsGroup: baseutils.Pointer(int64(1000)),
							},
							Env: []corev1.EnvVar{
								{Name: "EXPECTED_SIZE_BYTES", Value: fmt.Sprintf("%d", expectedSizeBytes)},
								{Name: "MOUNT_PATH", Value: mountPath},
							},
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								progressMonitorScript,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "cache", MountPath: mountPath, ReadOnly: true},
							},
							// Minimal resources for the monitor
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "model-download",
							Image:           downloadImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  baseutils.Pointer(int64(1000)),
								RunAsGroup: baseutils.Pointer(int64(1000)),
							},
							Env:     newEnv,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								fmt.Sprintf(`
# Bail out if this AIM_DEBUG_CAUSE_FAILURE is set
if [ -n "$AIM_DEBUG_CAUSE_FAILURE" ]; then
	echo "AIM_DEBUG_CAUSE_FAILURE is set, bailing out"
	exit 1
fi
if [ -n "$AIM_DEBUG_CAUSE_HANG" ]; then
	echo "AIM_DEBUG_CAUSE_HANG is set, will sleep for 120 minutes before exiting"
	ret=$(python -c "import time; time.sleep(7200)")
	echo "Sleep returned: $ret"
	exit $ret
fi

# Set umask so downloaded files are readable by others
umask 0022

# Create temp directories on the same filesystem as destination
mkdir -p %s/.tmp %s/.hf_home %s/.hf_cache %s/.xet_cache

# Download the model
python /storage-initializer/scripts/initializer-entrypoint %s %s &&
(
# Report sizes before cleanup
echo "Storage usage before cleanup:"
du -sh %s
du -sh %s/.cache 2>/dev/null || true

# Clean up HF cache directories to save space (keeps only final model files)
echo "Cleaning up HF cache to save space..."
rm -rf %s/.cache %s/.tmp %s/.hf_home %s/.hf_cache %s/.xet_cache 2>/dev/null || true

# Report final sizes
echo "Final storage usage:"
du -sh %s || true
)
				`, mountPath, mountPath, mountPath, mountPath, mc.Spec.SourceURI, mountPath, mountPath, mountPath, mountPath, mountPath, mountPath, mountPath, mountPath, mountPath),
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

// progressMonitorScript is the shell script for the download progress monitor sidecar.
// It reports download progress every 10 seconds in JSON format.
//
// This runs as a native sidecar (init container with restartPolicy=Always).
// Kubernetes automatically sends SIGTERM when all regular containers terminate,
// so we just need to handle the signal gracefully.
//
// JSON output types:
//   - "start": Initial message when monitor starts
//   - "progress": Periodic progress update
//   - "complete": Download finished successfully (detected via marker file)
//   - "terminated": Received SIGTERM from kubelet (main container finished)
const progressMonitorScript = `
# Handle SIGTERM gracefully - kubelet sends this when main container terminates
terminated=false
trap 'terminated=true' TERM

# Output a JSON log message
# Usage: log_json <type> [key=value ...]
log_json() {
    type=$1
    shift
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    json="{\"timestamp\":\"$timestamp\",\"type\":\"$type\""
    for kv in "$@"; do
        key="${kv%%=*}"
        value="${kv#*=}"
        # Check if value is numeric
        case "$value" in
            ''|*[!0-9]*) json="$json,\"$key\":\"$value\"" ;;  # string
            *) json="$json,\"$key\":$value" ;;                 # number
        esac
    done
    echo "$json}"
}

expected_size=${EXPECTED_SIZE_BYTES:-0}
mount_path=${MOUNT_PATH:-/cache}
interval=10
stall_timeout=300  # 5 minutes

log_json "start" "expectedBytes=$expected_size" "intervalSeconds=$interval" "stallTimeoutSeconds=$stall_timeout"

last_size=0
last_change_time=$(date +%s)

log_json "start" "expectedBytes=$expected_size" "intervalSeconds=$interval"

while true; do
    # Check if we received SIGTERM (main container terminated)
    if [ "$terminated" = "true" ]; then
        current_size=$(du -sb "$mount_path" 2>/dev/null | cut -f1 || echo 0)
        log_json "terminated" "currentBytes=$current_size" "expectedBytes=$expected_size" "message=Main container terminated"
        exit 0
    fi

    # Check if download completed successfully (marker file from main container)
    if [ -f "$mount_path/.download-complete" ]; then
        current_size=$(du -sb "$mount_path" 2>/dev/null | cut -f1 || echo 0)
        log_json "complete" "currentBytes=$current_size" "expectedBytes=$expected_size"
        exit 0
    fi

    current_size=$(du -sb "$mount_path" 2>/dev/null | cut -f1 || echo 0)
    now=$(date +%s)

    # Track progress for stall detection
    if [ "$current_size" -gt "$last_size" ]; then
        last_size=$current_size
        last_change_time=$now
    fi

    # Check for stall (no progress for stall_timeout seconds)
    stall_duration=$((now - last_change_time))
    if [ "$stall_duration" -ge "$stall_timeout" ]; then
        log_json "stall" "currentBytes=$current_size" "stallDurationSeconds=$stall_duration" "message=Download stalled, killing downloader"
        # Find and kill the python process in the model-download container
        pkill -9 -f "python" 2>/dev/null || true
        exit 0
    fi

    if [ "$expected_size" -gt 0 ] && [ "$current_size" -gt 0 ]; then
        percent=$((current_size * 100 / expected_size))
        # Cap at 100% (during download, temp files may exceed expected size)
        if [ $percent -gt 100 ]; then
            percent=100
        fi
        log_json "progress" "percent=$percent" "currentBytes=$current_size" "expectedBytes=$expected_size"
    elif [ "$current_size" -gt 0 ]; then
        log_json "progress" "currentBytes=$current_size" "expectedBytes=0" "message=Expected size unknown"
    else
        log_json "progress" "currentBytes=0" "expectedBytes=$expected_size" "message=Waiting for download to start"
    fi

    # Use a loop with short sleeps so we can check for SIGTERM more frequently
    # sleep in busybox doesn't get interrupted by signals, so we poll
    i=0
    while [ $i -lt $interval ] && [ "$terminated" = "false" ]; do
        sleep 1
        i=$((i + 1))
    done
done
`

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
