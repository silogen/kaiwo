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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
)

// SetupWithManager registers all infrastructure controllers with the manager.
func SetupWithManager(mgr ctrl.Manager, clientset kubernetes.Interface) error {
	// Add field indexers
	if err := setupFieldIndexers(mgr); err != nil {
		return err
	}

	// Register PartitioningPlan controller
	if err := (&PartitioningPlanReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("partitioning-plan-controller"),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Register NodePartitioning controller
	if err := (&NodePartitioningReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("node-partitioning-controller"),
		Clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}

// setupFieldIndexers adds field indexers for efficient lookups.
func setupFieldIndexers(mgr ctrl.Manager) error {
	// Index NodePartitioning by spec.nodeName for quick lookup by node name
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&infrastructurev1alpha1.NodePartitioning{},
		".spec.nodeName",
		func(obj client.Object) []string {
			np := obj.(*infrastructurev1alpha1.NodePartitioning)
			return []string{np.Spec.NodeName}
		},
	); err != nil {
		return err
	}

	// Index Pod by spec.nodeName for efficient node-based pod lookups (used in draining)
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&corev1.Pod{},
		"spec.nodeName",
		func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		},
	); err != nil {
		return err
	}

	return nil
}
