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

package kueue

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// GetLocalQueueNamesForCluster returns the set of LocalQueue keys (namespace+name)
// that reference the given ClusterQueue. Keys are NamespacedName for disambiguation.
func GetLocalQueueNamesForCluster(ctx context.Context, c client.Client, clusterQueueName string) (map[types.NamespacedName]struct{}, error) {
	var lqList kueuev1beta1.LocalQueueList
	if err := c.List(ctx, &lqList); err != nil {
		return nil, fmt.Errorf("failed to list LocalQueues: %w", err)
	}
	keys := make(map[types.NamespacedName]struct{})
	for _, lq := range lqList.Items {
		if string(lq.Spec.ClusterQueue) == clusterQueueName {
			keys[types.NamespacedName{Namespace: lq.Namespace, Name: lq.Name}] = struct{}{}
		}
	}
	return keys, nil
}

// ResolveClusterQueueFromWorkload maps a Workload -> LocalQueue -> ClusterQueue.
// Falls back to returning the LocalQueue name if lookup fails.
func ResolveClusterQueueFromWorkload(ctx context.Context, c client.Client, wl *kueuev1beta1.Workload) string {
	if wl == nil {
		return ""
	}
	lqName := string(wl.Spec.QueueName)
	if lqName == "" && wl.Labels != nil {
		lqName = wl.Labels["kueue.x-k8s.io/queue-name"]
	}
	if lqName == "" {
		return ""
	}
	var lq kueuev1beta1.LocalQueue
	if err := c.Get(ctx, client.ObjectKey{Name: lqName, Namespace: wl.Namespace}, &lq); err != nil {
		// Fall back: assume LocalQueue name equals ClusterQueue (legacy behavior)
		return lqName
	}
	return string(lq.Spec.ClusterQueue)
}
