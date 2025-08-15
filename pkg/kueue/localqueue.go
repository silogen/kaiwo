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

	"github.com/silogen/kaiwo/pkg/common"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

// EnsureLocalQueue makes sure that a local queue exists. If it doesn't, it attempts to create it.
// Creation will fail if the referenced ClusterQueue does not exist within the KaiwoQueueConfig, or if the
// referenced ClusterQueue lists namespaces and the provided one does not match any of them.
func EnsureLocalQueue(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, clusterQueueName string, name string, namespace string) error {
	logger := log.FromContext(ctx)

	localQueue := &kueuev1beta1.LocalQueue{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, localQueue); err == nil {
		return nil
	} else if apierrors.IsNotFound(err) {
		kaiwoQueueConfig := &v1alpha1.KaiwoQueueConfig{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: common.KaiwoQueueConfigName}, kaiwoQueueConfig); err != nil {
			return fmt.Errorf("failed to get KaiwoQueueConfig: %w", err)
		}
		for _, clusterQueueConfig := range kaiwoQueueConfig.Spec.ClusterQueues {
			if clusterQueueName == clusterQueueConfig.Name {
				if len(clusterQueueConfig.Namespaces) == 0 {
					logger.Info(fmt.Sprintf("Creating a new LocalQueue %s/%s", namespace, clusterQueueName))

					clusterQueue := &kueuev1beta1.ClusterQueue{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: clusterQueueName}, clusterQueue); err != nil {
						return fmt.Errorf("failed to get cluster queue: %w", err)
					}

					localQueue = &kueuev1beta1.LocalQueue{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
						},
						Spec: kueuev1beta1.LocalQueueSpec{
							ClusterQueue: kueuev1beta1.ClusterQueueReference(clusterQueueName),
						},
					}

					if err := controllerutil.SetOwnerReference(clusterQueue, localQueue, scheme); err != nil {
						return fmt.Errorf("failed to set owner reference: %w", err)
					}

					// Create the LocalQueue
					err = k8sClient.Create(ctx, localQueue)
					if err != nil {
						logger.Error(err, "Failed to create LocalQueue", "Name", name, "Namespace", namespace)
						return fmt.Errorf("failed to create LocalQueue %s in namespace %s: %w", clusterQueueName, namespace, err)
					}
					return nil
				} else {
					// The cluster queue defines one or more namespaces, ensure the one provided matches
					for _, clusterQueueNamespace := range clusterQueueConfig.Namespaces {
						if clusterQueueNamespace == clusterQueueName {
							return nil
						}
					}
					return fmt.Errorf("cluster queue %s defines one or more namespaces, use one of these namespaces to use this cluster queue", clusterQueueConfig.Name)
				}
			}
		}
		return fmt.Errorf("the referenced cluster queue %s does not exist within the KaiwoQueueConfig object", clusterQueueName)
	} else {
		return fmt.Errorf("failed to get local queue: %w", err)
	}
}
