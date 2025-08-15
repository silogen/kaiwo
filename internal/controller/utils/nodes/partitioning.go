package nodeutils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Labels/taints we own

	KaiwoGpuPartitioningTaint = "kaiwo-gpu-partitioning"
	AmdDcmTaint               = "amd-dcm"
	AmdDcmUpValue             = "up"

	AmdDcmProfileLabel           = "dcm.amd.com/gpu-config-profile"
	AmdDcmPartitioningStateLabel = "dcm.amd.com/gpu-config-profile-state"

	// Correlate requests across retries

	AmdDcmPartitioningRequestIdLabel = "kaiwo.silogen.ai/partitioning-request-id"
	KaiwoPartitioningManagedLabel    = "kaiwo.silogen.ai/partitioning-managed"

	// AMD GPU components

	KubeAmdGpuNamespace = "kube-amd-gpu"

	// Well-known labels (prefer these over name suffixes)

	AmdDevicePluginSelectorKey   = "app.kubernetes.io/name"
	AmdDevicePluginSelectorValue = "amd-gpu-device-plugin"

	AmdNodeLabelerSelectorKey   = "app.kubernetes.io/name"
	AmdNodeLabelerSelectorValue = "amd-gpu-node-labeller"

	// Progress tracking
	retryAttemptAnnotation          = "kaiwo.silogen.ai/retry-attempt"
	partitioningStartTimeAnnotation = "kaiwo.silogen.ai/partitioning-started"
	phaseStartTimeAnnotation        = "kaiwo.silogen.ai/phase-started"
	progressTimestampLabel          = "kaiwo.silogen.ai/last-progress"

	// Timing
	baseRequeueDelay      = 5 * time.Second
	maxRequeueDelay       = 5 * time.Minute
	resourceUpdateTimeout = 30 * time.Second
	podDrainingTimeout    = 5 * time.Minute
	dcmOperationTimeout   = 2 * time.Minute
)

var (
	amdDcmTaint = corev1.Taint{
		Key:    AmdDcmTaint,
		Value:  AmdDcmUpValue,
		Effect: corev1.TaintEffectNoExecute,
	}
	kaiwoPartitioningTaint = corev1.Taint{
		Key:    KaiwoGpuPartitioningTaint,
		Effect: corev1.TaintEffectNoSchedule,
	}

	// Minimal, pragmatic allowlist signal (we still always exclude kube-system)
	systemComponentLabelKeys = []string{
		"k8s-app",
		"app.kubernetes.io/name",
		"app.kubernetes.io/component",
	}
)

type GpuPartitionTask struct {
	Client   client.Client
	Recorder record.EventRecorder
}

func (t *GpuPartitionTask) Name() string { return "GpuPartition" }

// ----- Entry -----

func (t *GpuPartitionTask) Run(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	l := log.FromContext(ctx)
	if obj.KaiwoNode == nil || obj.Node == nil {
		return nil, nil
	}
	if !obj.KaiwoNode.Spec.Partitioning.Enabled {
		return nil, nil
	}
	if obj.KaiwoNode.Status.NodeType != v1alpha1.NodeTypeGpu {
		t.setError(obj, "Cannot partition a node without GPUs")
		return nil, nil
	}

	// Refresh applied profile view (from labeler-derived status)
	if prof, err := extractCurrentProfile(ctx, obj.KaiwoNode); err == nil {
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = &prof
	} else {
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = nil
	}

	if t.isComplete(obj) {
		return t.onCompleted(ctx, obj)
	}

	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusPartitioning
	return t.dispatchPhase(ctx, obj, l)
}

// ----- Phase machine -----

func (t *GpuPartitionTask) dispatchPhase(ctx context.Context, obj *KaiwoNodeWrapper, l logr.Logger) (*ctrl.Result, error) {
	switch obj.KaiwoNode.Status.Partitioning.Phase {
	case "", v1alpha1.PartitioningPhaseCompleted:
		return t.phaseInit(ctx, obj)
	case v1alpha1.PartitioningPhaseDrainingUntoleratedPods:
		return t.phaseDrain(ctx, obj)
	case v1alpha1.PartitioningPhaseApplyingPartitions:
		return t.phaseApply(ctx, obj)
	case v1alpha1.PartitioningPhaseWaitingForResourceUpdate:
		return t.phaseWaitResources(ctx, obj)
	case v1alpha1.PartitioningPhaseRestartingPods:
		return t.phaseRestartPods(ctx, obj)
	case v1alpha1.PartitioningPhaseWaitingForLabels:
		return t.phaseWaitLabels(ctx, obj)
	case v1alpha1.PartitioningPhaseError:
		// allow auto-recovery if conditions changed (e.g., admin fixed DCM)
		if t.isComplete(obj) {
			return t.onCompleted(ctx, obj)
		}
		if t.shouldRetryFromError(obj) {
			obj.KaiwoNode.Status.Partitioning.Phase = ""
			return t.phaseInit(ctx, obj)
		}
		return &ctrl.Result{RequeueAfter: t.backoff(obj, 3)}, nil
	default:
		t.setError(obj, fmt.Sprintf("Unknown partitioning phase: %s", obj.KaiwoNode.Status.Partitioning.Phase))
		return &ctrl.Result{RequeueAfter: t.backoff(obj, 3)}, nil
	}
}

// ----- Phases -----

func (t *GpuPartitionTask) phaseInit(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	// Add taints: NoSchedule first (stop new), then NoExecute (evict existing)
	if err := t.ensureTaint(ctx, obj.Node, kaiwoPartitioningTaint, true); err != nil {
		return nil, fmt.Errorf("add partitioning taint: %w", err)
	}
	if err := t.ensureTaint(ctx, obj.Node, amdDcmTaint, true); err != nil {
		return nil, fmt.Errorf("add DCM taint: %w", err)
	}
	t.inProgress(obj, v1alpha1.PartitioningPhaseDrainingUntoleratedPods, "Draining untolerated pods")
	return &ctrl.Result{RequeueAfter: t.backoff(obj, 0)}, nil
}

func (t *GpuPartitionTask) phaseDrain(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	l := log.FromContext(ctx)
	offending, err := t.listOffendingPods(ctx, obj.Node)
	if err != nil {
		return nil, fmt.Errorf("list offending pods: %w", err)
	}
	if len(offending) > 0 {
		// Try active eviction (best-effort). This complements the NoExecute taint.
		_ = t.evictPods(ctx, offending)

		elapsed := t.phaseElapsed(obj)
		if elapsed >= podDrainingTimeout {
			t.setError(obj, fmt.Sprintf("Draining timeout; stuck pods: %v", names(offending)))
			_ = t.cleanupTaints(ctx, obj.Node) // don't strand the node
			return &ctrl.Result{RequeueAfter: t.backoff(obj, 3)}, nil
		}
		t.inProgress(obj, v1alpha1.PartitioningPhaseDrainingUntoleratedPods,
			fmt.Sprintf("Waiting on %d pod(s), elapsed %s", len(offending), elapsed.Round(time.Second)))
		l.Info("Drain waiting", "count", len(offending), "pods", names(offending))
		return &ctrl.Result{RequeueAfter: t.backoff(obj, 1)}, nil
	}

	// Request partitioning via labels; clear any old state label to avoid mixing attempts
	if err := t.patchNodeLabels(ctx, obj.Node, map[string]string{
		AmdDcmProfileLabel:               string(obj.KaiwoNode.Spec.Partitioning.Profile),
		AmdDcmPartitioningRequestIdLabel: fmt.Sprintf("%d", time.Now().UnixNano()),
		KaiwoPartitioningManagedLabel:    "true",
	}, []string{AmdDcmPartitioningStateLabel}); err != nil {
		return nil, fmt.Errorf("apply DCM labels: %w", err)
	}

	t.inProgress(obj, v1alpha1.PartitioningPhaseApplyingPartitions, "Applying partitioning via DCM")
	return &ctrl.Result{RequeueAfter: t.backoff(obj, 0)}, nil
}

func (t *GpuPartitionTask) phaseApply(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	state, ok := obj.Node.Labels[AmdDcmPartitioningStateLabel]
	reqID := obj.Node.Labels[AmdDcmPartitioningRequestIdLabel]

	switch {
	case ok && state == "failure":
		t.setError(obj, fmt.Sprintf("DCM reported failure (request %s). Check DCM logs.", reqID))
		_ = t.cleanupTaints(ctx, obj.Node)
		return &ctrl.Result{RequeueAfter: t.backoff(obj, 3)}, nil

	case ok && state == "success":
		// Remove the eviction taint; keep NoSchedule until we finish the flow
		_ = t.ensureTaint(ctx, obj.Node, amdDcmTaint, false)
		t.inProgress(obj, v1alpha1.PartitioningPhaseWaitingForResourceUpdate, "Waiting for resources/labels to settle")
		return &ctrl.Result{RequeueAfter: t.backoff(obj, 1)}, nil

	default:
		if t.phaseElapsed(obj) >= dcmOperationTimeout {
			t.setError(obj, fmt.Sprintf("DCM timeout after %s (request %s)", dcmOperationTimeout, reqID))
			_ = t.cleanupTaints(ctx, obj.Node)
			return &ctrl.Result{RequeueAfter: t.backoff(obj, 3)}, nil
		}
		return &ctrl.Result{RequeueAfter: t.backoff(obj, 1)}, nil
	}
}

func (t *GpuPartitionTask) phaseWaitResources(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	if prof, err := extractCurrentProfile(ctx, obj.KaiwoNode); err == nil {
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = &prof
		if prof == obj.KaiwoNode.Spec.Partitioning.Profile {
			t.inProgress(obj, v1alpha1.PartitioningPhaseWaitingForLabels, "Resources updated; waiting final labels")
			return &ctrl.Result{RequeueAfter: t.backoff(obj, 0)}, nil
		}
	}

	if t.phaseElapsed(obj) >= resourceUpdateTimeout {
		t.inProgress(obj, v1alpha1.PartitioningPhaseRestartingPods, "Resources stale; restarting AMD DaemonSets")
		return &ctrl.Result{Requeue: true}, nil
	}
	return &ctrl.Result{RequeueAfter: t.backoff(obj, 0)}, nil
}

func (t *GpuPartitionTask) phaseRestartPods(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	// Restart device-plugin & labeler by label; suffix fallback remains
	_ = t.rolloutRestartDaemonSets(ctx, map[string]string{AmdDevicePluginSelectorKey: AmdDevicePluginSelectorValue})
	_ = t.rolloutRestartDaemonSets(ctx, map[string]string{AmdNodeLabelerSelectorKey: AmdNodeLabelerSelectorValue})
	t.inProgress(obj, v1alpha1.PartitioningPhaseWaitingForLabels, "DaemonSets restarted; waiting for labels")
	return &ctrl.Result{RequeueAfter: t.backoff(obj, 1)}, nil
}

func (t *GpuPartitionTask) phaseWaitLabels(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	// We complete as soon as isComplete() turns true on the next Reconcile
	return &ctrl.Result{RequeueAfter: t.backoff(obj, 0)}, nil
}

// ----- Completion -----

func (t *GpuPartitionTask) isComplete(obj *KaiwoNodeWrapper) bool {
	state, ok := obj.Node.Labels[AmdDcmPartitioningStateLabel]
	hasDesired := obj.KaiwoNode.Status.Partitioning.AppliedProfile != nil &&
		*obj.KaiwoNode.Status.Partitioning.AppliedProfile == obj.KaiwoNode.Spec.Partitioning.Profile

	// Treat DCM success as authoritative completion; profile match confirms steady state
	if ok && state == "success" && hasDesired {
		return true
	}
	// Handle pre-partitioned nodes in Completed phase (e.g., after operator restart)
	return !ok && hasDesired && obj.KaiwoNode.Status.Partitioning.Phase == v1alpha1.PartitioningPhaseCompleted
}

func (t *GpuPartitionTask) onCompleted(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	l := log.FromContext(ctx)

	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusReady
	obj.KaiwoNode.Status.Partitioning.Phase = v1alpha1.PartitioningPhaseCompleted
	t.resetAttempts(obj)
	t.resetProgress(obj)

	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  string(v1alpha1.PartitioningConditionComplete),
		Message: "Partitioning matches requested profile",
	})

	// Remove our NoSchedule taint now that we are done
	if err := t.ensureTaint(ctx, obj.Node, kaiwoPartitioningTaint, false); err != nil {
		l.Info("Failed to remove partitioning taint", "err", err)
	}
	return nil, nil
}

// ----- Node patch helpers -----

func (t *GpuPartitionTask) ensureTaint(ctx context.Context, node *corev1.Node, taint corev1.Taint, present bool) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var fresh corev1.Node
		if err := t.Client.Get(ctx, client.ObjectKeyFromObject(node), &fresh); err != nil {
			return err
		}
		orig := fresh.DeepCopy()

		has := false
		for _, nt := range fresh.Spec.Taints {
			if nt.MatchTaint(&taint) {
				has = true
				break
			}
		}
		switch {
		case present && !has:
			fresh.Spec.Taints = append(fresh.Spec.Taints, taint)
		case !present && has:
			newTaints := make([]corev1.Taint, 0, len(fresh.Spec.Taints))
			for _, nt := range fresh.Spec.Taints {
				if !nt.MatchTaint(&taint) {
					newTaints = append(newTaints, nt)
				}
			}
			fresh.Spec.Taints = newTaints
		default:
			return nil
		}
		return t.Client.Patch(ctx, &fresh, client.MergeFrom(orig))
	})
}

func (t *GpuPartitionTask) patchNodeLabels(ctx context.Context, node *corev1.Node, add map[string]string, remove []string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var fresh corev1.Node
		if err := t.Client.Get(ctx, client.ObjectKeyFromObject(node), &fresh); err != nil {
			return err
		}
		orig := fresh.DeepCopy()
		labels := fresh.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		for k, v := range add {
			labels[k] = v
		}
		for _, k := range remove {
			delete(labels, k)
		}
		fresh.SetLabels(labels)
		return t.Client.Patch(ctx, &fresh, client.MergeFrom(orig))
	})
}

// ----- Pods: list / evict -----

func (t *GpuPartitionTask) listOffendingPods(ctx context.Context, node *corev1.Node) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := t.Client.List(ctx, &pods, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return nil, err
	}
	out := make([]corev1.Pod, 0, len(pods.Items))
	for _, p := range pods.Items {
		if p.Namespace == "kube-system" || p.DeletionTimestamp != nil {
			continue
		}
		if toleratesDCM(p.Spec.Tolerations) {
			continue
		}
		if isSystemish(&p) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (t *GpuPartitionTask) evictPods(ctx context.Context, pods []corev1.Pod) error {
	grace := int64(30)
	for i := range pods {
		p := &pods[i]
		ev := &policyv1.Eviction{
			ObjectMeta:    metav1.ObjectMeta{Namespace: p.Namespace, Name: p.Name},
			DeleteOptions: &metav1.DeleteOptions{GracePeriodSeconds: &grace},
		}
		// Ignore RBAC/feature errors; taint-based eviction still applies
		_ = t.Client.SubResource("eviction").Create(ctx, p, ev)
	}
	return nil
}

// ----- Conditions / progress / backoff -----

func (t *GpuPartitionTask) setError(obj *KaiwoNodeWrapper, message string) {
	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusError
	obj.KaiwoNode.Status.Partitioning.Phase = v1alpha1.PartitioningPhaseError
	t.touchProgress(obj, v1alpha1.PartitioningPhaseError)

	attempt := t.attempt(obj)
	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  string(v1alpha1.PartitioningConditionFailed),
		Message: fmt.Sprintf("%s (attempt %d)", message, attempt+1),
	})
}

func (t *GpuPartitionTask) inProgress(obj *KaiwoNodeWrapper, phase v1alpha1.PartitioningPhase, msg string) {
	obj.KaiwoNode.Status.Partitioning.Phase = phase
	t.touchProgress(obj, phase)
	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  string(v1alpha1.PartitioningConditionInProgress),
		Message: msg,
	})
}

func (t *GpuPartitionTask) touchProgress(obj *KaiwoNodeWrapper, phase v1alpha1.PartitioningPhase) {
	now := time.Now().Format(time.RFC3339)
	if obj.KaiwoNode.Annotations == nil {
		obj.KaiwoNode.Annotations = map[string]string{}
	}
	if obj.Node.Labels == nil {
		obj.Node.Labels = map[string]string{}
	}
	if _, ok := obj.KaiwoNode.Annotations[partitioningStartTimeAnnotation]; !ok {
		obj.KaiwoNode.Annotations[partitioningStartTimeAnnotation] = now
	}
	obj.KaiwoNode.Annotations[phaseStartTimeAnnotation] = now
	obj.Node.Labels[progressTimestampLabel] = fmt.Sprintf("%d", time.Now().Unix())
}

func (t *GpuPartitionTask) resetProgress(obj *KaiwoNodeWrapper) {
	if obj.KaiwoNode.Annotations != nil {
		delete(obj.KaiwoNode.Annotations, partitioningStartTimeAnnotation)
		delete(obj.KaiwoNode.Annotations, phaseStartTimeAnnotation)
	}
	if obj.Node.Labels != nil {
		delete(obj.Node.Labels, progressTimestampLabel)
	}
}

func (t *GpuPartitionTask) phaseElapsed(obj *KaiwoNodeWrapper) time.Duration {
	if obj.KaiwoNode.Annotations == nil {
		return 0
	}
	s := obj.KaiwoNode.Annotations[phaseStartTimeAnnotation]
	if s == "" {
		return 0
	}
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return time.Since(tm)
}

func (t *GpuPartitionTask) backoff(obj *KaiwoNodeWrapper, severity int) time.Duration {
	// exponential by attempts, scaled by severity (0..4)
	n := t.attempt(obj)
	f := 1 << min(n, 6) // cap at 64x
	base := baseRequeueDelay * time.Duration(max(1, 1<<severity))
	d := base * time.Duration(f)
	if d > maxRequeueDelay {
		d = maxRequeueDelay
	}
	// bump attempt for next time
	t.bumpAttempt(obj)
	return withJitter(d, 0.25)
}

func withJitter(d time.Duration, frac float64) time.Duration {
	j := time.Duration(frac * float64(d))
	if j == 0 {
		return d
	}
	n := time.Now().UnixNano()
	if n%2 == 0 {
		return d + time.Duration(n%int64(j))
	}
	return d - time.Duration(n%int64(j))
}

func (t *GpuPartitionTask) attempt(obj *KaiwoNodeWrapper) int {
	if obj.KaiwoNode.Annotations == nil {
		return 0
	}
	var n int
	fmt.Sscanf(obj.KaiwoNode.Annotations[retryAttemptAnnotation], "%d", &n)
	return n
}

func (t *GpuPartitionTask) bumpAttempt(obj *KaiwoNodeWrapper) {
	if obj.KaiwoNode.Annotations == nil {
		obj.KaiwoNode.Annotations = map[string]string{}
	}
	obj.KaiwoNode.Annotations[retryAttemptAnnotation] = fmt.Sprintf("%d", t.attempt(obj)+1)
}

func (t *GpuPartitionTask) resetAttempts(obj *KaiwoNodeWrapper) {
	if obj.KaiwoNode.Annotations != nil {
		delete(obj.KaiwoNode.Annotations, retryAttemptAnnotation)
	}
}

// ----- Misc helpers -----

func toleratesDCM(tols []corev1.Toleration) bool {
	for _, tol := range tols {
		if tol.Key != AmdDcmTaint || tol.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		switch tol.Operator {
		case corev1.TolerationOpExists:
			return true
		case corev1.TolerationOpEqual:
			if tol.Value == AmdDcmUpValue {
				return true
			}
		}
	}
	return false
}

func isSystemish(p *corev1.Pod) bool {
	if p.Spec.PriorityClassName == "system-cluster-critical" || p.Spec.PriorityClassName == "system-node-critical" {
		return true
	}
	for _, key := range systemComponentLabelKeys {
		if v, ok := p.Labels[key]; ok {
			// lightweight heuristic; you can refine this list
			if strings.Contains(v, "coredns") || strings.Contains(v, "kube-proxy") ||
				strings.Contains(v, "calico") || strings.Contains(v, "cilium") ||
				strings.Contains(v, "metrics-server") || strings.Contains(v, "node-exporter") ||
				strings.Contains(v, "csi") || strings.Contains(v, "device-plugin") {
				return true
			}
		}
	}
	return false
}

func names(pods []corev1.Pod) []string {
	out := make([]string, len(pods))
	for i := range pods {
		out[i] = pods[i].Namespace + "/" + pods[i].Name
	}
	return out
}

// Restart DaemonSets by label selector
func (t *GpuPartitionTask) rolloutRestartDaemonSets(ctx context.Context, sel map[string]string) error {
	var dsList appsv1.DaemonSetList
	if err := t.Client.List(ctx, &dsList, client.InNamespace(KubeAmdGpuNamespace), client.MatchingLabels(sel)); err != nil {
		return err
	}
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		orig := ds.DeepCopy()
		if ds.Spec.Template.Annotations == nil {
			ds.Spec.Template.Annotations = map[string]string{}
		}
		ds.Spec.Template.Annotations["kaiwo.silogen.ai/restartedAt"] = time.Now().Format(time.RFC3339)
		if err := t.Client.Patch(ctx, ds, client.MergeFrom(orig)); err != nil {
			return fmt.Errorf("restart DS %s/%s: %w", ds.Namespace, ds.Name, err)
		}
		t.Recorder.Eventf(ds, corev1.EventTypeNormal, "RolloutRestart", "Restarted %s/%s", ds.Namespace, ds.Name)
	}
	return nil
}

// Remove both taints on error to avoid stranding the node
func (t *GpuPartitionTask) cleanupTaints(ctx context.Context, node *corev1.Node) error {
	_ = t.ensureTaint(ctx, node, amdDcmTaint, false)
	_ = t.ensureTaint(ctx, node, kaiwoPartitioningTaint, false)
	return nil
}

// Derive current profile from KaiwoNode status (still your original logic)
func extractCurrentProfile(ctx context.Context, kn *v1alpha1.KaiwoNode) (v1alpha1.GpuPartitioningProfile, error) {
	g := kn.Status.Resources.Gpus
	if g.LogicalVramPerGpu == nil {
		return "", fmt.Errorf("no vRAM information; labels likely not ready")
	}
	if g.PhysicalCount != nil && *g.PhysicalCount != g.LogicalCount {
		return v1alpha1.GpuPartitioningProfileAmdCpx, nil
	}
	return v1alpha1.GpuPartitioningProfileAmdSpx, nil
}

// Manual retry hook from Error phase
func (t *GpuPartitionTask) shouldRetryFromError(obj *KaiwoNodeWrapper) bool {
	attempt := t.attempt(obj)
	if attempt >= 5 {
		return false
	}
	cond := meta.FindStatusCondition(obj.KaiwoNode.Status.Conditions, v1alpha1.PartitioningCompletedConditionType)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		return false
	}
	elapsed := time.Since(cond.LastTransitionTime.Time)
	want := []time.Duration{2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 30 * time.Minute}
	idx := min(attempt, len(want)-1)
	return elapsed >= want[idx]
}

// tiny utils
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
