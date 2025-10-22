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

package partitioning

import (
	"context"
	goerrors "errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ErrDrainInProgress indicates that drain is still in progress (pods are being evicted).
// This is not a failure, just a signal that the operation needs more time.
var ErrDrainInProgress = goerrors.New("drain in progress")

// CordonNode marks a node as unschedulable.
func CordonNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	// Retry on conflict
	return retryOnConflict(ctx, func() error {
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
			return err
		}

		logger.Info("Cordoned node", "node", nodeName)
		return nil
	})
}

// UncordonNode marks a node as schedulable.
func UncordonNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	// Retry on conflict
	return retryOnConflict(ctx, func() error {
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
			return err
		}

		logger.Info("Uncordoned node", "node", nodeName)
		return nil
	})
}

// TaintNode adds the amd-dcm taint to a node.
func TaintNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	// Retry on conflict
	return retryOnConflict(ctx, func() error {
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
			return err
		}

		logger.Info("Tainted node", "node", nodeName, "taint", TaintKey)
		return nil
	})
}

// UntaintNode removes the amd-dcm taint from a node.
func UntaintNode(ctx context.Context, c client.Client, nodeName string) error {
	logger := log.FromContext(ctx)

	// Retry on conflict
	return retryOnConflict(ctx, func() error {
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
			return err
		}

		logger.Info("Untainted node", "node", nodeName, "taint", TaintKey)
		return nil
	})
}

// DrainNode evicts all pods from a node (except those that tolerate the amd-dcm taint).
func DrainNode(
	ctx context.Context,
	c client.Client,
	clientset kubernetes.Interface,
	nodeName string,
) error {
	logger := log.FromContext(ctx)

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
		logger.Info("No pods to evict - drain complete", "node", nodeName)
		return nil
	}

	// Pods still need to be evicted - the taint will cause them to terminate
	// Return ErrDrainInProgress to signal that we need to wait (not a failure)
	logger.Info("Waiting for pods to be evicted by taint", "node", nodeName, "remainingPods", len(podsToEvict))
	return fmt.Errorf("waiting for %d pods to be evicted: %w", len(podsToEvict), ErrDrainInProgress)
}

// toleratesAMDDCMTaint checks if a pod tolerates the amd-dcm taint.
func toleratesAMDDCMTaint(pod *corev1.Pod) bool {
	for _, toleration := range pod.Spec.Tolerations {
		// Check for wildcard toleration (empty key matches all taints with the effect)
		if toleration.Key == "" && toleration.Operator == corev1.TolerationOpExists {
			// Empty key with Exists operator matches all taints
			if toleration.Effect == "" || toleration.Effect == corev1.TaintEffectNoExecute {
				return true
			}
		}

		// Check for specific amd-dcm taint toleration
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

// retryOnConflict retries a function if it encounters a conflict error.
// This is useful for node operations that may conflict with other controllers.
func retryOnConflict(ctx context.Context, fn func() error) error {
	backoff := wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
	}

	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		err := fn()
		if err == nil {
			return true, nil // Success
		}

		// Retry on conflict
		if errors.IsConflict(err) {
			return false, nil // Retry
		}

		// Don't retry other errors
		return false, err
	})
}
