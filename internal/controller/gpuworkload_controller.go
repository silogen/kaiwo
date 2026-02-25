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

package controller

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configapi "github.com/silogen/kaiwo/apis/config/v1alpha1"
	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

const (
	AnnotationPrefix        = "kaiwo.silogen.ai/gpu-preemption."
	AnnotationEnabled       = AnnotationPrefix + "enabled"
	AnnotationThreshold     = AnnotationPrefix + "threshold"
	AnnotationIfIdleAfter   = AnnotationPrefix + "if-idle-after"
	AnnotationPolicy        = AnnotationPrefix + "policy"
	AnnotationAggregation   = AnnotationPrefix + "aggregation"
	AnnotationTTL           = AnnotationPrefix + "ttl"
	GpuWorkloadFinalizer    = "kaiwo.silogen.ai/gpu-workload-protection"
	PreemptionEvalLeaseName = "gpu-preemption-eval"
	PreemptionEvalLeaseDur  = 30 * time.Second
	IdleRequeueInterval     = 60 * time.Second
	GpuResourcePrefix       = "amd.com/gpu"
	EnvGpuPreemptionPrefix  = "GPU_PREEMPTION_"
	EnvEnabled              = EnvGpuPreemptionPrefix + "ENABLED"
	EnvDefaultThreshold     = EnvGpuPreemptionPrefix + "DEFAULT_THRESHOLD"
	EnvDefaultIfIdleAfter   = EnvGpuPreemptionPrefix + "DEFAULT_IF_IDLE_AFTER"
	EnvDefaultPolicy        = EnvGpuPreemptionPrefix + "DEFAULT_POLICY"
	EnvDefaultAggregation   = EnvGpuPreemptionPrefix + "DEFAULT_AGGREGATION"
	EnvDefaultTTL           = EnvGpuPreemptionPrefix + "DEFAULT_TTL"
	EnvOperatorNamespace    = EnvGpuPreemptionPrefix + "OPERATOR_NAMESPACE"
)

func IsGpuPreemptionEnabled() bool {
	return os.Getenv(EnvEnabled) == "true"
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=gpuworkloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=gpuworkloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=gpuworkloads/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets;statefulsets,verbs=get;list;delete
// +kubebuilder:rbac:groups=ray.io,resources=rayjobs;rayservices;rayclusters,verbs=get;list;delete
// +kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers,verbs=get;list;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwojobs;kaiwoservices,verbs=get;list;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

type GpuWorkloadReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	holderID string
}

func (r *GpuWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var gw kaiwo.GpuWorkload
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var gpuCfg configapi.KaiwoGpuPreemptionConfig
	if cfgCtx, err := common.GetContextWithConfig(ctx, r.Client); err != nil {
		logger.Error(err, "failed to fetch KaiwoConfig, proceeding with env/hardcoded defaults")
	} else {
		ctx = cfgCtx
		gpuCfg = common.ConfigFromContext(ctx).GpuPreemption
	}

	// Handle Preempting: delete the underlying workload, transition to Preempted
	if gw.Status.Phase == kaiwo.GpuWorkloadPhasePreempting {
		return r.handlePreempting(ctx, &gw)
	}

	// Handle terminal phases: check TTL and clean up
	if gw.Status.Phase == kaiwo.GpuWorkloadPhasePreempted || gw.Status.Phase == kaiwo.GpuWorkloadPhaseDeleted {
		return r.handleTerminal(ctx, &gw, gpuCfg)
	}

	// Check if the referenced workload still exists
	exists, err := r.ownerExists(ctx, &gw)
	if err != nil {
		logger.Error(err, "failed to check owner existence")
		return ctrl.Result{}, err
	}
	if !exists {
		now := metav1.Now()
		gw.Status.Phase = kaiwo.GpuWorkloadPhaseDeleted
		gw.Status.FinishedAt = &now
		gw.Status.IdleSince = nil
		gw.Status.AggregatedUtilization = nil
		gw.Status.PodUtilizations = nil
		if err := r.Status().Update(ctx, &gw); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&gw, corev1.EventTypeNormal, "WorkloadDeleted",
			"Underlying %s/%s no longer exists; tracking will be retained until TTL expires",
			gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name)
		return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
	}

	ownedPods, requeue, err := r.syncOwnedPods(ctx, req, &gw)
	if requeue {
		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	oldPhase := gw.Status.Phase

	// Phase computation
	phase := r.computePhase(ctx, &gw, ownedPods, gpuCfg)
	gw.Status.Phase = phase

	// Update IdleSince tracking
	if phase == kaiwo.GpuWorkloadPhaseIdle {
		if gw.Status.IdleSince == nil {
			now := metav1.Now()
			gw.Status.IdleSince = &now
		}
	} else {
		gw.Status.IdleSince = nil
	}

	if err := r.Status().Update(ctx, &gw); err != nil {
		if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	// Emit events on phase transitions
	phaseChanged := oldPhase != phase
	if phaseChanged {
		r.emitPhaseTransitionEvent(&gw, oldPhase, phase)
	}

	if phaseChanged && (phase == kaiwo.GpuWorkloadPhaseIdle || phase == kaiwo.GpuWorkloadPhasePendingGpu || phase == kaiwo.GpuWorkloadPhaseActive) {
		if err := r.tryRunPreemptionEvaluation(ctx, &gw, gpuCfg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if phase == kaiwo.GpuWorkloadPhaseIdle {
		if err := r.tryRunPreemptionEvaluation(ctx, &gw, gpuCfg); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
	}

	return ctrl.Result{}, nil
}

func (r *GpuWorkloadReconciler) computePhase(_ context.Context, gw *kaiwo.GpuWorkload, pods []corev1.Pod, gpuCfg configapi.KaiwoGpuPreemptionConfig) kaiwo.GpuWorkloadPhase {
	if len(pods) == 0 {
		if gw.Status.Phase == "" {
			return kaiwo.GpuWorkloadPhasePendingOther
		}
		// No pods but owner still exists (checked before this function) --
		// keep the current phase and let the next reconcile sort it out.
		return gw.Status.Phase
	}

	// Check if any pods are pending specifically due to GPU insufficiency.
	// This is the demand signal for OnPressure preemption.
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodPending && isPendingDueToGPU(&pods[i], gw.Spec.GpuResources) {
			return kaiwo.GpuWorkloadPhasePendingGpu
		}
	}

	// Only evaluate utilization once at least one pod is Running.
	// Pods in ContainerCreating, pending for PVCs/affinity/etc., or running
	// init containers are PendingOther -- not idle, not a GPU demand signal.
	hasRunningPod := false
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodRunning {
			hasRunningPod = true
			break
		}
	}
	if !hasRunningPod {
		return kaiwo.GpuWorkloadPhasePendingOther
	}

	// Compute aggregated utilization from status.podUtilizations
	aggUtil := r.computeAggregatedUtilization(gw, gpuCfg)
	gw.Status.AggregatedUtilization = aggUtil

	if aggUtil == nil {
		return kaiwo.GpuWorkloadPhaseActive
	}

	threshold := r.getThreshold(gw, gpuCfg)
	if *aggUtil >= threshold {
		return kaiwo.GpuWorkloadPhaseActive
	}
	return kaiwo.GpuWorkloadPhaseIdle
}

func isPendingDueToGPU(pod *corev1.Pod, gpuResources map[string]int) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Reason == "Unschedulable" {
			msg := strings.ToLower(cond.Message)
			for resourceName := range gpuResources {
				if strings.Contains(msg, strings.ToLower("Insufficient "+resourceName)) {
					return true
				}
			}
			if strings.Contains(msg, strings.ToLower("Insufficient "+GpuResourcePrefix)) {
				return true
			}
		}
	}
	return false
}

func (r *GpuWorkloadReconciler) computeAggregatedUtilization(gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) *float64 {
	if len(gw.Status.PodUtilizations) == 0 {
		return nil
	}

	policy := r.getAggregationPolicy(gw, gpuCfg)

	podUtils := make(map[string][]float64)
	for _, pu := range gw.Status.PodUtilizations {
		podUtils[pu.PodName] = append(podUtils[pu.PodName], pu.Utilization)
	}

	var podAverages []float64
	for _, gpuUtils := range podUtils {
		sum := 0.0
		for _, u := range gpuUtils {
			sum += u
		}
		podAverages = append(podAverages, sum/float64(len(gpuUtils)))
	}

	if len(podAverages) == 0 {
		return nil
	}

	var result float64
	switch policy {
	case kaiwo.AggregationPolicyMin:
		result = podAverages[0]
		for _, v := range podAverages[1:] {
			if v < result {
				result = v
			}
		}
	case kaiwo.AggregationPolicyMax:
		result = podAverages[0]
		for _, v := range podAverages[1:] {
			if v > result {
				result = v
			}
		}
	case kaiwo.AggregationPolicyAvg:
		sum := 0.0
		for _, v := range podAverages {
			sum += v
		}
		result = sum / float64(len(podAverages))
	default:
		sum := 0.0
		for _, v := range podAverages {
			sum += v
		}
		result = sum / float64(len(podAverages))
	}

	result = math.Round(result*100) / 100
	return &result
}

func (r *GpuWorkloadReconciler) pruneStaleUtilizations(gw *kaiwo.GpuWorkload, ownedPods []corev1.Pod) {
	if len(gw.Status.PodUtilizations) == 0 {
		return
	}
	activePods := make(map[string]bool, len(ownedPods))
	for i := range ownedPods {
		activePods[ownedPods[i].Name] = true
	}
	kept := gw.Status.PodUtilizations[:0]
	for _, pu := range gw.Status.PodUtilizations {
		if activePods[pu.PodName] {
			kept = append(kept, pu)
		}
	}
	gw.Status.PodUtilizations = kept
	if len(kept) == 0 {
		gw.Status.AggregatedUtilization = nil
	}
}

func (r *GpuWorkloadReconciler) handlePreempting(ctx context.Context, gw *kaiwo.GpuWorkload) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if err := r.deleteWorkload(ctx, gw); err != nil {
		if errors.IsForbidden(err) {
			// We don't have RBAC to delete this resource type. Record the
			// failure as an event and revert to Idle so the evaluator doesn't
			// keep retrying against a resource we can never delete.
			logger.Error(err, "RBAC forbids deleting workload, reverting to Idle",
				"kind", gw.Spec.WorkloadRef.Kind, "name", gw.Spec.WorkloadRef.Name)
			r.Recorder.Eventf(gw, corev1.EventTypeWarning, "PreemptionFailed",
				"Cannot delete %s/%s: the operator lacks RBAC delete permission for %s resources",
				gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name, gw.Spec.WorkloadRef.Kind)
			gw.Status.Phase = kaiwo.GpuWorkloadPhaseIdle
			gw.Status.PreemptedFor = ""
			gw.Status.PreemptionReason = ""
			if err := r.Status().Update(ctx, gw); err != nil {
				if errors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
		}
		if !errors.IsNotFound(err) {
			logger.Error(err, "failed to delete workload for preemption")
			return ctrl.Result{}, err
		}
	}

	now := metav1.Now()
	gw.Status.Phase = kaiwo.GpuWorkloadPhasePreempted
	gw.Status.PreemptedAt = &now
	gw.Status.FinishedAt = &now
	gw.Status.IdleSince = nil
	gw.Status.AggregatedUtilization = nil
	gw.Status.PodUtilizations = nil

	r.Recorder.Eventf(gw, corev1.EventTypeWarning, "WorkloadPreempted",
		"%s/%s was deleted to free GPU resources; reason: %s",
		gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name,
		gw.Status.PreemptionReason)

	if err := r.Status().Update(ctx, gw); err != nil {
		if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
}

func (r *GpuWorkloadReconciler) handleTerminal(ctx context.Context, gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) (ctrl.Result, error) {
	ttl := r.getTTL(gw, gpuCfg)
	if ttl == 0 {
		return ctrl.Result{}, nil
	}

	if gw.Status.FinishedAt == nil {
		return ctrl.Result{}, nil
	}

	elapsed := time.Since(gw.Status.FinishedAt.Time)
	if elapsed >= ttl {
		if err := r.Delete(ctx, gw); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: ttl - elapsed}, nil
}

// syncOwnedPods lists pods in the workload's namespace, filters by ownership,
// recomputes the workload-wide GPU resource total in the spec, and prunes
// stale utilization entries. If the spec update hits an optimistic concurrency
// conflict, requeue is returned as true so the caller can retry.
func (r *GpuWorkloadReconciler) syncOwnedPods(ctx context.Context, req ctrl.Request, gw *kaiwo.GpuWorkload) (pods []corev1.Pod, requeue bool, err error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(gw.Namespace)); err != nil {
		return nil, false, err
	}

	var ownedPods []corev1.Pod
	for i := range podList.Items {
		if r.isPodOwnedByWorkload(ctx, &podList.Items[i], gw) {
			ownedPods = append(ownedPods, podList.Items[i])
		}
	}

	if len(ownedPods) > 0 {
		totalGpu := computeTotalGpuResources(ownedPods)
		if !gpuResourcesEqual(gw.Spec.GpuResources, totalGpu) {
			gw.Spec.GpuResources = totalGpu
			if err := r.Update(ctx, gw); err != nil {
				if errors.IsConflict(err) {
					return nil, true, nil
				}
				return nil, false, err
			}
			if err := r.Get(ctx, req.NamespacedName, gw); err != nil {
				return nil, false, client.IgnoreNotFound(err)
			}
		}
	}

	r.pruneStaleUtilizations(gw, ownedPods)
	return ownedPods, false, nil
}

func (r *GpuWorkloadReconciler) deleteWorkload(ctx context.Context, gw *kaiwo.GpuWorkload) error {
	ref := gw.Spec.WorkloadRef
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvkFromAPIVersionKind(ref.APIVersion, ref.Kind))
	obj.SetName(ref.Name)
	obj.SetNamespace(gw.Namespace)
	return r.Delete(ctx, obj)
}

func (r *GpuWorkloadReconciler) ownerExists(ctx context.Context, gw *kaiwo.GpuWorkload) (bool, error) {
	ref := gw.Spec.WorkloadRef
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvkFromAPIVersionKind(ref.APIVersion, ref.Kind))
	err := r.Get(ctx, client.ObjectKey{Namespace: gw.Namespace, Name: ref.Name}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return string(obj.GetUID()) == string(ref.UID), nil
}

// isPodOwnedByWorkload checks if a pod belongs to the workload tracked by a
// GpuWorkload CR. It handles bare pods (direct UID match), single-hop owners
// (Job, StatefulSet), and multi-level hierarchies (Deployment -> ReplicaSet,
// KaiwoJob -> RayCluster -> ...) by walking the controller ownerReference
// chain. The expensive chain walk is only performed for pods that request GPU
// resources, since non-GPU pods cannot belong to a GPU workload.
func (r *GpuWorkloadReconciler) isPodOwnedByWorkload(ctx context.Context, pod *corev1.Pod, gw *kaiwo.GpuWorkload) bool {
	targetUID := gw.Spec.WorkloadRef.UID

	// Bare pod: the workloadRef points to the pod itself.
	if gw.Spec.WorkloadRef.Kind == "Pod" && pod.UID == targetUID {
		return true
	}

	// Single-hop: pod's direct ownerReference matches (Job, StatefulSet).
	for _, ref := range pod.OwnerReferences {
		if ref.UID == targetUID {
			return true
		}
	}

	// Multi-hop: walk the controller owner chain upward (Deployment, RayJob, etc.).
	// Only worth the API calls for pods that request GPU resources.
	if hasGpuResources(pod) {
		if controllerRef := getControllerOwnerRef(pod.OwnerReferences); controllerRef != nil {
			if r.isAncestorOf(ctx, pod.Namespace, controllerRef, targetUID) {
				return true
			}
		}
	}

	return false
}

// isAncestorOf walks from startRef up the controller ownerReference chain,
// returning true if targetUID appears as an ownerReference at any level.
func (r *GpuWorkloadReconciler) isAncestorOf(ctx context.Context, namespace string, startRef *metav1.OwnerReference, targetUID types.UID) bool {
	seen := map[types.UID]bool{startRef.UID: true}
	currentRef := startRef

	for i := 0; i < maxOwnerDepth && currentRef != nil; i++ {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvkFromAPIVersionKind(currentRef.APIVersion, currentRef.Kind))
		if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: currentRef.Name}, obj); err != nil {
			return false
		}

		for _, ref := range obj.GetOwnerReferences() {
			if ref.UID == targetUID {
				return true
			}
		}

		nextRef := getControllerOwnerRef(obj.GetOwnerReferences())
		if nextRef == nil || seen[nextRef.UID] {
			return false
		}
		seen[nextRef.UID] = true
		currentRef = nextRef
	}
	return false
}

func hasGpuResources(pod *corev1.Pod) bool {
	for _, c := range pod.Spec.Containers {
		for resName := range c.Resources.Requests {
			if strings.HasPrefix(string(resName), GpuResourcePrefix) {
				return true
			}
		}
		for resName := range c.Resources.Limits {
			if strings.HasPrefix(string(resName), GpuResourcePrefix) {
				return true
			}
		}
	}
	return false
}

func (r *GpuWorkloadReconciler) emitPhaseTransitionEvent(gw *kaiwo.GpuWorkload, oldPhase, newPhase kaiwo.GpuWorkloadPhase) {
	var eventType, reason, message string

	switch newPhase {
	case kaiwo.GpuWorkloadPhaseIdle:
		eventType = corev1.EventTypeNormal
		reason = "WorkloadIdle"
		if gw.Status.AggregatedUtilization != nil {
			message = fmt.Sprintf("GPU utilization dropped to %.1f%%, workload is now considered idle", *gw.Status.AggregatedUtilization)
		} else {
			message = "GPU utilization is below threshold, workload is now considered idle"
		}

	case kaiwo.GpuWorkloadPhaseActive:
		eventType = corev1.EventTypeNormal
		reason = "WorkloadActive"
		if gw.Status.AggregatedUtilization != nil {
			message = fmt.Sprintf("GPU utilization is %.1f%%, workload is actively using GPUs", *gw.Status.AggregatedUtilization)
		} else {
			message = "Workload is actively using GPUs"
		}

	case kaiwo.GpuWorkloadPhasePendingGpu:
		eventType = corev1.EventTypeWarning
		reason = "WorkloadPendingGpu"
		message = "Pods cannot be scheduled due to insufficient GPU resources"

	case kaiwo.GpuWorkloadPhasePendingOther:
		eventType = corev1.EventTypeNormal
		reason = "WorkloadPending"
		message = "Waiting for pods to become ready (image pull, volume binding, init containers, etc.)"

	case kaiwo.GpuWorkloadPhasePreempting:
		eventType = corev1.EventTypeWarning
		reason = "PreemptionStarted"
		message = fmt.Sprintf("Preemption initiated: %s", gw.Status.PreemptionReason)

	case kaiwo.GpuWorkloadPhasePreempted:
		eventType = corev1.EventTypeWarning
		reason = "WorkloadPreempted"
		message = fmt.Sprintf("Workload %s/%s was preempted", gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name)
		if gw.Status.PreemptedFor != "" {
			message += fmt.Sprintf(" to free GPUs for %s", gw.Status.PreemptedFor)
		}

	case kaiwo.GpuWorkloadPhaseDeleted:
		eventType = corev1.EventTypeNormal
		reason = "WorkloadDeleted"
		message = fmt.Sprintf("Underlying %s/%s no longer exists", gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name)

	default:
		return
	}

	r.Recorder.Event(gw, eventType, reason, message)
}

// --- Preemption Evaluation ---

type workloadEntry struct {
	gw        *kaiwo.GpuWorkload
	gpuCount  map[string]int
	idleSince time.Time
}

type preemptionState struct {
	pendingByResource map[string][]workloadEntry
	idleByResource    map[string][]workloadEntry
	// inFlight tracks GPU capacity already being freed for each pending
	// workload (by name), keyed by resource name, preventing over-preemption
	// across evaluation cycles.
	inFlight map[string]map[string]int
}

func (r *GpuWorkloadReconciler) tryRunPreemptionEvaluation(ctx context.Context, _ *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) error {
	logger := log.FromContext(ctx)

	if !r.tryAcquireLease(ctx) {
		logger.V(1).Info("another replica holds the preemption evaluation lease")
		return nil
	}
	defer r.releaseLease(ctx)

	var allWorkloads kaiwo.GpuWorkloadList
	if err := r.List(ctx, &allWorkloads); err != nil {
		return fmt.Errorf("failed to list GpuWorkloads: %w", err)
	}

	state, err := r.classifyWorkloads(ctx, allWorkloads.Items, gpuCfg)
	if err != nil {
		return err
	}

	return r.matchAndMarkVictims(ctx, state)
}

// classifyWorkloads partitions all GpuWorkloads into pending, idle, and
// in-flight buckets. Always-policy workloads that exceed their idle duration
// are marked for preemption immediately. Both pending and idle lists are
// sorted for deterministic victim selection.
func (r *GpuWorkloadReconciler) classifyWorkloads(ctx context.Context, workloads []kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) (*preemptionState, error) {
	logger := log.FromContext(ctx)

	state := &preemptionState{
		pendingByResource: make(map[string][]workloadEntry),
		idleByResource:    make(map[string][]workloadEntry),
		inFlight:          make(map[string]map[string]int),
	}

	for i := range workloads {
		w := &workloads[i]
		policy := r.getPreemptionPolicy(w, gpuCfg)

		switch w.Status.Phase {
		case kaiwo.GpuWorkloadPhasePendingGpu:
			for resName := range w.Spec.GpuResources {
				state.pendingByResource[resName] = append(state.pendingByResource[resName], workloadEntry{
					gw:       w,
					gpuCount: w.Spec.GpuResources,
				})
			}

		case kaiwo.GpuWorkloadPhasePreempting:
			if w.Status.PreemptedFor != "" {
				if state.inFlight[w.Status.PreemptedFor] == nil {
					state.inFlight[w.Status.PreemptedFor] = make(map[string]int)
				}
				for resName, count := range w.Spec.GpuResources {
					state.inFlight[w.Status.PreemptedFor][resName] += count
				}
			}

		case kaiwo.GpuWorkloadPhaseIdle:
			if w.Status.IdleSince == nil {
				continue
			}

			ifIdleAfter := r.getIfIdleAfter(w, gpuCfg)
			idleDuration := time.Since(w.Status.IdleSince.Time)
			if idleDuration < ifIdleAfter {
				continue
			}

			if policy == kaiwo.PreemptionPolicyAlways {
				w.Status.Phase = kaiwo.GpuWorkloadPhasePreempting
				w.Status.IdleSince = nil
				w.Status.PreemptionReason = fmt.Sprintf(
					"Policy is Always and workload has been idle for %s (threshold: %s)",
					idleDuration.Round(time.Second), ifIdleAfter,
				)
				if err := r.Status().Update(ctx, w); err != nil {
					if errors.IsConflict(err) {
						logger.V(1).Info("conflict marking Always-policy workload, skipping", "name", w.Name)
						continue
					}
					return nil, err
				}
				r.Recorder.Eventf(w, corev1.EventTypeWarning, "PreemptionStarted",
					"Workload has been idle for %s (limit: %s, policy: Always); preempting",
					idleDuration.Round(time.Second), ifIdleAfter)
				continue
			}

			for resName := range w.Spec.GpuResources {
				state.idleByResource[resName] = append(state.idleByResource[resName], workloadEntry{
					gw:        w,
					gpuCount:  w.Spec.GpuResources,
					idleSince: w.Status.IdleSince.Time,
				})
			}
		}
	}

	for resName := range state.idleByResource {
		sort.Slice(state.idleByResource[resName], func(i, j int) bool {
			return state.idleByResource[resName][i].idleSince.Before(state.idleByResource[resName][j].idleSince)
		})
	}
	for resName := range state.pendingByResource {
		sort.Slice(state.pendingByResource[resName], func(i, j int) bool {
			return state.pendingByResource[resName][i].gw.CreationTimestamp.Before(&state.pendingByResource[resName][j].gw.CreationTimestamp)
		})
	}

	return state, nil
}

// matchAndMarkVictims walks pending workloads (oldest first) and accumulates
// idle victims (longest-idle first) until each pending workload's GPU demand
// is satisfied. Victims are marked Preempting only when the combined capacity
// meets or exceeds the demand (all-or-nothing). The claimed map prevents
// double-claiming a victim for multiple pending workloads.
func (r *GpuWorkloadReconciler) matchAndMarkVictims(ctx context.Context, state *preemptionState) error {
	logger := log.FromContext(ctx)
	claimed := make(map[types.UID]bool)

	for resName, pendingList := range state.pendingByResource {
		idlePool, ok := state.idleByResource[resName]
		if !ok || len(idlePool) == 0 {
			continue
		}

		for _, pending := range pendingList {
			demandCount := pending.gpuCount[resName]
			if demandCount <= 0 {
				continue
			}

			if alreadyFreeing, ok := state.inFlight[pending.gw.Name]; ok {
				demandCount -= alreadyFreeing[resName]
				if demandCount <= 0 {
					continue
				}
			}

			var victims []workloadEntry
			accumulated := 0

			for _, idle := range idlePool {
				if claimed[idle.gw.Spec.WorkloadRef.UID] {
					continue
				}
				count := idle.gpuCount[resName]
				if count <= 0 {
					continue
				}
				victims = append(victims, idle)
				accumulated += count
				if accumulated >= demandCount {
					break
				}
			}

			if accumulated < demandCount {
				continue
			}

			for _, victim := range victims {
				claimed[victim.gw.Spec.WorkloadRef.UID] = true

				victim.gw.Status.Phase = kaiwo.GpuWorkloadPhasePreempting
				victim.gw.Status.IdleSince = nil
				victim.gw.Status.PreemptedFor = pending.gw.Name
				victim.gw.Status.PreemptionReason = fmt.Sprintf(
					"GPU pressure: pending workload %s needs %d %s GPUs",
					pending.gw.Name, demandCount, resName,
				)

				if err := r.Status().Update(ctx, victim.gw); err != nil {
					if errors.IsConflict(err) {
						logger.V(1).Info("conflict marking victim, skipping", "victim", victim.gw.Name)
						continue
					}
					return err
				}

				r.Recorder.Eventf(victim.gw, corev1.EventTypeWarning, "PreemptionStarted",
					"Freeing %d %s GPU(s) for pending workload %s",
					victim.gpuCount[resName], resName, pending.gw.Name)
				logger.Info("marked workload for preemption",
					"victim", victim.gw.Name,
					"for", pending.gw.Name,
					"resource", resName)
			}
		}
	}

	return nil
}

// --- Lease Management ---

func (r *GpuWorkloadReconciler) getOperatorNamespace() string {
	ns := os.Getenv(EnvOperatorNamespace)
	if ns == "" {
		ns = "kaiwo-system"
	}
	return ns
}

func (r *GpuWorkloadReconciler) tryAcquireLease(ctx context.Context) bool {
	logger := log.FromContext(ctx)
	ns := r.getOperatorNamespace()

	lease := &coordinationv1.Lease{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: PreemptionEvalLeaseName}, lease)

	now := metav1.NewMicroTime(time.Now())
	leaseDur := int32(PreemptionEvalLeaseDur.Seconds())

	if errors.IsNotFound(err) {
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PreemptionEvalLeaseName,
				Namespace: ns,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &r.holderID,
				LeaseDurationSeconds: &leaseDur,
				AcquireTime:          &now,
				RenewTime:            &now,
			},
		}
		if err := r.Create(ctx, lease); err != nil {
			logger.V(1).Info("failed to create lease", "error", err)
			return false
		}
		return true
	}
	if err != nil {
		logger.V(1).Info("failed to get lease", "error", err)
		return false
	}

	// Check if lease is expired or held by us
	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != r.holderID {
		if lease.Spec.RenewTime != nil {
			dur := time.Duration(0)
			if lease.Spec.LeaseDurationSeconds != nil {
				dur = time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
			}
			if time.Since(lease.Spec.RenewTime.Time) < dur {
				return false
			}
		}
	}

	lease.Spec.HolderIdentity = &r.holderID
	lease.Spec.LeaseDurationSeconds = &leaseDur
	lease.Spec.AcquireTime = &now
	lease.Spec.RenewTime = &now

	if err := r.Update(ctx, lease); err != nil {
		logger.V(1).Info("failed to update lease", "error", err)
		return false
	}
	return true
}

func (r *GpuWorkloadReconciler) releaseLease(ctx context.Context) {
	logger := log.FromContext(ctx)
	ns := r.getOperatorNamespace()

	lease := &coordinationv1.Lease{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: PreemptionEvalLeaseName}, lease); err != nil {
		logger.V(1).Info("failed to get lease for release", "error", err)
		return
	}

	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != r.holderID {
		return
	}

	empty := ""
	lease.Spec.HolderIdentity = &empty
	if err := r.Update(ctx, lease); err != nil {
		logger.V(1).Info("failed to release lease", "error", err)
	}
}

// --- Configuration Helpers ---

func (r *GpuWorkloadReconciler) getThreshold(gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) float64 {
	if gw.Spec.UtilizationThreshold != nil {
		return *gw.Spec.UtilizationThreshold
	}
	if gpuCfg.DefaultThreshold != nil {
		return *gpuCfg.DefaultThreshold
	}
	val, err := strconv.ParseFloat(os.Getenv(EnvDefaultThreshold), 64)
	if err != nil {
		return 5.0
	}
	return val
}

func (r *GpuWorkloadReconciler) getIfIdleAfter(gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) time.Duration {
	if gw.Spec.IfIdleAfter != nil {
		return gw.Spec.IfIdleAfter.Duration
	}
	if gpuCfg.DefaultIfIdleAfter != "" {
		if d, err := time.ParseDuration(gpuCfg.DefaultIfIdleAfter); err == nil {
			return d
		}
	}
	val, err := time.ParseDuration(os.Getenv(EnvDefaultIfIdleAfter))
	if err != nil {
		return 10 * time.Minute
	}
	return val
}

func (r *GpuWorkloadReconciler) getPreemptionPolicy(gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) kaiwo.PreemptionPolicy {
	if gw.Spec.PreemptionPolicy != nil {
		return *gw.Spec.PreemptionPolicy
	}
	if gpuCfg.DefaultPolicy != "" {
		if gpuCfg.DefaultPolicy == string(kaiwo.PreemptionPolicyAlways) {
			return kaiwo.PreemptionPolicyAlways
		}
		return kaiwo.PreemptionPolicyOnPressure
	}
	val := os.Getenv(EnvDefaultPolicy)
	if val == string(kaiwo.PreemptionPolicyAlways) {
		return kaiwo.PreemptionPolicyAlways
	}
	return kaiwo.PreemptionPolicyOnPressure
}

func (r *GpuWorkloadReconciler) getAggregationPolicy(gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) kaiwo.AggregationPolicy {
	if gw.Spec.AggregationPolicy != nil {
		return *gw.Spec.AggregationPolicy
	}
	if gpuCfg.DefaultAggregation != "" {
		switch kaiwo.AggregationPolicy(gpuCfg.DefaultAggregation) {
		case kaiwo.AggregationPolicyMin:
			return kaiwo.AggregationPolicyMin
		case kaiwo.AggregationPolicyAvg:
			return kaiwo.AggregationPolicyAvg
		case kaiwo.AggregationPolicyMax:
			return kaiwo.AggregationPolicyMax
		}
	}
	val := os.Getenv(EnvDefaultAggregation)
	switch kaiwo.AggregationPolicy(val) {
	case kaiwo.AggregationPolicyMin:
		return kaiwo.AggregationPolicyMin
	case kaiwo.AggregationPolicyAvg:
		return kaiwo.AggregationPolicyAvg
	default:
		return kaiwo.AggregationPolicyMax
	}
}

func (r *GpuWorkloadReconciler) getTTL(gw *kaiwo.GpuWorkload, gpuCfg configapi.KaiwoGpuPreemptionConfig) time.Duration {
	if gw.Spec.TTLAfterFinished != nil {
		return gw.Spec.TTLAfterFinished.Duration
	}
	if gpuCfg.DefaultTTL != "" {
		if d, err := time.ParseDuration(gpuCfg.DefaultTTL); err == nil {
			return d
		}
	}
	val, err := time.ParseDuration(os.Getenv(EnvDefaultTTL))
	if err != nil {
		return 24 * time.Hour
	}
	return val
}

// --- Pod Event Handling & Discovery ---

func (r *GpuWorkloadReconciler) podToGpuWorkload(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	gpuResources := extractGpuResources(pod)
	if len(gpuResources) == 0 {
		return nil
	}

	rootResult, err := ResolveRootOwner(ctx, r.Client, pod.Namespace, pod.Name, "Pod", "v1", pod.UID)
	if err != nil {
		logger.Error(err, "failed to resolve root owner", "pod", pod.Name)
		return nil
	}

	rootObj := &unstructured.Unstructured{}
	rootObj.SetGroupVersionKind(gvkFromAPIVersionKind(rootResult.Ref.APIVersion, rootResult.Ref.Kind))
	if err := r.Get(ctx, client.ObjectKey{Namespace: rootResult.Namespace, Name: rootResult.Ref.Name}, rootObj); err != nil {
		logger.V(1).Info("could not fetch root owner", "error", err)
		return nil
	}

	annotations := rootObj.GetAnnotations()
	if !isGpuPreemptionAnnotated(annotations) {
		return nil
	}

	gwName := GpuWorkloadName(rootResult.Ref.Kind, rootResult.Ref.Name, rootResult.Ref.UID)

	var existing kaiwo.GpuWorkload
	err = r.Get(ctx, client.ObjectKey{Namespace: rootResult.Namespace, Name: gwName}, &existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "failed to check for existing GpuWorkload")
			return nil
		}

		gw := &kaiwo.GpuWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gwName,
				Namespace: rootResult.Namespace,
			},
			Spec: kaiwo.GpuWorkloadSpec{
				WorkloadRef:  rootResult.Ref,
				GpuResources: gpuResources,
			},
		}

		parseAnnotationsIntoSpec(annotations, &gw.Spec)

		if err := r.Create(ctx, gw); err != nil {
			if !errors.IsAlreadyExists(err) {
				logger.Error(err, "failed to create GpuWorkload", "name", gwName)
			}
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: rootResult.Namespace, Name: gwName}}}
		}

		gw.Status.Phase = kaiwo.GpuWorkloadPhasePendingOther
		gw.Status.OwnerChain = rootResult.OwnerChain
		if err := r.Status().Update(ctx, gw); err != nil {
			if !errors.IsConflict(err) {
				logger.Error(err, "failed to set initial status", "name", gwName)
			}
		}

		logger.Info("created GpuWorkload", "name", gwName, "owner", rootResult.Ref.Kind+"/"+rootResult.Ref.Name)
	} else {
		// Update gpuResources if new resource types appeared
		updated := false
		for resName, count := range gpuResources {
			if existing.Spec.GpuResources[resName] < count {
				if existing.Spec.GpuResources == nil {
					existing.Spec.GpuResources = make(map[string]int)
				}
				existing.Spec.GpuResources[resName] = count
				updated = true
			}
		}
		if updated {
			if err := r.Update(ctx, &existing); err != nil {
				logger.Error(err, "failed to update GpuWorkload gpuResources")
			}
		}
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: rootResult.Namespace, Name: gwName}}}
}

func extractGpuResources(pod *corev1.Pod) map[string]int {
	resources := make(map[string]int)
	for _, c := range pod.Spec.Containers {
		for resName, qty := range c.Resources.Requests {
			if strings.HasPrefix(string(resName), GpuResourcePrefix) {
				resources[string(resName)] += int(qty.Value())
			}
		}
		for resName, qty := range c.Resources.Limits {
			if strings.HasPrefix(string(resName), GpuResourcePrefix) {
				if current, exists := resources[string(resName)]; !exists || int(qty.Value()) > current {
					resources[string(resName)] = int(qty.Value())
				}
			}
		}
	}
	for _, c := range pod.Spec.InitContainers {
		for resName, qty := range c.Resources.Requests {
			if strings.HasPrefix(string(resName), GpuResourcePrefix) {
				if current, exists := resources[string(resName)]; !exists || int(qty.Value()) > current {
					resources[string(resName)] = int(qty.Value())
				}
			}
		}
	}
	return resources
}

// computeTotalGpuResources sums GPU resource requests across all pods to get
// the workload-wide total (e.g. 4 pods each requesting 1 GPU = 4 total).
func computeTotalGpuResources(pods []corev1.Pod) map[string]int {
	total := make(map[string]int)
	for i := range pods {
		for name, count := range extractGpuResources(&pods[i]) {
			total[name] += count
		}
	}
	return total
}

func gpuResourcesEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// isGpuPreemptionAnnotated returns true if the resource has the explicit
// enabled annotation OR any other gpu-preemption annotation (implicit enable).
func isGpuPreemptionAnnotated(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	for k := range annotations {
		if strings.HasPrefix(k, AnnotationPrefix) {
			return true
		}
	}
	return false
}

func parseAnnotationsIntoSpec(annotations map[string]string, spec *kaiwo.GpuWorkloadSpec) {
	if v, ok := annotations[AnnotationThreshold]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			spec.UtilizationThreshold = &f
		}
	}
	if v, ok := annotations[AnnotationIfIdleAfter]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			dur := metav1.Duration{Duration: d}
			spec.IfIdleAfter = &dur
		}
	}
	if v, ok := annotations[AnnotationPolicy]; ok {
		p := kaiwo.PreemptionPolicy(v)
		if p == kaiwo.PreemptionPolicyAlways || p == kaiwo.PreemptionPolicyOnPressure {
			spec.PreemptionPolicy = &p
		}
	}
	if v, ok := annotations[AnnotationAggregation]; ok {
		a := kaiwo.AggregationPolicy(v)
		if a == kaiwo.AggregationPolicyMin || a == kaiwo.AggregationPolicyMax || a == kaiwo.AggregationPolicyAvg {
			spec.AggregationPolicy = &a
		}
	}
	if v, ok := annotations[AnnotationTTL]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			dur := metav1.Duration{Duration: d}
			spec.TTLAfterFinished = &dur
		}
	}
}

// --- SetupWithManager ---

func (r *GpuWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	hostname, _ := os.Hostname()
	r.holderID = fmt.Sprintf("gpuworkload-%s-%d", hostname, os.Getpid())

	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwo.GpuWorkload{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.podToGpuWorkload)).
		Complete(r)
}
