package kueue

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/silogen/kaiwo/pkg/api"
	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	"k8s.io/apimachinery/pkg/api/meta"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	visibilityv1beta1 "sigs.k8s.io/kueue/apis/visibility/v1beta1"
)

// Outcome represents the result of a reclamation attempt
type Outcome int

const (
	Idle Outcome = iota
	PendingInsufficient
	Evicted
	Settling
)

const (
	// Kaiwo condition types (status.conditions on your Kaiwo CR)
	KaiwoCondAdmissionPrep = "AdmissionPrep"
	KaiwoCondExpired       = "Expired"
	KaiwoCondEvicted       = "Evicted"

	// Reasons/messages
	AdmissionPrepReasonEvicting = "EvictingExpiredWorkloads"
	EvictedReasonAdmissionPrep  = "AdmissionPrep"

	// Gate TTL: how long AdmissionPrep blocks another batch
	AdmissionPrepTTL = 2 * time.Minute
)

// ReclaimerLogic contains the core logic for workload reclamation
type ReclaimerLogic struct {
	client                  client.Client
	headOfLineKueueWorkload *kueuev1beta1.Workload
	headOfLineKaiwoWorkload api.KaiwoWorkload
}

// NewReclaimerLogic creates a new ReclaimerLogic instance
func NewReclaimerLogic(c client.Client) *ReclaimerLogic {
	return &ReclaimerLogic{client: c}
}

// Reclaim attempts to free resources for the given ClusterQueue by evicting expired workloads
func (r *ReclaimerLogic) Reclaim(ctx context.Context, clusterQueueName string) (Outcome, error) {
	logger := log.FromContext(ctx).WithName("ReclaimerLogic").WithValues("clusterQueue", clusterQueueName)

	// Head-of-Line pending WL
	hol, err := r.getHeadOfLinePendingWorkload(ctx, clusterQueueName)
	if err != nil {
		return Idle, fmt.Errorf("failed to get pending workloads: %w", err)
	}
	if hol == nil {
		return Idle, nil
	}
	r.headOfLineKueueWorkload = hol

	// Resolve HoL Kaiwo CR
	holKaiwo, err := workloadutils.GetKaiwoForWorkload(ctx, r.client, hol)
	if err != nil {
		return Idle, fmt.Errorf("failed to get Kaiwo workload: %w", err)
	}

	// Gate via Kaiwo condition
	if isKaiwoAdmissionPrepActive(holKaiwo, AdmissionPrepTTL) {
		return Settling, nil
	}

	// GPU demand for HoL
	requiredAll := r.calculateResourceRequirements(hol)
	gpuNeed := filterGpuResources(requiredAll)
	if len(gpuNeed) == 0 {
		return Idle, nil
	}

	// Candidates: admitted+active AND Kaiwo.Expired=True
	expired, err := r.getExpiredAdmittedWorkloads(ctx, clusterQueueName)
	if err != nil {
		return Idle, fmt.Errorf("failed to get expired workloads: %w", err)
	}
	if !r.canSatisfyResourceNeeds(expired, gpuNeed) {
		return PendingInsufficient, nil
	}

	// Mark HoL: Kaiwo.AdmissionPrep=True (status condition)
	if err := setKaiwoCondition(
		ctx,
		r.client,
		holKaiwo,
		KaiwoCondAdmissionPrep,
		metav1.ConditionTrue,
		AdmissionPrepReasonEvicting,
		fmt.Sprintf("Preparing admission for pending %s", hol.Name),
	); err != nil {
		logger.Info("Failed to set Kaiwo AdmissionPrep; will not evict in this pass", "error", err.Error())
		return Settling, nil
	}

	// Pick victims to cover GPU needs (single pass)
	victims := r.selectVictimsCoveringNeeds(expired, gpuNeed)

	// Evict and reflect on each victim's Kaiwo CR
	evicted := 0
	for i := range victims {
		v := victims[i]
		if err := r.evictWorkload(ctx, v, hol.Name); err != nil {
			logger.Info("Workload eviction failed; continuing best-effort", "victim", v.Name, "error", err.Error())
			continue
		}
		// Set Kaiwo.Evicted=True on victim CR (no WL annotations)
		if kaiwoVictim, err := workloadutils.GetKaiwoForWorkload(ctx, r.client, &v); err == nil {
			_ = setKaiwoCondition(ctx, r.client, kaiwoVictim, KaiwoCondEvicted, metav1.ConditionTrue,
				EvictedReasonAdmissionPrep, fmt.Sprintf("Evicted for pending %s", hol.Name))
		}
		evicted++
	}

	if evicted == 0 {
		return Settling, nil
	}

	logger.Info("Batch eviction complete", "pendingWorkload", hol.Name, "victimsEvicted", evicted)
	return Evicted, nil
}

// getHeadOfLinePendingWorkload retrieves the first pending workload using Kueue's Visibility API
func (r *ReclaimerLogic) getHeadOfLinePendingWorkload(ctx context.Context, clusterQueueName string) (*kueuev1beta1.Workload, error) {
	// Try to get pending workloads via the Visibility API subresource
	var pendingWorkloads visibilityv1beta1.PendingWorkloadsSummary
	if err := r.getPendingWorkloadsFromVisibilityAPI(ctx, clusterQueueName, &pendingWorkloads); err != nil {
		// If Visibility API is not available, fall back to listing workloads directly
		return r.getHeadOfLinePendingWorkloadFallback(ctx, clusterQueueName)
	}

	if len(pendingWorkloads.Items) == 0 {
		return nil, nil
	}

	// Pick the item with the smallest PositionInClusterQueue (HoL)
	minIdx := 0
	minPos := pendingWorkloads.Items[0].PositionInClusterQueue
	for i := 1; i < len(pendingWorkloads.Items); i++ {
		if pendingWorkloads.Items[i].PositionInClusterQueue < minPos {
			minPos = pendingWorkloads.Items[i].PositionInClusterQueue
			minIdx = i
		}
	}

	var workload kueuev1beta1.Workload
	key := types.NamespacedName{
		Name:      pendingWorkloads.Items[minIdx].Name,
		Namespace: pendingWorkloads.Items[minIdx].Namespace,
	}
	if err := r.client.Get(ctx, key, &workload); err != nil {
		return nil, fmt.Errorf("failed to get workload %s: %w", key, err)
	}
	return &workload, nil
}

// getHeadOfLinePendingWorkloadFallback is a fallback when Visibility API is not available
func (r *ReclaimerLogic) getHeadOfLinePendingWorkloadFallback(ctx context.Context, clusterQueueName string) (*kueuev1beta1.Workload, error) {
	// Build a set of LocalQueue names that map to this ClusterQueue
	lqNames, err := GetLocalQueueNamesForCluster(ctx, r.client, clusterQueueName)
	if err != nil {
		return nil, err
	}

	var workloadList kueuev1beta1.WorkloadList
	if err := r.client.List(ctx, &workloadList); err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	// Filter for pending workloads that belong to one of the LocalQueues and sort by creation time
	var pendingWorkloads []kueuev1beta1.Workload
	for _, wl := range workloadList.Items {
		lqName := string(wl.Spec.QueueName)
		if lqName == "" {
			continue
		}
		if _, ok := lqNames[types.NamespacedName{Namespace: wl.Namespace, Name: lqName}]; !ok {
			continue
		}
		if !isWorkloadAdmitted(&wl) {
			pendingWorkloads = append(pendingWorkloads, wl)
		}
	}

	if len(pendingWorkloads) == 0 {
		return nil, nil
	}

	// Sort by creation timestamp (FIFO)
	sort.Slice(pendingWorkloads, func(i, j int) bool {
		return pendingWorkloads[i].CreationTimestamp.Before(&pendingWorkloads[j].CreationTimestamp)
	})

	return &pendingWorkloads[0], nil
}

// ---------- Kaiwo-driven expiry ----------
func (r *ReclaimerLogic) getExpiredAdmittedWorkloads(ctx context.Context, clusterQueueName string) ([]kueuev1beta1.Workload, error) {
	lqNames, err := GetLocalQueueNamesForCluster(ctx, r.client, clusterQueueName)
	if err != nil {
		return nil, err
	}

	var workloadList kueuev1beta1.WorkloadList
	if err := r.client.List(ctx, &workloadList); err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	var out []kueuev1beta1.Workload
	var names []string
	skipped := 0

	for _, wl := range workloadList.Items {
		lqName := string(wl.Spec.QueueName)
		if lqName == "" || !isWorkloadAdmitted(&wl) || !isWorkloadActive(&wl) {
			skipped++
			continue
		}
		if _, ok := lqNames[types.NamespacedName{Namespace: wl.Namespace, Name: lqName}]; !ok {
			skipped++
			continue
		}
		// Join WL -> Kaiwo and require Expired=True
		kaiwoCR, err := workloadutils.GetKaiwoForWorkload(ctx, r.client, &wl)
		if err != nil || !isKaiwoExpired(kaiwoCR) {
			skipped++
			continue
		}
		out = append(out, wl)
		names = append(names, wl.Name)
	}

	if len(out) > 0 {
		log.FromContext(ctx).WithName("getExpiredAdmittedWorkloads").Info(
			"Found evictable expired workloads (via Kaiwo conditions)",
			"clusterQueue", clusterQueueName, "evictableCount", len(out), "skippedCount", skipped, "workloadNames", names,
		)
	}
	return out, nil
}

func (r *ReclaimerLogic) evictWorkload(ctx context.Context, workload kueuev1beta1.Workload, pendingWorkloadName string) error {
	var current kueuev1beta1.Workload
	key := client.ObjectKeyFromObject(&workload)
	if err := r.client.Get(ctx, key, &current); err != nil {
		return fmt.Errorf("failed to re-read workload: %w", err)
	}
	if !isWorkloadAdmitted(&current) || !isWorkloadActive(&current) {
		return fmt.Errorf("workload is no longer admitted or active")
	}

	patch := client.MergeFrom(current.DeepCopy())
	active := false
	current.Spec.Active = &active

	// No annotations/labels; just deactivate
	if err := r.client.Patch(ctx, &current, patch); err != nil {
		return fmt.Errorf("failed to evict workload: %w", err)
	}
	return nil
}

func isKaiwoExpired(k api.KaiwoWorkload) bool {
	// Kaiwo CR exposes standard metav1.Conditions in Status.Conditions
	return meta.IsStatusConditionTrue(k.GetCommonStatusSpec().Conditions, KaiwoCondExpired)
}

// ---------- HoL gate ----------

// ---------- Visibility API ----------

func (r *ReclaimerLogic) getPendingWorkloadsFromVisibilityAPI(ctx context.Context, clusterQueueName string, result *visibilityv1beta1.PendingWorkloadsSummary) error {
	// Use the SubResource method to access the visibility API
	return r.client.SubResource("pendingworkloads").Get(ctx, &kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{Name: clusterQueueName},
	}, result)
}

// ---------- Resource accounting ----------

func (r *ReclaimerLogic) calculateResourceRequirements(workload *kueuev1beta1.Workload) map[corev1.ResourceName]resource.Quantity {
	// Prefer normalized resource requests from status (matches Kueue's math)
	if len(workload.Status.ResourceRequests) > 0 {
		return r.calculateFromStatusResourceRequests(workload)
	}
	// Fallback to spec-based calculation if status is not available
	return r.calculateFromSpecPodSets(workload)
}

func (r *ReclaimerLogic) calculateFromStatusResourceRequests(workload *kueuev1beta1.Workload) map[corev1.ResourceName]resource.Quantity {
	resources := make(map[corev1.ResourceName]resource.Quantity)
	for _, rr := range workload.Status.ResourceRequests {
		for rn, q := range rr.Resources {
			if existing, exists := resources[rn]; exists {
				existing.Add(q)
				resources[rn] = existing
			} else {
				resources[rn] = q.DeepCopy()
			}
		}
	}
	return resources
}

func (r *ReclaimerLogic) calculateFromSpecPodSets(workload *kueuev1beta1.Workload) map[corev1.ResourceName]resource.Quantity {
	resources := make(map[corev1.ResourceName]resource.Quantity)
	for _, ps := range workload.Spec.PodSets {
		count := int64(ps.Count)
		for _, c := range ps.Template.Spec.Containers {
			for rn, q := range c.Resources.Requests {
				total := q.DeepCopy()
				if count > 1 {
					total.SetMilli(total.MilliValue() * count)
				}
				if existing, ok := resources[rn]; ok {
					existing.Add(total)
					resources[rn] = existing
				} else {
					resources[rn] = total
				}
			}
		}
	}
	return resources
}

// ---------- Candidate selection ----------

func (r *ReclaimerLogic) canSatisfyResourceNeeds(expiredWorkloads []kueuev1beta1.Workload, requiredResources map[corev1.ResourceName]resource.Quantity) bool {
	totalAvailable := make(map[corev1.ResourceName]resource.Quantity)
	for _, wl := range expiredWorkloads {
		workloadResources := filterGpuResources(r.calculateResourceRequirements(&wl))
		for rn, q := range workloadResources {
			if existing, exists := totalAvailable[rn]; exists {
				existing.Add(q)
				totalAvailable[rn] = existing
			} else {
				totalAvailable[rn] = q.DeepCopy()
			}
		}
	}
	for rn, need := range requiredResources {
		if have, ok := totalAvailable[rn]; !ok || have.Cmp(need) < 0 {
			return false
		}
	}
	return true
}

func (r *ReclaimerLogic) selectVictimsCoveringNeeds(
	expired []kueuev1beta1.Workload,
	required map[corev1.ResourceName]resource.Quantity,
) []kueuev1beta1.Workload {
	remaining := copyResourceMap(required)
	sort.Slice(expired, func(i, j int) bool {
		return expired[i].CreationTimestamp.Before(&expired[j].CreationTimestamp)
	})

	var victims []kueuev1beta1.Workload
	for _, wl := range expired {
		if allZeroOrLess(remaining) {
			break
		}
		contrib := filterGpuResources(r.calculateResourceRequirements(&wl))
		if contributes(contrib, remaining) {
			victims = append(victims, wl)
			decrementRemaining(remaining, contrib)
		}
	}
	return victims
}

func contributes(offer, need map[corev1.ResourceName]resource.Quantity) bool {
	for k, v := range need {
		if ov, ok := offer[k]; ok && ov.Cmp(resource.MustParse("0")) > 0 && v.Cmp(resource.MustParse("0")) > 0 {
			return true
		}
	}
	return false
}

func allZeroOrLess(m map[corev1.ResourceName]resource.Quantity) bool {
	for _, q := range m {
		if q.Sign() > 0 {
			return false
		}
	}
	return true
}

func decrementRemaining(remaining, offer map[corev1.ResourceName]resource.Quantity) {
	for k, need := range remaining {
		if off, ok := offer[k]; ok {
			rv := need.MilliValue() - off.MilliValue()
			if rv < 0 {
				rv = 0
			}
			need.SetMilli(rv)
			remaining[k] = need
		}
	}
}

// ---------- Visibility helpers ----------

func filterGpuResources(resources map[corev1.ResourceName]resource.Quantity) map[corev1.ResourceName]resource.Quantity {
	if len(resources) == 0 {
		return resources
	}
	out := make(map[corev1.ResourceName]resource.Quantity, len(resources))
	for name, qty := range resources {
		if containsGPU(string(name)) {
			out[name] = qty
		}
	}
	return out
}

func containsGPU(s string) bool { return strings.Contains(strings.ToLower(s), "gpu") }

// ---------- Generic helpers ----------

func isWorkloadAdmitted(workload *kueuev1beta1.Workload) bool {
	return meta.IsStatusConditionTrue(workload.Status.Conditions, kueuev1beta1.WorkloadAdmitted)
}

func isWorkloadActive(workload *kueuev1beta1.Workload) bool {
	return workload.Spec.Active == nil || *workload.Spec.Active
}

func copyResourceMap(original map[corev1.ResourceName]resource.Quantity) map[corev1.ResourceName]resource.Quantity {
	cp := make(map[corev1.ResourceName]resource.Quantity)
	for k, v := range original {
		cp[k] = v.DeepCopy()
	}
	return cp
}

// Gate check: Kaiwo.AdmissionPrep=True and still fresh
func isKaiwoAdmissionPrepActive(kaiwoWorkload api.KaiwoWorkload, ttl time.Duration) bool {
	cond := meta.FindStatusCondition(kaiwoWorkload.GetCommonStatusSpec().Conditions, KaiwoCondAdmissionPrep)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		return false
	}
	// Use LastTransitionTime as "last seen" marker for TTL
	if ttl <= 0 || cond.LastTransitionTime.IsZero() {
		return true
	}
	return time.Since(cond.LastTransitionTime.Time) <= ttl
}

// setKaiwoCondition sets/updates a Kaiwo status condition idempotently.
func setKaiwoCondition(ctx context.Context, c client.Client, k api.KaiwoWorkload,
	condType string, status metav1.ConditionStatus, reason, msg string,
) error {
	obj := k.(client.Object)
	orig := obj.DeepCopyObject().(client.Object)

	newCond := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: obj.GetGeneration(),
		LastTransitionTime: metav1.Now(),
	}
	meta.SetStatusCondition(&k.GetCommonStatusSpec().Conditions, newCond)
	return c.Status().Patch(ctx, obj, client.MergeFrom(orig))
}
