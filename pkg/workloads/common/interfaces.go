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

package common

import (
	"context"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GroupReconciler provides a way to link related reconcilers and observe their status
type GroupReconciler interface {
	GetResourceReconcilers() []ResourceReconciler

	// ObserveStatus gives the current status and conditions of this reconciler group
	ObserveStatus(ctx context.Context, k8sClient client.Client, previousWorkloadStatus kaiwo.WorkloadStatus) (kaiwo.WorkloadStatus, []metav1.Condition, error)
}

// ResourceReconciler is an interface for building and updating Kubernetes resources
type ResourceReconciler interface {
	// GetInitializedObject returns an object that has the correct TypeMeta and ObjectMeta attributes set
	GetInitializedObject() client.Object

	// BuildDesired builds the desired downstream workload object state
	BuildDesired(ctx context.Context, clusterCtx ClusterContext) (client.Object, error)

	// MutateActual provides a way to mutate an existing object to match the desired state
	MutateActual(ctx context.Context, clusterCtx ClusterContext, actual client.Object) error

	// ObserveStatus returns the current status of the object.
	// The object is guaranteed to have existed at least right before this method is called, so
	// a not-nil check is not required.
	ObserveStatus(ctx context.Context, k8sClient client.Client, obj client.Object, previousWorkloadStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error)
}

// KaiwoWorkload is a generic interface for any Kaiwo workload
type KaiwoWorkload interface {
	// GetKaiwoWorkloadObject returns the underlying Kaiwo workload object (KaiwoJob or KaiwoService)
	GetKaiwoWorkloadObject() client.Object

	// GetCommonSpec returns the common spec object
	GetCommonSpec() kaiwo.CommonMetaSpec

	// GetCommonStatusSpec returns the common status object
	GetCommonStatusSpec() *kaiwo.CommonStatusSpec
}

// WorkloadReconciler extends ResourceReconciler with workload-specific methods
type WorkloadReconciler interface {
	ResourceReconciler
	KaiwoWorkload

	// GetKueueWorkloads returns a list of the Kueue Workloads that govern the admission of this workload
	GetKueueWorkloads(ctx context.Context, k8sClient client.Client) ([]kueuev1beta1.Workload, error)
}
