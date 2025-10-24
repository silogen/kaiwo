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
	"fmt"

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
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: KaiwoQueueConfigName}, kaiwoQueueConfig); err != nil {
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
						logger.Error(err, "Failed to create LocalQueue", "Description", name, "Namespace", namespace)
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
