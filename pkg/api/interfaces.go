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

package api

import (
	"context"

	config2 "github.com/silogen/kaiwo/pkg/config"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GroupReconciler provides a way to link related reconcilers and observe their status
type GroupReconciler interface {
	GetResourceReconcilers(ctx context.Context) []ResourceReconciler
}

// ResourceReconciler is an interface for building and updating Kubernetes resources
type ResourceReconciler interface {
	// GetInitializedObject returns an object that has the correct TypeMeta and ObjectMeta attributes set
	GetInitializedObject() client.Object

	// BuildDesired builds the desired downstream workload object state
	BuildDesired(ctx context.Context, clusterCtx ClusterContext) (client.Object, error)

	// MutateActual provides a way to mutate an existing object to match the desired state
	MutateActual(ctx context.Context, clusterCtx ClusterContext, actual client.Object) error
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

func GetClusterQueueName(ctx context.Context, workload KaiwoWorkload) string {
	if clusterQueue := workload.GetCommonSpec().ClusterQueue; clusterQueue != "" {
		return clusterQueue
	} else {
		config := config2.ConfigFromContext(ctx)
		return config.DefaultClusterQueueName
	}
}

// ClusterContext provides context of the cluster and its resources to help build downstream objects
type ClusterContext struct {
	Nodes            []kaiwo.KaiwoNode
	KaiwoQueueConfig *kaiwo.KaiwoQueueConfig
}
