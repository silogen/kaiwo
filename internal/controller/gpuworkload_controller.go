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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

const (
	AnnotationPrefix        = "kaiwo.silogen.ai/gpu-preemption."
	AnnotationEnabled       = AnnotationPrefix + "enabled"
	AnnotationThreshold     = AnnotationPrefix + "threshold"
	AnnotationGracePeriod   = AnnotationPrefix + "grace-period"
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
	EnvDefaultGracePeriod   = EnvGpuPreemptionPrefix + "DEFAULT_GRACE_PERIOD"
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
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update
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

	// Handle Preempting: delete the underlying workload, transition to Preempted
	if gw.Status.Phase == kaiwo.GpuWorkloadPhasePreempting {
		return r.handlePreempting(ctx, &gw)
	}

	// Handle terminal phases: check TTL and clean up
	if gw.Status.Phase == kaiwo.GpuWorkloadPhasePreempted || gw.Status.Phase == kaiwo.GpuWorkloadPhaseDeleted {
		return r.handleTerminal(ctx, &gw)
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
		if err := r.Status().Update(ctx, &gw); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
	}

	// List pods belonging to this workload
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(gw.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	var ownedPods []corev1.Pod
	for i := range podList.Items {
		if isPodOwnedByWorkload(&podList.Items[i], &gw) {
			ownedPods = append(ownedPods, podList.Items[i])
		}
	}

	oldPhase := gw.Status.Phase

	// Phase computation
	phase := r.computePhase(ctx, &gw, ownedPods)
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
		return ctrl.Result{}, err
	}

	// Trigger preemption evaluation on relevant phase transitions
	phaseChanged := oldPhase != phase
	if phaseChanged && (phase == kaiwo.GpuWorkloadPhaseIdle || phase == kaiwo.GpuWorkloadPhasePending || phase == kaiwo.GpuWorkloadPhaseActive) {
		if err := r.tryRunPreemptionEvaluation(ctx, &gw); err != nil {
			logger.Error(err, "preemption evaluation failed")
		}
	}

	if phase == kaiwo.GpuWorkloadPhaseIdle {
		return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
	}

	return ctrl.Result{}, nil
}

func (r *GpuWorkloadReconciler) computePhase(_ context.Context, gw *kaiwo.GpuWorkload, pods []corev1.Pod) kaiwo.GpuWorkloadPhase {
	if len(pods) == 0 {
		if gw.Status.Phase == "" {
			return kaiwo.GpuWorkloadPhasePending
		}
		return gw.Status.Phase
	}

	// Check if any pods are pending due to GPU insufficiency
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodPending && isPendingDueToGPU(&pods[i], gw.Spec.GpuResources) {
			return kaiwo.GpuWorkloadPhasePending
		}
	}

	// Compute aggregated utilization from status.podUtilizations
	aggUtil := r.computeAggregatedUtilization(gw)
	gw.Status.AggregatedUtilization = aggUtil

	if aggUtil == nil {
		// No utilization data yet; if pods are running, consider Active until data arrives
		for i := range pods {
			if pods[i].Status.Phase == corev1.PodRunning {
				return kaiwo.GpuWorkloadPhaseActive
			}
		}
		return gw.Status.Phase
	}

	threshold := r.getThreshold(gw)
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

func (r *GpuWorkloadReconciler) computeAggregatedUtilization(gw *kaiwo.GpuWorkload) *float64 {
	if len(gw.Status.PodUtilizations) == 0 {
		return nil
	}

	policy := r.getAggregationPolicy(gw)

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
				"Cannot delete %s/%s: insufficient RBAC permissions. "+
					"Add delete permission for %s to the operator ServiceAccount.",
				gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name, gw.Spec.WorkloadRef.Kind)
			gw.Status.Phase = kaiwo.GpuWorkloadPhaseIdle
			gw.Status.PreemptedFor = ""
			gw.Status.PreemptionReason = ""
			if err := r.Status().Update(ctx, gw); err != nil {
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

	r.Recorder.Eventf(gw, corev1.EventTypeWarning, "Preempted",
		"Workload %s/%s preempted for %s: %s",
		gw.Spec.WorkloadRef.Kind, gw.Spec.WorkloadRef.Name,
		gw.Status.PreemptedFor, gw.Status.PreemptionReason)

	if err := r.Status().Update(ctx, gw); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: IdleRequeueInterval}, nil
}

func (r *GpuWorkloadReconciler) handleTerminal(ctx context.Context, gw *kaiwo.GpuWorkload) (ctrl.Result, error) {
	ttl := r.getTTL(gw)
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
// GpuWorkload CR. It walks the pod's controller ownerReference chain looking
// for a UID match against the workload ref.
func isPodOwnedByWorkload(pod *corev1.Pod, gw *kaiwo.GpuWorkload) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.UID == gw.Spec.WorkloadRef.UID {
			return true
		}
	}
	// The pod may not directly reference the root owner; the pod event handler
	// set a label on the GpuWorkload. Instead, check for matching namespace and
	// walk labels if needed. For now, we also check if the pod carries a label
	// pointing to this GpuWorkload.
	if pod.Labels != nil && pod.Labels["kaiwo.silogen.ai/gpu-workload"] == gw.Name {
		return true
	}
	return false
}

// --- Preemption Evaluation ---

func (r *GpuWorkloadReconciler) tryRunPreemptionEvaluation(ctx context.Context, _ *kaiwo.GpuWorkload) error {
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

	type workloadEntry struct {
		gw        *kaiwo.GpuWorkload
		gpuCount  map[string]int
		idleSince time.Time
	}

	pendingByResource := make(map[string][]workloadEntry)
	idleByResource := make(map[string][]workloadEntry)

	for i := range allWorkloads.Items {
		w := &allWorkloads.Items[i]
		policy := r.getPreemptionPolicy(w)

		switch w.Status.Phase {
		case kaiwo.GpuWorkloadPhasePending:
			for resName := range w.Spec.GpuResources {
				pendingByResource[resName] = append(pendingByResource[resName], workloadEntry{
					gw:       w,
					gpuCount: w.Spec.GpuResources,
				})
			}

		case kaiwo.GpuWorkloadPhaseIdle:
			if w.Status.IdleSince == nil {
				continue
			}

			gracePeriod := r.getGracePeriod(w)
			idleDuration := time.Since(w.Status.IdleSince.Time)
			if idleDuration < gracePeriod {
				continue
			}

			if policy == kaiwo.PreemptionPolicyAlways {
				// Always-policy: mark immediately regardless of demand
				w.Status.Phase = kaiwo.GpuWorkloadPhasePreempting
				w.Status.PreemptionReason = "PreemptionPolicy=Always: idle grace period expired"
				if err := r.Status().Update(ctx, w); err != nil {
					if errors.IsConflict(err) {
						logger.V(1).Info("conflict marking Always-policy workload, skipping", "name", w.Name)
						continue
					}
					return err
				}
				continue
			}

			for resName := range w.Spec.GpuResources {
				idleByResource[resName] = append(idleByResource[resName], workloadEntry{
					gw:        w,
					gpuCount:  w.Spec.GpuResources,
					idleSince: w.Status.IdleSince.Time,
				})
			}
		}
	}

	// Sort idle workloads by idle duration (longest idle first) for each resource
	for resName := range idleByResource {
		sort.Slice(idleByResource[resName], func(i, j int) bool {
			return idleByResource[resName][i].idleSince.Before(idleByResource[resName][j].idleSince)
		})
	}

	// Sort pending workloads by creation time (oldest first)
	for resName := range pendingByResource {
		sort.Slice(pendingByResource[resName], func(i, j int) bool {
			return pendingByResource[resName][i].gw.CreationTimestamp.Before(&pendingByResource[resName][j].gw.CreationTimestamp)
		})
	}

	// Track which workloads have been claimed as victims to prevent double-claiming
	claimed := make(map[types.UID]bool)

	for resName, pendingList := range pendingByResource {
		idlePool, ok := idleByResource[resName]
		if !ok || len(idlePool) == 0 {
			continue
		}

		for _, pending := range pendingList {
			demandCount := pending.gpuCount[resName]
			if demandCount <= 0 {
				continue
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

func (r *GpuWorkloadReconciler) getThreshold(gw *kaiwo.GpuWorkload) float64 {
	if gw.Spec.UtilizationThreshold != nil {
		return *gw.Spec.UtilizationThreshold
	}
	val, err := strconv.ParseFloat(os.Getenv(EnvDefaultThreshold), 64)
	if err != nil {
		return 5.0
	}
	return val
}

func (r *GpuWorkloadReconciler) getGracePeriod(gw *kaiwo.GpuWorkload) time.Duration {
	if gw.Spec.GracePeriod != nil {
		return gw.Spec.GracePeriod.Duration
	}
	val, err := time.ParseDuration(os.Getenv(EnvDefaultGracePeriod))
	if err != nil {
		return 10 * time.Minute
	}
	return val
}

func (r *GpuWorkloadReconciler) getPreemptionPolicy(gw *kaiwo.GpuWorkload) kaiwo.PreemptionPolicy {
	if gw.Spec.PreemptionPolicy != nil {
		return *gw.Spec.PreemptionPolicy
	}
	val := os.Getenv(EnvDefaultPolicy)
	if val == string(kaiwo.PreemptionPolicyAlways) {
		return kaiwo.PreemptionPolicyAlways
	}
	return kaiwo.PreemptionPolicyOnPressure
}

func (r *GpuWorkloadReconciler) getAggregationPolicy(gw *kaiwo.GpuWorkload) kaiwo.AggregationPolicy {
	if gw.Spec.AggregationPolicy != nil {
		return *gw.Spec.AggregationPolicy
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

func (r *GpuWorkloadReconciler) getTTL(gw *kaiwo.GpuWorkload) time.Duration {
	if gw.Spec.TTLAfterFinished != nil {
		return gw.Spec.TTLAfterFinished.Duration
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
	if annotations == nil || annotations[AnnotationEnabled] != "true" {
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

		if err := controllerutil.SetOwnerReference(rootObj, gw, r.Scheme); err != nil {
			logger.V(1).Info("could not set owner reference on GpuWorkload (non-fatal)", "error", err)
		}

		if err := r.Create(ctx, gw); err != nil {
			if !errors.IsAlreadyExists(err) {
				logger.Error(err, "failed to create GpuWorkload", "name", gwName)
			}
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: rootResult.Namespace, Name: gwName}}}
		}

		gw.Status.Phase = kaiwo.GpuWorkloadPhasePending
		gw.Status.OwnerChain = rootResult.OwnerChain
		if err := r.Status().Update(ctx, gw); err != nil {
			logger.Error(err, "failed to set initial status", "name", gwName)
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

func parseAnnotationsIntoSpec(annotations map[string]string, spec *kaiwo.GpuWorkloadSpec) {
	if v, ok := annotations[AnnotationThreshold]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			spec.UtilizationThreshold = &f
		}
	}
	if v, ok := annotations[AnnotationGracePeriod]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			dur := metav1.Duration{Duration: d}
			spec.GracePeriod = &dur
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
