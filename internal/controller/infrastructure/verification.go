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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
)

// VerifyPartitioning verifies that a node has been successfully partitioned.
// It checks:
// 1. The GPU ready label is present
// 2. Expected resources are present in node.status.allocatable
// 3. Resource counts match expectations
func VerifyPartitioning(
	ctx context.Context,
	c client.Client,
	nodeName string,
	profile *infrastructurev1alpha1.PartitioningProfile,
	verification infrastructurev1alpha1.VerificationSpec,
) error {
	logger := log.FromContext(ctx)

	deadline := time.Now().Add(time.Duration(verification.TimeoutSeconds) * time.Second)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("verification timeout after %ds", verification.TimeoutSeconds)
		}

		// Get current node state
		var node corev1.Node
		if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("node not found during verification")
			}
			return fmt.Errorf("failed to get node: %w", err)
		}

		// Check GPU ready label
		if verification.GpuReadyLabel != "" {
			if ready, ok := node.Labels[verification.GpuReadyLabel]; !ok || ready != "true" {
				logger.V(1).Info("GPU ready label not set, waiting",
					"node", nodeName, "label", verification.GpuReadyLabel)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		// Check expected resources
		allResourcesPresent := true
		for _, expectedResource := range profile.Spec.ExpectedResources {
			quantity, exists := node.Status.Allocatable[corev1.ResourceName(expectedResource.Name)]
			if !exists {
				logger.V(1).Info("Expected resource not present in node allocatable, waiting",
					"node", nodeName, "resource", expectedResource.Name)
				allResourcesPresent = false
				break
			}

			// Check count
			actualCount := quantity.Value()
			expectedCount := int64(expectedResource.Count)
			if actualCount != expectedCount {
				logger.V(1).Info("Resource count mismatch, waiting",
					"node", nodeName,
					"resource", expectedResource.Name,
					"expected", expectedCount,
					"actual", actualCount)
				allResourcesPresent = false
				break
			}
		}

		if !allResourcesPresent {
			time.Sleep(5 * time.Second)
			continue
		}

		// Check additional required plugin resources from verification spec
		for _, resourceName := range verification.RequirePluginResources {
			if _, exists := node.Status.Allocatable[corev1.ResourceName(resourceName)]; !exists {
				logger.V(1).Info("Required plugin resource not present, waiting",
					"node", nodeName, "resource", resourceName)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		// All checks passed
		logger.Info("Node verification succeeded",
			"node", nodeName,
			"profile", profile.Name)
		return nil
	}
}

// WaitForOperatorReady waits for the AMD GPU operator to be ready on a node.
// This is a simplified check that verifies the device plugin DaemonSet pod is running.
func WaitForOperatorReady(
	ctx context.Context,
	c client.Client,
	nodeName string,
	timeoutSeconds int32,
) error {
	logger := log.FromContext(ctx)

	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("operator ready timeout after %ds", timeoutSeconds)
		}

		// Check if device plugin pods are running on the node
		// AMD device plugin pods typically run in kube-amd-gpu namespace
		var podList corev1.PodList
		if err := c.List(ctx, &podList, client.InNamespace("kube-amd-gpu")); err != nil {
			logger.V(1).Info("Failed to list device plugin pods", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Look for a device plugin pod on this node
		devicePluginReady := false
		for _, pod := range podList.Items {
			if pod.Spec.NodeName != nodeName {
				continue
			}

			// Check if this is likely a device plugin pod (by name pattern)
			if contains(pod.Name, "device-plugin") || contains(pod.Name, "amd-gpu") {
				// Check if pod is ready
				if isPodReady(&pod) {
					devicePluginReady = true
					break
				}
			}
		}

		if devicePluginReady {
			logger.Info("AMD GPU operator device plugin is ready", "node", nodeName)
			return nil
		}

		logger.V(1).Info("Waiting for AMD GPU operator device plugin to be ready", "node", nodeName)
		time.Sleep(5 * time.Second)
	}
}

// isPodReady checks if a pod is in Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// contains checks if a string contains a substring (helper function).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
