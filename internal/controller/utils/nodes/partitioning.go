// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nodeutils

import (
	"context"
	"fmt"
	"strings"
	"time"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	KindDaemonSet = "DaemonSet"
)

// GpuPartitionTask is responsible for ensuring that the nodes are partitioned correctly
type GpuPartitionTask struct {
	Client   client.Client
	Recorder record.EventRecorder
}

func NewGpuPartitionTask(client client.Client, recorder record.EventRecorder) *GpuPartitionTask {
	return &GpuPartitionTask{
		Client:   client,
		Recorder: recorder,
	}
}

type PartitioningProfile string

const (
	PartitioningProfileSPX PartitioningProfile = "spx"
	PartitioningProfileCPX PartitioningProfile = "cpx"

	KaiwoGpuPartitioningTaint = "kaiwo-gpu-partitioning"

	AmdDcmTaint   = "amd-dcm"
	AmdDcmUpValue = "up"

	AmdDcmProfileLabel           = "dcm.amd.com/gpu-config-profile"
	AmdDcmPartitioningStateLabel = "dcm.amd.com/gpu-config-profile-state"
	// DCM request ID label to avoid race conditions
	AmdDcmPartitioningRequestIdLabel = "kaiwo.silogen.ai/partitioning-request-id"
	// Controller managed field to identify our ownership
	KaiwoPartitioningManagedLabel = "kaiwo.silogen.ai/partitioning-managed"

	devicePluginDaemonsetSuffix = "device-plugin"
	nodeLabelerDaemonsetSuffix  = "node-labeller"

	KubeAmdGpuNamespace                 = "kube-amd-gpu"
	AmdDeviceConfigManagerConfigMapName = "config-manager-config"

	// Timing constants
	shortRequeueDelay  = 5 * time.Second
	mediumRequeueDelay = 10 * time.Second
	longRequeueDelay   = 30 * time.Second

	resourceUpdateTimeout = 30 * time.Second
	// Pod draining timeout - how long to wait for pods to be evicted
	podDrainingTimeout = 5 * time.Minute
)

var (
	// System component allowlist - pods with these labels are considered system components
	systemComponentLabels = []string{
		"k8s-app",
		"app.kubernetes.io/name",
		"app.kubernetes.io/component",
	}

	amdDcmTaint = corev1.Taint{
		Key:    AmdDcmTaint,
		Value:  AmdDcmUpValue,
		Effect: corev1.TaintEffectNoExecute,
	}
	kaiwoPartitioningTaint = corev1.Taint{
		Key:    KaiwoGpuPartitioningTaint,
		Effect: corev1.TaintEffectNoSchedule,
	}
)

func (t *GpuPartitionTask) Name() string { return "GpuPartition" }

// extractCurrentProfile extracts the currently applied profile
func extractCurrentProfile(kaiwoNode *v1alpha1.KaiwoNode) (v1alpha1.GpuPartitioningProfile, error) {
	logger := log.FromContext(context.Background())

	baseutils.Debug(logger, "extractCurrentProfile: Starting profile extraction", "node", kaiwoNode.Name)

	gpus := kaiwoNode.Status.Resources.Gpus

	logger.Info("extractCurrentProfile: GPU resource information",
		"node", kaiwoNode.Name,
		"logicalCount", gpus.LogicalCount,
		"physicalCount", gpus.PhysicalCount,
		"logicalVramPerGpu", gpus.LogicalVramPerGpu)

	if gpus.LogicalVramPerGpu == nil {
		logger.Info("extractCurrentProfile: No vRAM information available",
			"node", kaiwoNode.Name)
		return "", fmt.Errorf("no vRAM information, labels are likely not set correctly")
	}

	if gpus.PhysicalCount != nil && *gpus.PhysicalCount != gpus.LogicalCount {
		logger.Info("extractCurrentProfile: Detected CPX profile (physical != logical)",
			"node", kaiwoNode.Name,
			"physicalCount", *gpus.PhysicalCount,
			"logicalCount", gpus.LogicalCount)
		return v1alpha1.GpuPartitioningProfileAmdCpx, nil
	}

	logger.Info("extractCurrentProfile: Detected SPX profile (physical == logical or no physical count)",
		"node", kaiwoNode.Name,
		"physicalCount", gpus.PhysicalCount,
		"logicalCount", gpus.LogicalCount)
	return v1alpha1.GpuPartitioningProfileAmdSpx, nil
}

func (t *GpuPartitionTask) Run(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	baseutils.Debug(logger, "GpuPartitionTask: Starting partitioning task", "node", obj.Node.Name)

	// Return if partitioning is disabled
	if !obj.KaiwoNode.Spec.Partitioning.Enabled {
		baseutils.Debug(logger, "GpuPartitionTask: Partitioning is disabled, skipping", "node", obj.Node.Name)
		return nil, nil
	}

	logger.Info("GpuPartitionTask: Processing node for partitioning",
		"node", obj.Node.Name,
		"nodeType", obj.KaiwoNode.Status.NodeType,
		"desiredProfile", obj.KaiwoNode.Spec.Partitioning.Profile)

	// If the node is not a GPU node, return with error
	if obj.KaiwoNode.Status.NodeType != v1alpha1.NodeTypeGpu {
		logger.Info("GpuPartitionTask: Node is not a GPU node, cannot partition",
			"node", obj.Node.Name,
			"nodeType", obj.KaiwoNode.Status.NodeType)
		t.setErrorCondition(obj, "Cannot partition a node without GPUs")
		return nil, nil
	}

	currentProfile, err := extractCurrentProfile(obj.KaiwoNode)
	if err != nil {
		logger.Info("GpuPartitionTask: Failed to extract current profile",
			"node", obj.Node.Name,
			"error", err.Error())
		baseutils.Debug(logger, "GpuPartitionTask: Setting AppliedProfile to nil due to extraction error", "node", obj.Node.Name)
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = nil
	} else {
		logger.Info("GpuPartitionTask: Successfully extracted current profile",
			"node", obj.Node.Name,
			"currentProfile", currentProfile)
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = &currentProfile
	}

	// Check if partitioning is complete and matches desired state
	logger.Info("GpuPartitionTask: Checking if partitioning is complete", "node", obj.Node.Name)
	if t.isPartitioningComplete(obj) {
		logger.Info("GpuPartitionTask: Partitioning is complete, handling completion", "node", obj.Node.Name)
		return t.handleCompletedPartitioning(ctx, obj)
	}

	// Handle partitioning phases
	logger.Info("GpuPartitionTask: Partitioning not complete, handling partitioning phases",
		"node", obj.Node.Name,
		"currentPhase", obj.KaiwoNode.Status.Partitioning.Phase)
	return t.handlePartitioningPhase(ctx, obj)
}

// isPartitioningComplete checks if the current state matches the desired state
func (t *GpuPartitionTask) isPartitioningComplete(obj *KaiwoNodeWrapper) bool {
	profileState, profileStateExists := obj.Node.Labels[AmdDcmPartitioningStateLabel]
	requestID := obj.Node.Labels[AmdDcmPartitioningRequestIdLabel]

	appliedProfile := "<nil>"
	if obj.KaiwoNode.Status.Partitioning.AppliedProfile != nil {
		appliedProfile = string(*obj.KaiwoNode.Status.Partitioning.AppliedProfile)
	}
	desiredProfile := string(obj.KaiwoNode.Spec.Partitioning.Profile)
	phase := string(obj.KaiwoNode.Status.Partitioning.Phase)

	log.FromContext(context.Background()).Info("Checking if partitioning is complete",
		"node", obj.Node.Name,
		"appliedProfile", appliedProfile,
		"desiredProfile", desiredProfile,
		"phase", phase,
		"dcmStateLabel", profileState,
		"dcmStateLabelExists", profileStateExists,
		"requestID", requestID)

	// Check if we have the applied profile and it matches the desired profile
	hasCorrectProfile := obj.KaiwoNode.Status.Partitioning.AppliedProfile != nil &&
		*obj.KaiwoNode.Status.Partitioning.AppliedProfile == obj.KaiwoNode.Spec.Partitioning.Profile

	log.FromContext(context.Background()).Info("Profile matching check",
		"node", obj.Node.Name,
		"hasCorrectProfile", hasCorrectProfile)

	// If we have the DCM state label and it indicates success, we're definitely complete
	if profileStateExists && profileState == "success" && hasCorrectProfile {
		log.FromContext(context.Background()).Info("Partitioning complete via DCM state label",
			"node", obj.Node.Name, "requestID", requestID)
		return true
	}

	// If we don't have the state label but can detect the correct profile is applied,
	// consider it complete (handles pre-partitioned nodes)
	if hasCorrectProfile && !profileStateExists {
		// Additional validation: only consider it complete if we're in a clearly completed state
		// Do NOT consider empty phase as complete, as that's the initial state for new partitioning
		currentPhase := obj.KaiwoNode.Status.Partitioning.Phase
		log.FromContext(context.Background()).Info("Checking pre-partitioned node completion",
			"node", obj.Node.Name,
			"currentPhase", currentPhase)

		if currentPhase == v1alpha1.PartitioningPhaseCompleted {
			log.FromContext(context.Background()).Info("Partitioning complete via pre-partitioned node detection",
				"node", obj.Node.Name)
			return true
		} else {
			log.FromContext(context.Background()).Info("Pre-partitioned node but not in completed phase, allowing partitioning to proceed",
				"node", obj.Node.Name,
				"phase", currentPhase)
		}
	}

	log.FromContext(context.Background()).Info("Partitioning not complete",
		"node", obj.Node.Name)
	return false
}

// handleCompletedPartitioning handles the case where partitioning is already complete
func (t *GpuPartitionTask) handleCompletedPartitioning(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("GpuPartitionTask: Handling completed partitioning",
		"node", obj.Node.Name,
		"previousStatus", obj.KaiwoNode.Status.Status,
		"previousPhase", obj.KaiwoNode.Status.Partitioning.Phase)

	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusReady
	obj.KaiwoNode.Status.Partitioning.Phase = v1alpha1.PartitioningPhaseCompleted

	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Reason:  string(v1alpha1.PartitioningConditionComplete),
		Status:  metav1.ConditionTrue,
		Message: "Partitioning matches requested profile",
	})

	// Remove partitioning taint if it exists
	if t.nodeHasTaint(obj.Node, kaiwoPartitioningTaint) {
		logger.Info("GpuPartitionTask: Removing partitioning taint from node", "node", obj.Node.Name)
		if err := t.removeTaint(ctx, obj.Node, kaiwoPartitioningTaint); err != nil {
			return nil, fmt.Errorf("failed to remove partitioning taint: %w", err)
		}
	} else {
		baseutils.Debug(logger, "GpuPartitionTask: No partitioning taint found on node", "node", obj.Node.Name)
	}

	logger.Info("GpuPartitionTask: Successfully completed partitioning handling",
		"node", obj.Node.Name,
		"newStatus", obj.KaiwoNode.Status.Status,
		"newPhase", obj.KaiwoNode.Status.Partitioning.Phase)

	return nil, nil
}

// handlePartitioningPhase handles the main partitioning state machine
func (t *GpuPartitionTask) handlePartitioningPhase(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusPartitioning

	switch obj.KaiwoNode.Status.Partitioning.Phase {
	case "", v1alpha1.PartitioningPhaseCompleted:
		return t.handleInitializePartitioning(ctx, obj)

	case v1alpha1.PartitioningPhaseDrainingUntoleratedPods:
		return t.handleDrainingPods(ctx, obj)

	case v1alpha1.PartitioningPhaseApplyingPartitions:
		return t.handleApplyingPartitions(ctx, obj)

	case v1alpha1.PartitioningPhaseWaitingForResourceUpdate:
		return t.handleWaitingForResourceUpdate(ctx, obj)

	case v1alpha1.PartitioningPhaseRestartingPods:
		return t.handleRestartingPods(ctx, obj)

	case v1alpha1.PartitioningPhaseWaitingForLabels:
		return t.handleWaitingForLabels(ctx, obj)

	case v1alpha1.PartitioningPhaseError:
		logger.Info("GpuPartitionTask: Node is in error phase, checking if recovery is possible",
			"node", obj.Node.Name)
		baseutils.Debug(logger, "GpuPartitionTask: Attempting error recovery for partitioning", "node", obj.Node.Name)

		// Check if partitioning is actually complete despite being in error state
		// This can happen when a pre-partitioned node was incorrectly marked as error
		if t.isPartitioningComplete(obj) {
			logger.Info("GpuPartitionTask: Node was in error state but partitioning is actually complete, recovering",
				"node", obj.Node.Name)
			return t.handleCompletedPartitioning(ctx, obj)
		}

		// Check if we should retry after some time (look for retry annotation or condition age)
		if t.shouldRetryFromError(obj) {
			logger.Info("GpuPartitionTask: Attempting retry from error state", "node", obj.Node.Name)
			return t.handleInitializePartitioning(ctx, obj)
		}

		logger.Info("GpuPartitionTask: Partitioning is in error state, will retry after delay",
			"node", obj.Node.Name)
		// Requeue with backoff to allow for manual intervention or transient issue resolution
		return &ctrl.Result{RequeueAfter: longRequeueDelay}, nil

	default:
		logger.Error(nil, "Unknown partitioning phase", "phase", obj.KaiwoNode.Status.Partitioning.Phase)
		t.setErrorCondition(obj, fmt.Sprintf("Unknown partitioning phase: %s", obj.KaiwoNode.Status.Partitioning.Phase))
		return nil, nil
	}
}

// handleInitializePartitioning sets up taints for partitioning
func (t *GpuPartitionTask) handleInitializePartitioning(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Verify our own DaemonSets have the required tolerations (documentation check only)
	if err := t.verifyOwnDaemonSetTolerations(ctx); err != nil {
		logger.Info("Warning: Our DaemonSets may not have required tolerations", "error", err.Error())
		// Don't fail the operation, just log the warning
	}

	// Add the partitioning taint first to prevent new scheduling
	if !t.nodeHasTaint(obj.Node, kaiwoPartitioningTaint) {
		if err := t.addTaint(ctx, obj.Node, kaiwoPartitioningTaint); err != nil {
			return nil, fmt.Errorf("failed to add partitioning taint: %w", err)
		}
	}

	// Add the DCM taint to trigger pod eviction
	if !t.nodeHasTaint(obj.Node, amdDcmTaint) {
		if err := t.addTaint(ctx, obj.Node, amdDcmTaint); err != nil {
			return nil, fmt.Errorf("failed to add DCM taint: %w", err)
		}
	}

	t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseDrainingUntoleratedPods, "Draining untolerated pods")

	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

// handleDrainingPods waits for untolerated pods to be drained with timeout
func (t *GpuPartitionTask) handleDrainingPods(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	offendingPods, err := t.getOffendingPods(ctx, obj.Node)
	if err != nil {
		return nil, fmt.Errorf("failed to check offending pods: %w", err)
	}

	if len(offendingPods) > 0 {
		// Check if we've been draining for too long
		condition := meta.FindStatusCondition(obj.KaiwoNode.Status.Conditions, v1alpha1.PartitioningCompletedConditionType)
		if condition != nil {
			elapsed := time.Since(condition.LastTransitionTime.Time)
			if elapsed >= podDrainingTimeout {
				// Log which pods are still stuck
				podNames := make([]string, len(offendingPods))
				for i, pod := range offendingPods {
					podNames[i] = fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
				}
				message := fmt.Sprintf("Pod draining timeout exceeded. Stuck pods: %v. Check if pods have PDBs or are stuck in terminating state.", podNames)
				t.setErrorCondition(obj, message)
				return &ctrl.Result{RequeueAfter: longRequeueDelay}, nil
			}

			// Update status with progress information
			message := fmt.Sprintf("Waiting for %d pod(s) to be evicted (elapsed: %v)", len(offendingPods), elapsed)
			t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseDrainingUntoleratedPods, message)
		}

		logger.Info("Still waiting for pods to be evicted",
			"node", obj.Node.Name,
			"podCount", len(offendingPods),
			"pods", getPodNamesForLogging(offendingPods))
		return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
	}

	// No more offending pods, proceed to apply partitions
	logger.Info("All non-tolerant pods have been evicted, proceeding with partitioning", "node", obj.Node.Name)
	if err := t.applyPartitioningLabels(ctx, obj); err != nil {
		return nil, fmt.Errorf("failed to apply partitioning labels: %w", err)
	}
	t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseApplyingPartitions, "Applying partitioning")

	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

func (t *GpuPartitionTask) applyPartitioningLabels(ctx context.Context, obj *KaiwoNodeWrapper) error {
	// Generate a unique request ID to avoid race conditions with DCM
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Apply labels with safe patching
	return t.patchNodeLabels(ctx, obj.Node, map[string]string{
		AmdDcmProfileLabel:               string(obj.KaiwoNode.Spec.Partitioning.Profile),
		AmdDcmPartitioningRequestIdLabel: requestID,
		KaiwoPartitioningManagedLabel:    "true",
	}, []string{AmdDcmPartitioningStateLabel})
}

// handleApplyingPartitions monitors the partitioning process
func (t *GpuPartitionTask) handleApplyingPartitions(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)
	profileState, profileStateExists := obj.Node.Labels[AmdDcmPartitioningStateLabel]
	requestID := obj.Node.Labels[AmdDcmPartitioningRequestIdLabel]

	logger.Info("Monitoring DCM partitioning progress",
		"node", obj.Node.Name,
		"profileState", profileState,
		"profileStateExists", profileStateExists,
		"requestID", requestID)

	if profileStateExists && profileState == "failure" {
		message := fmt.Sprintf("Partitioning failed (check device config manager logs). Request ID: %s", requestID)
		t.setErrorCondition(obj, message)
		// Clean up taints on failure
		if err := t.cleanupTaintsOnError(ctx, obj.Node); err != nil {
			logger.Error(err, "Failed to clean up taints after partitioning failure")
		}
		return &ctrl.Result{RequeueAfter: longRequeueDelay}, nil
	}

	if profileStateExists && profileState == "success" {
		logger.Info("DCM partitioning completed successfully", "node", obj.Node.Name, "requestID", requestID)

		// Remove the DCM taint as partitioning is complete
		if t.nodeHasTaint(obj.Node, amdDcmTaint) {
			if err := t.removeTaint(ctx, obj.Node, amdDcmTaint); err != nil {
				return nil, fmt.Errorf("failed to remove DCM taint: %w", err)
			}
		}

		t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseWaitingForResourceUpdate, "Waiting for resources and labels to update")
		return &ctrl.Result{RequeueAfter: mediumRequeueDelay}, nil
	}

	// Check for timeout in this phase
	condition := meta.FindStatusCondition(obj.KaiwoNode.Status.Conditions, v1alpha1.PartitioningCompletedConditionType)
	if condition != nil {
		elapsed := time.Since(condition.LastTransitionTime.Time)
		if elapsed >= resourceUpdateTimeout*2 { // Give DCM more time than resource updates
			message := fmt.Sprintf("DCM partitioning timeout after %v. Request ID: %s. Check DCM logs.", elapsed, requestID)
			t.setErrorCondition(obj, message)
			if err := t.cleanupTaintsOnError(ctx, obj.Node); err != nil {
				logger.Error(err, "Failed to clean up taints after timeout")
			}
			return &ctrl.Result{RequeueAfter: longRequeueDelay}, nil
		}
	}

	// Still in progress
	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

// handleWaitingForResourceUpdate waits for natural resource/label updates before restarting pods
func (t *GpuPartitionTask) handleWaitingForResourceUpdate(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if we can extract the current profile successfully (indicates resources are updated)
	updatedProfile, err := extractCurrentProfile(obj.KaiwoNode)
	if err == nil {
		// Successfully extracted profile, check if it matches the desired profile
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = &updatedProfile

		if updatedProfile == obj.KaiwoNode.Spec.Partitioning.Profile {
			// Resources have been updated correctly, skip to waiting for labels
			logger.Info("Resources updated, skipping pod restarts", "node", obj.Node.Name, "profile", updatedProfile)
			t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseWaitingForLabels, "Resources updated, waiting for final label updates")
			return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
		}
	}

	condition := meta.FindStatusCondition(obj.KaiwoNode.Status.Conditions, v1alpha1.PartitioningCompletedConditionType)
	if condition == nil {
		// This shouldn't happen, but handle gracefully by setting a new condition
		t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseWaitingForResourceUpdate, "Waiting for resources and labels to update")
		return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
	}

	elapsed := time.Since(condition.LastTransitionTime.Time)
	if elapsed >= resourceUpdateTimeout {
		logger.Info("Resource update timeout exceeded, proceeding with pod restart routine",
			"node", obj.Node.Name, "elapsed", elapsed)
		t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseRestartingPods, "Resources not updated naturally, restarting pods")
		return &ctrl.Result{Requeue: true}, nil
	}

	// Still within timeout period, continue waiting
	remainingTime := resourceUpdateTimeout - elapsed
	baseutils.Debug(logger, "Still waiting for resource update",
		"node", obj.Node.Name, "elapsed", elapsed, "remaining", remainingTime)

	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

// handleRestartingPods deletes both device plugin and node labeler pods to force restart
func (t *GpuPartitionTask) handleRestartingPods(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Delete device plugin pod if it exists
	if devicePod, err := t.getDaemonSetPod(ctx, KubeAmdGpuNamespace, devicePluginDaemonsetSuffix, obj.Node.Name); err == nil && devicePod != nil {
		logger.Info("Deleting device plugin pod to trigger restart",
			"node", obj.Node.Name, "podName", devicePod.Name)
		if err := t.Client.Delete(ctx, devicePod); err != nil && !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete device plugin pod: %w", err)
		}
	}

	// Delete node labeler pod if it exists
	if labelerPod, err := t.getDaemonSetPod(ctx, KubeAmdGpuNamespace, nodeLabelerDaemonsetSuffix, obj.Node.Name); err == nil && labelerPod != nil {
		logger.Info("Deleting node labeler pod to trigger restart",
			"node", obj.Node.Name, "podName", labelerPod.Name)
		if err := t.Client.Delete(ctx, labelerPod); err != nil && !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete node labeler pod: %w", err)
		}
	}

	// Move to waiting for labels phase
	t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseWaitingForLabels, "Pods deleted, waiting for recreation and label updates")
	return &ctrl.Result{RequeueAfter: mediumRequeueDelay}, nil
}

// handleWaitingForLabels waits for the node labels to be updated and completes partitioning
func (t *GpuPartitionTask) handleWaitingForLabels(_ context.Context, _ *KaiwoNodeWrapper) (*ctrl.Result, error) {
	// This phase will naturally transition to completed state on the next reconcile
	// when isPartitioningComplete() returns true
	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

func (t *GpuPartitionTask) getDaemonSetPod(ctx context.Context, namespace, dsSuffix, nodeName string) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	// Use field selector to limit results to the specific node
	fieldSelector := client.MatchingFields{"spec.nodeName": nodeName}
	if err := t.Client.List(ctx, podList, client.InNamespace(namespace), fieldSelector); err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == KindDaemonSet && strings.HasSuffix(owner.Name, dsSuffix) {
				return &pod, nil
			}
		}
	}
	return nil, nil
}

// Helper methods for state management

func (t *GpuPartitionTask) setErrorCondition(obj *KaiwoNodeWrapper, message string) {
	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusError
	obj.KaiwoNode.Status.Partitioning.Phase = v1alpha1.PartitioningPhaseError
	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Reason:  string(v1alpha1.PartitioningConditionFailed),
		Status:  metav1.ConditionFalse,
		Message: message,
	})
}

func (t *GpuPartitionTask) setInProgressCondition(obj *KaiwoNodeWrapper, phase v1alpha1.PartitioningPhase, message string) {
	obj.KaiwoNode.Status.Partitioning.Phase = phase
	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Reason:  string(v1alpha1.PartitioningConditionInProgress),
		Status:  metav1.ConditionFalse,
		Message: message,
	})
}

// verifyOwnDaemonSetTolerations checks that our own DaemonSets have required tolerations
// This is a verification step - we don't modify them as they should be deployed with correct tolerations
func (t *GpuPartitionTask) verifyOwnDaemonSetTolerations(ctx context.Context) error {
	logger := log.FromContext(ctx)
	requiredTaints := []corev1.Taint{amdDcmTaint, kaiwoPartitioningTaint}

	// Check our own DaemonSets in kube-amd-gpu namespace
	daemonSets := &appsv1.DaemonSetList{}
	if err := t.Client.List(ctx, daemonSets, client.InNamespace(KubeAmdGpuNamespace)); err != nil {
		// If namespace doesn't exist or we can't list, that's fine - components may not be deployed yet
		return nil
	}

	var missingTolerations []string
	for _, ds := range daemonSets.Items {
		// Only check DaemonSets we manage (device-plugin, node-labeller, etc.)
		if !t.isOwnDaemonSet(&ds) {
			continue
		}

		for _, taint := range requiredTaints {
			toleration := corev1.Toleration{
				Key:    taint.Key,
				Value:  taint.Value,
				Effect: taint.Effect,
			}
			if taint.Value == "" {
				toleration.Operator = corev1.TolerationOpExists
			} else {
				toleration.Operator = corev1.TolerationOpEqual
			}

			if !hasToleration(ds.Spec.Template.Spec.Tolerations, toleration) {
				missingTolerations = append(missingTolerations,
					fmt.Sprintf("%s/%s missing toleration for %s", ds.Namespace, ds.Name, taint.Key))
			}
		}
	}

	if len(missingTolerations) > 0 {
		logger.Info("Found DaemonSets without required tolerations - they should be deployed with tolerations",
			"missingTolerations", missingTolerations)
		return fmt.Errorf("missing tolerations: %v", missingTolerations)
	}

	return nil
}

// isOwnDaemonSet checks if a DaemonSet is one that we manage
func (t *GpuPartitionTask) isOwnDaemonSet(ds *appsv1.DaemonSet) bool {
	// Check for known DaemonSet suffixes
	knownSuffixes := []string{devicePluginDaemonsetSuffix, nodeLabelerDaemonsetSuffix}
	for _, suffix := range knownSuffixes {
		if strings.HasSuffix(ds.Name, suffix) {
			return true
		}
	}

	// Check for known labels that identify our DaemonSets
	if ds.Labels != nil {
		if component, exists := ds.Labels["app.kubernetes.io/component"]; exists {
			knownComponents := []string{"device-plugin", "node-labeller", "amd-gpu"}
			for _, knownComponent := range knownComponents {
				if strings.Contains(component, knownComponent) {
					return true
				}
			}
		}
	}

	return false
}

// hasToleration checks if a toleration list contains the specified toleration
func hasToleration(list []corev1.Toleration, want corev1.Toleration) bool {
	for _, toleration := range list {
		if toleration.MatchToleration(&want) {
			return true
		}
	}
	return false
}

func (t *GpuPartitionTask) nodeHasTaint(node *corev1.Node, taint corev1.Taint) bool {
	for _, nodeTaint := range node.Spec.Taints {
		if nodeTaint.MatchTaint(&taint) {
			return true
		}
	}
	return false
}

// addTaint adds a taint to the node with safe patching
func (t *GpuPartitionTask) addTaint(ctx context.Context, node *corev1.Node, taint corev1.Taint) error {
	// Check if taint already exists
	for _, existingTaint := range node.Spec.Taints {
		if existingTaint.MatchTaint(&taint) {
			return nil // Already exists
		}
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get fresh copy of the node
		freshNode := &corev1.Node{}
		if err := t.Client.Get(ctx, client.ObjectKeyFromObject(node), freshNode); err != nil {
			return err
		}

		// Check again if taint was added by another controller
		for _, existingTaint := range freshNode.Spec.Taints {
			if existingTaint.MatchTaint(&taint) {
				return nil // Already exists
			}
		}

		// Add the taint and update
		original := freshNode.DeepCopy()
		freshNode.Spec.Taints = append(freshNode.Spec.Taints, taint)

		if err := t.Client.Patch(ctx, freshNode, client.MergeFrom(original)); err != nil {
			return err
		}

		t.Recorder.Eventf(node, corev1.EventTypeNormal, "TaintNode",
			"Added %s=%s taint", taint.Key, taint.Value)
		return nil
	})
}

// removeTaint removes a taint from the node with safe patching
func (t *GpuPartitionTask) removeTaint(ctx context.Context, node *corev1.Node, taint corev1.Taint) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get fresh copy of the node
		freshNode := &corev1.Node{}
		if err := t.Client.Get(ctx, client.ObjectKeyFromObject(node), freshNode); err != nil {
			return err
		}

		// Filter out the taint
		var newTaints []corev1.Taint
		removed := false
		for _, nodeTaint := range freshNode.Spec.Taints {
			if !nodeTaint.MatchTaint(&taint) {
				newTaints = append(newTaints, nodeTaint)
			} else {
				removed = true
			}
		}

		if !removed {
			return nil // Taint wasn't present
		}

		// Update taints and patch
		original := freshNode.DeepCopy()
		freshNode.Spec.Taints = newTaints

		if err := t.Client.Patch(ctx, freshNode, client.MergeFrom(original)); err != nil {
			return err
		}

		t.Recorder.Eventf(node, corev1.EventTypeNormal, "UntaintNode",
			"Removed %s=%s taint", taint.Key, taint.Value)
		return nil
	})
}

// patchNodeLabels safely updates node labels with conflict retry
func (t *GpuPartitionTask) patchNodeLabels(ctx context.Context, node *corev1.Node, addLabels map[string]string, removeLabels []string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get fresh copy of the node
		freshNode := &corev1.Node{}
		if err := t.Client.Get(ctx, client.ObjectKeyFromObject(node), freshNode); err != nil {
			return err
		}

		original := freshNode.DeepCopy()
		labels := freshNode.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}

		// Add labels
		for key, value := range addLabels {
			labels[key] = value
		}

		// Remove labels
		for _, key := range removeLabels {
			delete(labels, key)
		}

		freshNode.SetLabels(labels)
		return t.Client.Patch(ctx, freshNode, client.MergeFrom(original))
	})
}

// getOffendingPods returns pods that don't tolerate the DCM taint and should be evicted
func (t *GpuPartitionTask) getOffendingPods(ctx context.Context, node *corev1.Node) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	fieldSelector := client.MatchingFields{"spec.nodeName": node.Name}
	if err := t.Client.List(ctx, pods, fieldSelector); err != nil {
		return nil, err
	}

	var offendingPods []corev1.Pod
	for _, pod := range pods.Items {
		// Skip if pod tolerates DCM taint
		if toleratesDCM(pod.Spec.Tolerations) {
			continue
		}

		// Skip system components using allowlist approach instead of blanket kube-system exclusion
		if t.isSystemComponent(&pod) {
			continue
		}

		// Skip pods that are already terminating
		if pod.DeletionTimestamp != nil {
			continue
		}

		offendingPods = append(offendingPods, pod)
	}

	return offendingPods, nil
}

// hasOffendingPods checks if there are any pods remaining on the node that don't tolerate the taint
func (t *GpuPartitionTask) hasOffendingPods(ctx context.Context, node *corev1.Node) (bool, error) {
	offendingPods, err := t.getOffendingPods(ctx, node)
	if err != nil {
		return false, err
	}
	return len(offendingPods) > 0, nil
}

// isSystemComponent checks if a pod is a system component that should not be evicted
func (t *GpuPartitionTask) isSystemComponent(pod *corev1.Pod) bool {
	// Always exclude kube-system namespace
	if pod.Namespace == "kube-system" {
		return true
	}

	// Check for system component labels
	for _, labelKey := range systemComponentLabels {
		if _, hasLabel := pod.Labels[labelKey]; hasLabel {
			// Further check if it's a known system component
			if t.isKnownSystemComponent(pod, labelKey) {
				return true
			}
		}
	}

	// Check for high priority class (system-critical components)
	if pod.Spec.PriorityClassName == "system-cluster-critical" ||
		pod.Spec.PriorityClassName == "system-node-critical" {
		return true
	}

	return false
}

// isKnownSystemComponent checks if a pod with system labels is actually a system component
func (t *GpuPartitionTask) isKnownSystemComponent(pod *corev1.Pod, labelKey string) bool {
	labelValue := pod.Labels[labelKey]

	// Known system components that should not be evicted
	systemComponents := []string{
		"kube-proxy", "flannel", "calico", "weave", "cilium", // Network components
		"coredns", "kube-dns", // DNS components
		"metrics-server", "node-exporter", // Monitoring components
		"csi-", "ebs-csi", "aws-ebs", // Storage components (prefix match)
		"cluster-autoscaler",              // Autoscaling
		"nvidia-device-plugin", "amd-gpu", // Device plugins
	}

	for _, component := range systemComponents {
		if strings.Contains(labelValue, component) {
			return true
		}
	}

	return false
}

// getPodNamesForLogging returns a slice of pod names for logging purposes
func getPodNamesForLogging(pods []corev1.Pod) []string {
	names := make([]string, len(pods))
	for i, pod := range pods {
		names[i] = fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
	}
	return names
}

// cleanupTaintsOnError removes our taints when partitioning fails
func (t *GpuPartitionTask) cleanupTaintsOnError(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)
	taintsToRemove := []corev1.Taint{amdDcmTaint, kaiwoPartitioningTaint}

	for _, taint := range taintsToRemove {
		if t.nodeHasTaint(node, taint) {
			if err := t.removeTaint(ctx, node, taint); err != nil {
				logger.Error(err, "Failed to remove taint during cleanup", "taintKey", taint.Key)
				return err
			}
		}
	}
	return nil
}

// shouldRetryFromError determines if we should retry partitioning from error state
func (t *GpuPartitionTask) shouldRetryFromError(obj *KaiwoNodeWrapper) bool {
	// Check for retry annotation that user can set to force retry
	if obj.KaiwoNode.Annotations != nil {
		if retryAnnotation, exists := obj.KaiwoNode.Annotations["kaiwo.silogen.ai/retry-partitioning"]; exists {
			// Remove the annotation and allow retry
			delete(obj.KaiwoNode.Annotations, "kaiwo.silogen.ai/retry-partitioning")
			return retryAnnotation == "true"
		}
	}

	// Check if we've been in error state for a long time - allow automatic retry
	condition := meta.FindStatusCondition(obj.KaiwoNode.Status.Conditions, v1alpha1.PartitioningCompletedConditionType)
	if condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == string(v1alpha1.PartitioningConditionFailed) {
		elapsed := time.Since(condition.LastTransitionTime.Time)
		// Auto-retry after 10 minutes in case it was a transient failure
		if elapsed >= 10*time.Minute {
			return true
		}
	}

	return false
}

// toleratesDCM checks if a pod tolerates the AMD DCM taint
func toleratesDCM(tolerations []corev1.Toleration) bool {
	for _, toleration := range tolerations {
		if toleration.Key != AmdDcmTaint || toleration.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		switch toleration.Operator {
		case corev1.TolerationOpExists:
			return true
		case corev1.TolerationOpEqual:
			if toleration.Value == AmdDcmUpValue {
				return true
			}
		}
	}
	return false
}
