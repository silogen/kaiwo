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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

	devicePluginDaemonsetSuffix = "device-plugin"
	nodeLabelerDaemonsetSuffix  = "node-labeller"

	KubeAmdGpuNamespace                 = "kube-amd-gpu"
	AmdDeviceConfigManagerConfigMapName = "config-manager-config"

	// Timing constants
	shortRequeueDelay  = 5 * time.Second
	mediumRequeueDelay = 10 * time.Second
	longRequeueDelay   = 30 * time.Second

	resourceUpdateTimeout = 30 * time.Second
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
)

func (t *GpuPartitionTask) Name() string { return "GpuPartition" }

// extractCurrentProfile extracts the currently applied profile
func extractCurrentProfile(kaiwoNode *v1alpha1.KaiwoNode) (v1alpha1.GpuPartitioningProfile, error) {
	gpus := kaiwoNode.Status.Resources.Gpus
	if gpus.LogicalVramPerGpu == nil {
		return "", fmt.Errorf("no vRAM information, labels are likely not set correctly")
	}
	if gpus.PhysicalCount != nil && *gpus.PhysicalCount != gpus.LogicalCount {
		return v1alpha1.GpuPartitioningProfileAmdCpx, nil
	}
	return v1alpha1.GpuPartitioningProfileAmdSpx, nil
}

func (t *GpuPartitionTask) Run(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	// Return if partitioning is disabled
	if !obj.KaiwoNode.Spec.Partitioning.Enabled {
		return nil, nil
	}

	// If the node is not a GPU node, return with error
	if obj.KaiwoNode.Status.NodeType != v1alpha1.NodeTypeGpu {
		return t.setErrorCondition(obj, "Cannot partition a node without GPUs"), nil
	}

	currentProfile, err := extractCurrentProfile(obj.KaiwoNode)
	if err != nil {
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = nil
	} else {
		obj.KaiwoNode.Status.Partitioning.AppliedProfile = &currentProfile
	}

	// Check if partitioning is complete and matches desired state
	if t.isPartitioningComplete(obj) {
		return t.handleCompletedPartitioning(ctx, obj)
	}

	// Handle partitioning phases
	return t.handlePartitioningPhase(ctx, obj)
}

// isPartitioningComplete checks if the current state matches the desired state
func (t *GpuPartitionTask) isPartitioningComplete(obj *KaiwoNodeWrapper) bool {
	profileState, profileStateExists := obj.Node.Labels[AmdDcmPartitioningStateLabel]
	return obj.KaiwoNode.Status.Partitioning.AppliedProfile != nil &&
		*obj.KaiwoNode.Status.Partitioning.AppliedProfile == obj.KaiwoNode.Spec.Partitioning.Profile &&
		profileStateExists && profileState == "success"
}

// handleCompletedPartitioning handles the case where partitioning is already complete
func (t *GpuPartitionTask) handleCompletedPartitioning(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
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
		t.removeTaint(ctx, obj.Node, kaiwoPartitioningTaint)
	}
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
		logger.Info("Partitioning is in error state", "node", obj.Node.Name)
		return nil, nil

	default:
		logger.Error(nil, "Unknown partitioning phase", "phase", obj.KaiwoNode.Status.Partitioning.Phase)
		return t.setErrorCondition(obj, fmt.Sprintf("Unknown partitioning phase: %s", obj.KaiwoNode.Status.Partitioning.Phase)), nil
	}
}

// handleInitializePartitioning sets up tolerations and taints for partitioning
func (t *GpuPartitionTask) handleInitializePartitioning(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	if err := t.setupSystemTolerations(ctx); err != nil {
		return nil, fmt.Errorf("failed to setup system tolerations: %w", err)
	}

	if !t.nodeHasTaint(obj.Node, amdDcmTaint) {
		t.addTaint(ctx, obj.Node, amdDcmTaint)
	}

	t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseDrainingUntoleratedPods, "Draining untolerated pods")

	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

// handleDrainingPods waits for untolerated pods to be drained
func (t *GpuPartitionTask) handleDrainingPods(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	hasOffending, err := t.hasOffendingPods(ctx, obj.Node)
	if err != nil {
		return nil, fmt.Errorf("failed to check offending pods: %w", err)
	}

	if hasOffending {
		return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
	}

	// No more offending pods, proceed to apply partitions
	t.applyPartitioningLabels(obj)
	t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseApplyingPartitions, "Applying partitioning")

	return &ctrl.Result{RequeueAfter: shortRequeueDelay}, nil
}

func (t *GpuPartitionTask) applyPartitioningLabels(obj *KaiwoNodeWrapper) {
	// Setting the profile label triggers the partitioning
	obj.Node.Labels[AmdDcmProfileLabel] = string(obj.KaiwoNode.Spec.Partitioning.Profile)

	// Remove any previous profile state label so we can track the new state
	labels := obj.Node.GetLabels()
	delete(labels, AmdDcmPartitioningStateLabel)
	obj.Node.SetLabels(labels)
}

// handleApplyingPartitions monitors the partitioning process
func (t *GpuPartitionTask) handleApplyingPartitions(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
	profileState, profileStateExists := obj.Node.Labels[AmdDcmPartitioningStateLabel]

	if profileStateExists && profileState == "failure" {
		return t.setErrorCondition(obj, "Partitioning failed (check device config manager logs)"), nil
	}

	if profileStateExists && profileState == "success" {
		// Remove the DCM taint as partitioning is complete
		if t.nodeHasTaint(obj.Node, amdDcmTaint) {
			t.removeTaint(ctx, obj.Node, amdDcmTaint)
		}

		t.setInProgressCondition(obj, v1alpha1.PartitioningPhaseWaitingForResourceUpdate, "Waiting for resources and labels to update")
		return &ctrl.Result{RequeueAfter: mediumRequeueDelay}, nil
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
func (t *GpuPartitionTask) handleWaitingForLabels(ctx context.Context, obj *KaiwoNodeWrapper) (*ctrl.Result, error) {
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
			if owner.Kind == "DaemonSet" && strings.HasSuffix(owner.Name, dsSuffix) {
				return &pod, nil
			}
		}
	}
	return nil, nil
}

// Helper methods for state management

func (t *GpuPartitionTask) setErrorCondition(obj *KaiwoNodeWrapper, message string) *ctrl.Result {
	obj.KaiwoNode.Status.Status = v1alpha1.KaiwoNodeStatusError
	obj.KaiwoNode.Status.Partitioning.Phase = v1alpha1.PartitioningPhaseError
	meta.SetStatusCondition(&obj.KaiwoNode.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.PartitioningCompletedConditionType,
		Reason:  string(v1alpha1.PartitioningConditionFailed),
		Status:  metav1.ConditionFalse,
		Message: message,
	})
	return nil
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

func (t *GpuPartitionTask) setupSystemTolerations(ctx context.Context) error {
	namespaces := []string{"kube-system", "kube-amd-gpu"}
	taints := []corev1.Taint{amdDcmTaint, kaiwoPartitioningTaint}

	for _, namespace := range namespaces {
		for _, taint := range taints {
			if err := t.ensureSystemTolerations(ctx, taint, namespace); err != nil {
				return fmt.Errorf("failed to ensure tolerations for %s in %s: %w", taint.Key, namespace, err)
			}
		}
	}
	return nil
}

// daemonSetPodExists checks if a pod exists for the given daemonset on the node
func (t *GpuPartitionTask) daemonSetPodExists(ctx context.Context, namespace, dsSuffix, nodeName string) (bool, error) {
	podList := &corev1.PodList{}
	fieldSelector := client.MatchingFields{"spec.nodeName": nodeName}
	if err := t.Client.List(ctx, podList, client.InNamespace(namespace), fieldSelector); err != nil {
		return false, err
	}

	for _, pod := range podList.Items {
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" && strings.HasSuffix(owner.Name, dsSuffix) {
				return true, nil
			}
		}
	}
	return false, nil
}

// Existing utility methods remain the same...

// ensureSystemTolerations adds tolerations to kube-system Deployments and DaemonSets
func (t *GpuPartitionTask) ensureSystemTolerations(ctx context.Context, taint corev1.Taint, namespace string) error {
	logger := log.FromContext(ctx)

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

	// Deployments
	deployments := &appsv1.DeploymentList{}
	if err := t.Client.List(ctx, deployments, client.InNamespace(namespace)); err != nil {
		return err
	}
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if !hasToleration(deployment.Spec.Template.Spec.Tolerations, toleration) {
			deployment.Spec.Template.Spec.Tolerations = append(deployment.Spec.Template.Spec.Tolerations, toleration)
			if err := t.Client.Update(ctx, deployment); err != nil {
				logger.Error(err, "failed to patch Deployment tolerations", "deployment", deployment.Name)
				return err
			}
			t.Recorder.Eventf(deployment, corev1.EventTypeNormal, "AddToleration",
				"Added toleration to Deployment %s in %s", deployment.Name, namespace)
		}
	}

	// DaemonSets
	daemonSets := &appsv1.DaemonSetList{}
	if err := t.Client.List(ctx, daemonSets, client.InNamespace(namespace)); err != nil {
		return err
	}
	for i := range daemonSets.Items {
		daemonSet := &daemonSets.Items[i]
		if !hasToleration(daemonSet.Spec.Template.Spec.Tolerations, toleration) {
			daemonSet.Spec.Template.Spec.Tolerations = append(daemonSet.Spec.Template.Spec.Tolerations, toleration)
			if err := t.Client.Update(ctx, daemonSet); err != nil {
				logger.Error(err, "failed to patch DaemonSet tolerations", "daemonset", daemonSet.Name)
				return err
			}
			t.Recorder.Eventf(daemonSet, corev1.EventTypeNormal, "AddToleration",
				"Added toleration to DaemonSet %s in %s", daemonSet.Name, namespace)
		}
	}
	return nil
}

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

// addTaint adds the AMD DCM taint to the node
func (t *GpuPartitionTask) addTaint(ctx context.Context, node *corev1.Node, taint corev1.Taint) {
	node.Spec.Taints = append(node.Spec.Taints, taint)
	t.Recorder.Eventf(node, corev1.EventTypeNormal, "TaintNode",
		"Added %s=%s taint", taint.Key, taint.Value)
}

// removeTaint removes the AMD DCM taint from the node
func (t *GpuPartitionTask) removeTaint(ctx context.Context, node *corev1.Node, taint corev1.Taint) {
	var newTaints []corev1.Taint
	for _, nodeTaint := range node.Spec.Taints {
		if !nodeTaint.MatchTaint(&taint) {
			newTaints = append(newTaints, nodeTaint)
		}
	}

	if len(newTaints) != len(node.Spec.Taints) {
		node.Spec.Taints = newTaints
		t.Recorder.Eventf(node, corev1.EventTypeNormal, "UntaintNode",
			"Removed %s=%s taint", taint.Key, taint.Value)
	}
}

// hasOffendingPods checks if there are any pods remaining on the node that don't tolerate the taint
func (t *GpuPartitionTask) hasOffendingPods(ctx context.Context, node *corev1.Node) (bool, error) {
	pods := &corev1.PodList{}
	fieldSelector := client.MatchingFields{"spec.nodeName": node.Name}
	if err := t.Client.List(ctx, pods, fieldSelector); err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		if toleratesDCM(pod.Spec.Tolerations) {
			continue
		}
		if pod.ObjectMeta.Namespace == "kube-system" {
			continue
		}
		return true, nil
	}
	return false, nil
}

func toleratesDCM(tolerations []corev1.Toleration) bool {
	for _, t := range tolerations {
		if t.Key != AmdDcmTaint || t.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		switch t.Operator {
		case corev1.TolerationOpExists:
			return true
		case corev1.TolerationOpEqual:
			if t.Value == AmdDcmUpValue {
				return true
			}
		}
	}
	return false
}

// isDaemonSetPodReady returns true if there exists a Pod in `namespace`
// owned by DaemonSet `dsName`, scheduled on `nodeName`, whose Phase is Running
// and which has the Ready condition == True.
func (t *GpuPartitionTask) isDaemonSetPodReady(ctx context.Context, namespace, dsSuffix, nodeName string) (bool, error) {
	podList := &corev1.PodList{}
	fieldSelector := client.MatchingFields{"spec.nodeName": nodeName}
	if err := t.Client.List(ctx, podList, client.InNamespace(namespace), fieldSelector); err != nil {
		return false, err
	}

	for _, pod := range podList.Items {
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" && strings.HasSuffix(owner.Name, dsSuffix) {
				if pod.Status.Phase != corev1.PodRunning {
					return false, nil
				}
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						return true, nil
					}
				}
				return false, nil
			}
		}
	}

	// No matching Pod found
	return false, nil
}
