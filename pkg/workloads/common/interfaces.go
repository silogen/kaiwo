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

package common

import (
	"context"

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
}
