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

package infrastructure

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
)

// CordonNode marks a node as unschedulable.
func CordonNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	if node.Spec.Unschedulable {
		logger.V(1).Info("Node already cordoned", "node", nodeName)
		return nil
	}

	node.Spec.Unschedulable = true
	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to cordon node %s: %w", nodeName, err)
	}

	logger.Info("Cordoned node", "node", nodeName)
	return nil
}

// UncordonNode marks a node as schedulable.
func UncordonNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if errors.IsNotFound(err) {
			return nil // Node already gone
		}
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	if !node.Spec.Unschedulable {
		logger.V(1).Info("Node already uncordoned", "node", nodeName)
		return nil
	}

	node.Spec.Unschedulable = false
	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to uncordon node %s: %w", nodeName, err)
	}

	logger.Info("Uncordoned node", "node", nodeName)
	return nil
}

// TaintNode adds the amd-dcm taint to a node.
func TaintNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	// Check if taint already exists
	taint := corev1.Taint{
		Key:    TaintKey,
		Value:  TaintValue,
		Effect: corev1.TaintEffectNoExecute,
	}

	for _, existingTaint := range node.Spec.Taints {
		if existingTaint.Key == taint.Key && existingTaint.Effect == taint.Effect {
			logger.V(1).Info("Node already tainted", "node", nodeName, "taint", TaintKey)
			return nil
		}
	}

	// Add taint
	node.Spec.Taints = append(node.Spec.Taints, taint)
	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to taint node %s: %w", nodeName, err)
	}

	logger.Info("Tainted node", "node", nodeName, "taint", TaintKey)
	return nil
}

// UntaintNode removes the amd-dcm taint from a node.
func UntaintNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if errors.IsNotFound(err) {
			return nil // Node already gone
		}
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	// Remove taint
	var newTaints []corev1.Taint
	found := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == TaintKey && taint.Effect == corev1.TaintEffectNoExecute {
			found = true
			continue // Skip this taint
		}
		newTaints = append(newTaints, taint)
	}

	if !found {
		logger.V(1).Info("Node taint not present", "node", nodeName, "taint", TaintKey)
		return nil
	}

	node.Spec.Taints = newTaints
	if err := c.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to untaint node %s: %w", nodeName, err)
	}

	logger.Info("Untainted node", "node", nodeName, "taint", TaintKey)
	return nil
}

// DrainNode evicts all pods from a node (except those that tolerate the amd-dcm taint).
func DrainNode(
	ctx context.Context,
	c client.Client,
	clientset kubernetes.Interface,
	nodeName string,
	policy infrastructurev1alpha1.DrainPolicy,
) error {
	logger := log.FromContext(ctx)

	if !policy.Enabled {
		logger.Info("Drain disabled, skipping", "node", nodeName)
		return nil
	}

	// List all pods on the node
	var podList corev1.PodList
	if err := c.List(ctx, &podList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
	}); err != nil {
		return fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
	}

	logger.Info("Draining node", "node", nodeName, "totalPods", len(podList.Items))

	// Filter pods that need to be evicted
	var podsToEvict []corev1.Pod
	for _, pod := range podList.Items {
		// Skip pods that are already terminating
		if pod.DeletionTimestamp != nil {
			continue
		}

		// Skip pods that tolerate the amd-dcm taint
		if toleratesAMDDCMTaint(&pod) {
			logger.V(1).Info("Pod tolerates amd-dcm taint, skipping eviction",
				"pod", pod.Name, "namespace", pod.Namespace)
			continue
		}

		// Skip static pods (mirror pods)
		if isStaticPod(&pod) {
			logger.V(1).Info("Skipping static pod", "pod", pod.Name, "namespace", pod.Namespace)
			continue
		}

		podsToEvict = append(podsToEvict, pod)
	}

	if len(podsToEvict) == 0 {
		logger.Info("No pods to evict", "node", nodeName)
		return nil
	}

	logger.Info("Evicting pods", "node", nodeName, "count", len(podsToEvict))

	// Evict pods
	for _, pod := range podsToEvict {
		if err := evictPod(ctx, clientset, &pod, policy); err != nil {
			return fmt.Errorf("failed to evict pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}

	// Wait for pods to terminate
	deadline := time.Now().Add(time.Duration(policy.TimeoutSeconds) * time.Second)
	for {
		if time.Now().After(deadline) {
			// Check remaining pods
			remaining, err := getRemainingPods(ctx, c, podsToEvict)
			if err != nil {
				return fmt.Errorf("failed to check remaining pods: %w", err)
			}
			if len(remaining) > 0 {
				return fmt.Errorf("drain timeout: %d pods still running after %ds", len(remaining), policy.TimeoutSeconds)
			}
			break
		}

		// Check if all pods are gone
		remaining, err := getRemainingPods(ctx, c, podsToEvict)
		if err != nil {
			return fmt.Errorf("failed to check remaining pods: %w", err)
		}

		if len(remaining) == 0 {
			logger.Info("All pods evicted successfully", "node", nodeName)
			return nil
		}

		logger.V(1).Info("Waiting for pods to terminate", "node", nodeName, "remaining", len(remaining))
		time.Sleep(5 * time.Second)
	}

	return nil
}

// evictPod evicts a single pod.
func evictPod(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod, policy infrastructurev1alpha1.DrainPolicy) error {
	logger := log.FromContext(ctx)

	switch policy.EvictionKind {
	case "Eviction":
		// Use Eviction API (respects PDBs)
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}

		err := clientset.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction)
		if err != nil {
			if errors.IsTooManyRequests(err) && policy.RespectPDB {
				logger.Info("Eviction rejected by PDB, will retry", "pod", pod.Name, "namespace", pod.Namespace)
				return nil // Don't fail, just wait
			}
			if errors.IsNotFound(err) {
				return nil // Pod already gone
			}
			return fmt.Errorf("eviction failed: %w", err)
		}

		logger.V(1).Info("Evicted pod", "pod", pod.Name, "namespace", pod.Namespace)
		return nil

	case "Delete":
		// Direct deletion (ignores PDBs)
		if err := clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
			if errors.IsNotFound(err) {
				return nil // Pod already gone
			}
			return fmt.Errorf("delete failed: %w", err)
		}

		logger.V(1).Info("Deleted pod", "pod", pod.Name, "namespace", pod.Namespace)
		return nil

	case "None":
		logger.V(1).Info("Eviction disabled, skipping pod", "pod", pod.Name, "namespace", pod.Namespace)
		return nil

	default:
		return fmt.Errorf("unknown eviction kind: %s", policy.EvictionKind)
	}
}

// getRemainingPods returns the list of pods from the original eviction list that are still running.
func getRemainingPods(ctx context.Context, c client.Client, originalPods []corev1.Pod) ([]corev1.Pod, error) {
	var remaining []corev1.Pod

	for _, originalPod := range originalPods {
		var pod corev1.Pod
		err := c.Get(ctx, types.NamespacedName{
			Name:      originalPod.Name,
			Namespace: originalPod.Namespace,
		}, &pod)
		if err != nil {
			if errors.IsNotFound(err) {
				continue // Pod is gone
			}
			return nil, err
		}

		// Pod still exists
		remaining = append(remaining, pod)
	}

	return remaining, nil
}

// toleratesAMDDCMTaint checks if a pod tolerates the amd-dcm taint.
func toleratesAMDDCMTaint(pod *corev1.Pod) bool {
	for _, toleration := range pod.Spec.Tolerations {
		if toleration.Key == TaintKey && toleration.Effect == corev1.TaintEffectNoExecute {
			// Check if operator is Equal and value matches, or operator is Exists
			if toleration.Operator == corev1.TolerationOpExists {
				return true
			}
			if toleration.Operator == corev1.TolerationOpEqual && toleration.Value == TaintValue {
				return true
			}
		}
	}
	return false
}

// isStaticPod checks if a pod is a static pod (mirror pod).
func isStaticPod(pod *corev1.Pod) bool {
	// Static pods have an owner reference with "Node" kind
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "Node" {
			return true
		}
	}

	// Alternative check: static pods have the annotation
	if _, ok := pod.Annotations["kubernetes.io/config.mirror"]; ok {
		return true
	}

	return false
}
