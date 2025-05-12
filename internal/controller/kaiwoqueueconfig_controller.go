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

package controller

import (
	"context"
	"fmt"
	"time"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/workloads/common"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

// KaiwoQueueConfigReconciler reconciles a KaiwoQueueConfig object
type KaiwoQueueConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoqueueconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoqueueconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoqueueconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=config.kaiwo.silogen.ai,resources=kaiwoconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *KaiwoQueueConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if req.Name != "kaiwo" {
		logger.Error(nil, "Invalid KaiwoQueueConfig name", "name", req.Name)
		return ctrl.Result{}, fmt.Errorf("only a KaiwoQueueConfig named 'kaiwo' is allowed")
	}

	ctx, err := controllerutils.GetContextWithConfig(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not get config context: %w", err)
	}

	config := controllerutils.ConfigFromContext(ctx)

	if config.DynamicallyUpdateDefaultClusterQueue {
		if err := r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName); err != nil {
			logger.Error(err, "Failed to create default KaiwoQueueConfig")
			return ctrl.Result{}, err
		}
	}

	// Fetch the requested KaiwoQueueConfig
	var queueConfig kaiwo.KaiwoQueueConfig
	err = r.Get(ctx, req.NamespacedName, &queueConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName); err != nil {
				logger.Error(err, "Failed to create default KaiwoQueueConfig")
				return ctrl.Result{}, err
			}
		}
		logger.Error(err, "Failed to get KaiwoQueueConfig")
		return ctrl.Result{}, err
	}

	// **Check if Status is already correct before updating**
	previousStatus := queueConfig.Status.Status
	if previousStatus == "" {
		queueConfig.Status.Status = kaiwo.StatusPending
	}

	// **Sync Kueue Resources**
	logger.Info("Syncing Kueue resources for KaiwoQueueConfig", "name", queueConfig.Name)
	err = r.SyncKueueResources(ctx, &queueConfig)

	if err != nil {
		logger.Error(err, "Failed to sync Kueue resources for KaiwoQueueConfig")
		queueConfig.Status.Status = kaiwo.StatusFailed
	} else {
		queueConfig.Status.Status = kaiwo.StatusReady
	}

	// **Only Update Status If It Has Changed**
	if previousStatus != queueConfig.Status.Status {
		if err := r.Status().Update(ctx, &queueConfig); err != nil {
			logger.Error(err, "Failed to update KaiwoQueueConfig status")
			return ctrl.Result{}, err
		}
		logger.Info("Updated KaiwoQueueConfig status", "name", queueConfig.Name, "Status", queueConfig.Status.Status)
	}

	// Requeue only if status is still pending
	if queueConfig.Status.Status == kaiwo.StatusPending {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *KaiwoQueueConfigReconciler) SyncKueueResources(ctx context.Context, queueConfig *kaiwo.KaiwoQueueConfig) error {
	logger := log.FromContext(ctx)

	existingFlavors := &kueuev1beta1.ResourceFlavorList{}
	existingQueues := &kueuev1beta1.ClusterQueueList{}
	existingLocalQueues := &kueuev1beta1.LocalQueueList{}
	existingPriorityClasses := &kueuev1beta1.WorkloadPriorityClassList{}

	if err := r.List(ctx, existingFlavors); err != nil {
		logger.Error(err, "Failed to list ResourceFlavors")
		return err
	}
	if err := r.List(ctx, existingQueues); err != nil {
		logger.Error(err, "Failed to list ClusterQueues")
		return err
	}
	if err := r.List(ctx, existingPriorityClasses); err != nil {
		logger.Error(err, "Failed to list WorkloadPriorityClasses")
		return err
	}
	if err := r.List(ctx, existingLocalQueues); err != nil {
		logger.Error(err, "Failed to list LocalQueues")
		return err
	}

	success := r.syncResourceFlavors(ctx, queueConfig, existingFlavors)
	success = r.syncClusterQueues(ctx, queueConfig, existingQueues) && success
	success = r.syncLocalQueues(ctx, queueConfig, existingLocalQueues) && success
	success = r.syncWorkloadPriorityClasses(ctx, queueConfig, existingPriorityClasses) && success

	if success {
		logger.Info("Successfully synced all Kueue resources")
		return nil
	}
	return fmt.Errorf("failed to sync some Kueue resources")
}

func (r *KaiwoQueueConfigReconciler) syncResourceFlavors(ctx context.Context, queueConfig *kaiwo.KaiwoQueueConfig, existingFlavors *kueuev1beta1.ResourceFlavorList) bool {
	logger := log.FromContext(ctx)

	success := true
	expectedFlavors := controllerutils.ConvertKaiwoToKueueResourceFlavors(queueConfig.Spec.ResourceFlavors)
	existingFlavorMap := make(map[string]kueuev1beta1.ResourceFlavor)

	for _, kueueFlavor := range expectedFlavors {
		existingFlavor, found := controllerutils.FindFlavor(existingFlavors.Items, kueueFlavor.Name)
		if !found {
			logger.Info("Creating ResourceFlavor", "name", kueueFlavor.Name)
			if err := ctrl.SetControllerReference(queueConfig, &kueueFlavor, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference", "name", kueueFlavor.Name)
				success = false
				continue
			}
			if err := r.Create(ctx, &kueueFlavor); err != nil {
				logger.Error(err, "Failed to create ResourceFlavor", "name", kueueFlavor.Name)
				success = false
			}
		} else if !controllerutils.CompareResourceFlavors(existingFlavor, kueueFlavor) {
			logger.Info("Updating ResourceFlavor", "name", kueueFlavor.Name)
			existingFlavor.Spec = kueueFlavor.Spec
			if err := r.Update(ctx, &existingFlavor); err != nil {
				logger.Error(err, "Failed to update ResourceFlavor", "name", kueueFlavor.Name)
				success = false
			}
		}
		existingFlavorMap[kueueFlavor.Name] = kueueFlavor
	}

	for _, existingFlavor := range existingFlavors.Items {
		if _, exists := existingFlavorMap[existingFlavor.Name]; !exists {
			logger.Info("Deleting ResourceFlavor", "name", existingFlavor.Name)
			if err := r.Delete(ctx, &existingFlavor); err != nil {
				logger.Error(err, "Failed to delete ResourceFlavor", "name", existingFlavor.Name)
				success = false
			}
		}
	}

	return success
}

func (r *KaiwoQueueConfigReconciler) syncClusterQueues(ctx context.Context, queueConfig *kaiwo.KaiwoQueueConfig, existingQueues *kueuev1beta1.ClusterQueueList) bool {
	logger := log.FromContext(ctx)

	success := true
	expectedQueues := make(map[string]kueuev1beta1.ClusterQueue)

	for _, kaiwoQueue := range queueConfig.Spec.ClusterQueues {
		kueueQueue := controllerutils.ConvertKaiwoToKueueClusterQueue(kaiwoQueue)
		for i, resourceGroup := range kueueQueue.Spec.ResourceGroups {
			for j, flavor := range resourceGroup.Flavors {
				resourceMap := make(map[string]kueuev1beta1.ResourceQuota)
				for _, flavorResource := range flavor.Resources {
					resourceMap[flavorResource.Name.String()] = flavorResource
				}
				var resources []kueuev1beta1.ResourceQuota
				for _, resourceName := range resourceGroup.CoveredResources {
					if resource, exists := resourceMap[resourceName.String()]; exists {
						resources = append(resources, resource)
					}
				}
				kueueQueue.Spec.ResourceGroups[i].Flavors[j].Resources = resources
			}
		}
		existingQueue := &kueuev1beta1.ClusterQueue{}
		err := r.Get(ctx, client.ObjectKey{Name: kueueQueue.Name}, existingQueue)

		if errors.IsNotFound(err) {
			logger.Info("Creating ClusterQueue", "name", kueueQueue.Name)
			if err := ctrl.SetControllerReference(queueConfig, &kueueQueue, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference", "name", kueueQueue.Name)
				success = false
				continue
			}
			if err := r.Create(ctx, &kueueQueue); err != nil {
				logger.Error(err, "Failed to create ClusterQueue", "name", kueueQueue.Name)
				success = false
			}
		} else if err != nil {
			logger.Error(err, "Failed to get ClusterQueue", "name", kueueQueue.Name)
			success = false
		} else if !controllerutils.CompareClusterQueues(*existingQueue, kueueQueue) {
			logger.Info("Updating ClusterQueue", "name", kueueQueue.Name)
			existingQueue.Spec = kueueQueue.Spec
			if err := r.Update(ctx, existingQueue); err != nil {
				logger.Error(err, "Failed to update ClusterQueue", "name", kueueQueue.Name)
				success = false
			}
		}
		expectedQueues[kueueQueue.Name] = kueueQueue
	}

	for _, existingQueue := range existingQueues.Items {
		if _, exists := expectedQueues[existingQueue.Name]; !exists {
			logger.Info("Deleting ClusterQueue", "name", existingQueue.Name)
			if err := r.Delete(ctx, &existingQueue); err != nil {
				logger.Error(err, "Failed to delete ClusterQueue", "name", existingQueue.Name)
				success = false
			}
		}
	}

	return success
}

// syncLocalQueues manages a set of LocalQueues based on the defined Cluster Queues and the namespaces that they apply to.
// This function ensures that each namespace has its corresponding LocalQueue, and if the namespace (or ClusterQueue) is removed,
// the corresponding LocalQueue is also deleted.
func (r *KaiwoQueueConfigReconciler) syncLocalQueues(
	ctx context.Context,
	kaiwoQueueConfig *kaiwo.KaiwoQueueConfig,
	existingLocalQueues *kueuev1beta1.LocalQueueList,
) bool {
	logger := log.FromContext(ctx)
	success := true

	// Map: clusterQueueName -> namespace -> LocalQueue
	staleQueues := map[string]map[string]kueuev1beta1.LocalQueue{}

	// Build map of all existing LocalQueues
	for _, localQueue := range existingLocalQueues.Items {
		if _, ok := staleQueues[localQueue.Name]; !ok {
			staleQueues[localQueue.Name] = map[string]kueuev1beta1.LocalQueue{}
		}
		staleQueues[localQueue.Name][localQueue.Namespace] = localQueue
	}

	// Reconcile expected LocalQueues
	for _, clusterQueue := range kaiwoQueueConfig.Spec.ClusterQueues {
		// Fetch the actual cluster queue to use for the owner reference
		actualClusterQueue := &kueuev1beta1.ClusterQueue{}
		if err := r.Get(ctx, client.ObjectKey{Name: clusterQueue.Name}, actualClusterQueue); err != nil {
			logger.Error(err, "Failed to get ClusterQueue", "name", clusterQueue.Name)
			success = false
			continue
		}
		for _, namespace := range clusterQueue.Namespaces {
			localQueue := &kueuev1beta1.LocalQueue{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterQueue.Name,
					Namespace: namespace,
				},
				Spec: kueuev1beta1.LocalQueueSpec{
					ClusterQueue: kueuev1beta1.ClusterQueueReference(clusterQueue.Name),
				},
			}

			key := client.ObjectKeyFromObject(localQueue)
			existing := &kueuev1beta1.LocalQueue{}
			err := r.Get(ctx, key, existing)

			switch {
			case err == nil:
				// Already exists — remove it from the stale map
				if namespaceMap, ok := staleQueues[clusterQueue.Name]; ok {
					delete(namespaceMap, namespace)
					if len(namespaceMap) == 0 {
						delete(staleQueues, clusterQueue.Name)
					}
				}

			case errors.IsNotFound(err):
				// Needs to be created
				if err := ctrl.SetControllerReference(actualClusterQueue, localQueue, r.Scheme); err != nil {
					logger.Error(err, "Failed to set owner reference", "name", clusterQueue.Name, "namespace", namespace)
					success = false
					continue
				}
				if err := r.Create(ctx, localQueue); err != nil {
					logger.Error(err, "Failed to create LocalQueue", "name", clusterQueue.Name, "namespace", namespace)
					success = false
				} else {
					logger.Info("Created LocalQueue", "name", clusterQueue.Name, "namespace", namespace)
				}

			default:
				// Unexpected error
				logger.Error(err, "Failed to get LocalQueue", "name", clusterQueue.Name, "namespace", namespace)
				success = false
			}
		}
	}

	// Clean up stale LocalQueues
	for queueName, namespaceMap := range staleQueues {
		for namespace, localQueue := range namespaceMap {
			logger.Info("Deleting stale LocalQueue", "name", queueName, "namespace", namespace)
			if err := r.Delete(ctx, &localQueue); err != nil {
				logger.Error(err, "Failed to delete stale LocalQueue", "name", queueName, "namespace", namespace)
				success = false
			}
		}
	}

	return success
}

func (r *KaiwoQueueConfigReconciler) syncWorkloadPriorityClasses(ctx context.Context, queueConfig *kaiwo.KaiwoQueueConfig, existingPriorityClasses *kueuev1beta1.WorkloadPriorityClassList) bool {
	logger := log.FromContext(ctx)

	success := true
	expectedPriorityClasses := make(map[string]kueuev1beta1.WorkloadPriorityClass)

	for _, priorityClassSpec := range queueConfig.Spec.WorkloadPriorityClasses {
		existingPriorityClass := &kueuev1beta1.WorkloadPriorityClass{}
		err := r.Get(ctx, client.ObjectKey{Name: priorityClassSpec.Name}, existingPriorityClass)

		if errors.IsNotFound(err) {
			logger.Info("Creating WorkloadPriorityClass", "name", priorityClassSpec.Name)
			if err := ctrl.SetControllerReference(queueConfig, &priorityClassSpec, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference", "name", priorityClassSpec.Name)
				success = false
				continue
			}
			if err := r.Create(ctx, &priorityClassSpec); err != nil {
				logger.Error(err, "Failed to create WorkloadPriorityClass", "name", priorityClassSpec.Name)
				success = false
			}
		} else if err != nil {
			logger.Error(err, "Failed to get WorkloadPriorityClass", "name", priorityClassSpec.Name)
			success = false
		} else if !controllerutils.ComparePriorityClasses(*existingPriorityClass, priorityClassSpec) {
			logger.Info("Updating WorkloadPriorityClass", "name", priorityClassSpec.Name)
			existingPriorityClass.Value = priorityClassSpec.Value
			if err := r.Update(ctx, existingPriorityClass); err != nil {
				logger.Error(err, "Failed to update WorkloadPriorityClass", "name", priorityClassSpec.Name)
				success = false
			}
		}
		expectedPriorityClasses[priorityClassSpec.Name] = priorityClassSpec
	}

	for _, existingPriorityClass := range existingPriorityClasses.Items {
		if _, exists := expectedPriorityClasses[existingPriorityClass.Name]; !exists {
			logger.Info("Deleting WorkloadPriorityClass", "name", existingPriorityClass.Name)
			if err := r.Delete(ctx, &existingPriorityClass); err != nil {
				logger.Error(err, "Failed to delete WorkloadPriorityClass", "name", existingPriorityClass.Name)
				success = false
			}
		}
	}

	return success
}

func (r *KaiwoQueueConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := log.Log.WithName("SetupWithManager")

	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		ctx, err := controllerutils.GetContextWithConfig(ctx, mgr.GetClient())
		if err != nil {
			return fmt.Errorf("could not get config: %w", err)
		}
		logger.Info("Ensuring default KaiwoQueueConfig exists on startup...")
		config := controllerutils.ConfigFromContext(ctx)
		if err := r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName); err != nil {
			logger.Error(err, "Failed to ensure default KaiwoQueueConfig on startup")
			return err
		}
		return nil
	})); err != nil {
		return err
	}

	return builder.ControllerManagedBy(mgr).
		For(&kaiwo.KaiwoQueueConfig{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{Name: common.KaiwoQueueConfigName}},
				}
			}),
		).
		Named("kaiwoqueueconfig").
		Complete(r)
}

func (r *KaiwoQueueConfigReconciler) EnsureKaiwoQueueConfig(ctx context.Context, kaiwoQueueConfigName string, clusterQueueName string) error {
	logger := log.FromContext(ctx)

	// Generate new flavors and clusterQueue
	newResourceFlavors, nodePoolResources, err := controllerutils.CreateDefaultResourceFlavors(ctx, r.Client)
	if err != nil {
		logger.Error(err, "Failed to create default resource flavors")
		return err
	}

	newClusterQueue := controllerutils.CreateClusterQueue(nodePoolResources, clusterQueueName)

	var existingConfig kaiwo.KaiwoQueueConfig
	err = r.Get(ctx, client.ObjectKey{Name: kaiwoQueueConfigName}, &existingConfig)
	notFound := errors.IsNotFound(err)

	if notFound {
		logger.Info("KaiwoQueueConfig does not exist, creating from scratch")
		newConfig := kaiwo.KaiwoQueueConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: kaiwoQueueConfigName,
			},
			Spec: kaiwo.KaiwoQueueConfigSpec{
				ClusterQueues:   []kaiwo.ClusterQueue{newClusterQueue},
				ResourceFlavors: newResourceFlavors,
			},
		}

		if err := r.Create(ctx, &newConfig); err != nil {
			logger.Error(err, "Failed to create KaiwoQueueConfig")
			return err
		}

		logger.Info("Successfully created KaiwoQueueConfig", "name", newConfig.Name)
		return nil
	} else if err != nil {
		logger.Error(err, "Failed to fetch existing KaiwoQueueConfig")
		return err
	}

	logger.Info("Merging with existing KaiwoQueueConfig")

	flavorMap := make(map[string]kaiwo.ResourceFlavorSpec)
	for _, rf := range existingConfig.Spec.ResourceFlavors {
		flavorMap[rf.Name] = rf
	}
	for _, rf := range newResourceFlavors {
		flavorMap[rf.Name] = rf
	}
	mergedFlavors := make([]kaiwo.ResourceFlavorSpec, 0, len(flavorMap))
	for _, rf := range flavorMap {
		mergedFlavors = append(mergedFlavors, rf)
	}

	queueMap := make(map[string]kaiwo.ClusterQueue)
	for _, cq := range existingConfig.Spec.ClusterQueues {
		queueMap[cq.Name] = cq
	}
	queueMap[newClusterQueue.Name] = newClusterQueue
	mergedQueues := make([]kaiwo.ClusterQueue, 0, len(queueMap))
	for _, cq := range queueMap {
		mergedQueues = append(mergedQueues, cq)
	}

	existingConfig.Spec.ResourceFlavors = mergedFlavors
	existingConfig.Spec.ClusterQueues = mergedQueues

	if err := r.Update(ctx, &existingConfig); err != nil {
		logger.Error(err, "Failed to update KaiwoQueueConfig with merged values")
		return err
	}

	logger.Info("Successfully updated KaiwoQueueConfig", "name", existingConfig.Name)
	return nil
}
