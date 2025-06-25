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
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/client-go/tools/record"

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
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
)

// KaiwoQueueConfigReconciler reconciles a KaiwoQueueConfig object
type KaiwoQueueConfigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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

	ctx, err := common.GetContextWithConfig(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not get config context: %w", err)
	}

	config := common.ConfigFromContext(ctx)

	if config.DynamicallyUpdateDefaultClusterQueue {
		if err := r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName, config.DefaultClusterQueueCohortName); err != nil {
			logger.Error(err, "Failed to create default KaiwoQueueConfig")
			return ctrl.Result{}, err
		}
	}

	// Fetch the requested KaiwoQueueConfig
	var queueConfig kaiwo.KaiwoQueueConfig
	err = r.Get(ctx, req.NamespacedName, &queueConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName, config.DefaultClusterQueueCohortName); err != nil {
				logger.Error(err, "Failed to create default KaiwoQueueConfig")
				return ctrl.Result{}, err
			}
		} else {
			logger.Error(err, "Failed to get KaiwoQueueConfig")
			return ctrl.Result{}, err
		}
	}

	// **Check if WorkloadStatus is already correct before updating**
	previousStatus := queueConfig.Status.Status

	// **Sync Kueue Resources**
	logger.Info("Syncing Kueue resources for KaiwoQueueConfig", "name", queueConfig.Name)
	err = r.SyncKueueResources(ctx, &queueConfig)

	if err != nil {
		logger.Error(err, "Failed to sync Kueue resources for KaiwoQueueConfig")
		queueConfig.Status.Status = kaiwo.QueueConfigStatusFailed
	} else {
		queueConfig.Status.Status = kaiwo.QueueConfigStatusReady
	}

	// **Only Update WorkloadStatus If It Has Changed**
	if previousStatus != queueConfig.Status.Status {
		if err := r.Status().Update(ctx, &queueConfig); err != nil {
			logger.Error(err, "Failed to update KaiwoQueueConfig status")
			return ctrl.Result{}, err
		}
		logger.Info("Updated KaiwoQueueConfig status", "name", queueConfig.Name, "WorkloadStatus", queueConfig.Status.Status)
	}

	return ctrl.Result{}, nil
}

func (r *KaiwoQueueConfigReconciler) emitEvent(queueConfig *kaiwo.KaiwoQueueConfig, target string, action string, obj client.Object, err error) {
	reason := fmt.Sprintf("KaiwoQueueConfig%s%s", common.ToPascalCase(target), common.ToPascalCase(action))

	var key string
	if obj != nil {
		key = fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
	} else {
		key = ""
	}

	eventType := corev1.EventTypeNormal
	var message string
	if err != nil {
		eventType = corev1.EventTypeWarning
		reason += "Failed"
		message = fmt.Sprintf("Failed to %s %s %s: %v", action, target, key, err)
	} else {
		reason += "d"
		message = fmt.Sprintf("%sd %s %s", action, target, key)
	}
	r.Recorder.Eventf(
		queueConfig,
		eventType,
		reason,
		message,
	)
}

func (r *KaiwoQueueConfigReconciler) SyncKueueResources(ctx context.Context, queueConfig *kaiwo.KaiwoQueueConfig) error {
	logger := log.FromContext(ctx)

	existingFlavors := &kueuev1beta1.ResourceFlavorList{}
	existingQueues := &kueuev1beta1.ClusterQueueList{}
	existingLocalQueues := &kueuev1beta1.LocalQueueList{}
	existingPriorityClasses := &kueuev1beta1.WorkloadPriorityClassList{}
	existingTopologies := &kueuev1alpha1.TopologyList{}

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
	if err := r.List(ctx, existingTopologies); err != nil {
		logger.Error(err, "Failed to list Topologies")
		return err
	}

	success := r.syncResourceFlavors(ctx, queueConfig, existingFlavors)
	success = r.syncClusterQueues(ctx, queueConfig, existingQueues) && success
	success = r.syncLocalQueues(ctx, queueConfig, existingLocalQueues) && success
	success = r.syncWorkloadPriorityClasses(ctx, queueConfig, existingPriorityClasses) && success
	success = r.syncTopologies(ctx, queueConfig, existingTopologies) && success

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
				r.emitEvent(queueConfig, "resource flavor", "owner reference", &kueueFlavor, err)
				continue
			}
			err := r.Create(ctx, &kueueFlavor)
			if err != nil {
				logger.Error(err, "Failed to create ResourceFlavor", "name", kueueFlavor.Name)
				success = false
			}
			r.emitEvent(queueConfig, "resource flavor", "create", &kueueFlavor, err)

		} else if !controllerutils.CompareResourceFlavors(existingFlavor, kueueFlavor) {
			logger.Info("Updating ResourceFlavor", "name", kueueFlavor.Name)
			existingFlavor.Spec = kueueFlavor.Spec
			err := r.Update(ctx, &existingFlavor)
			if err != nil {
				logger.Error(err, "Failed to update ResourceFlavor", "name", kueueFlavor.Name)
				success = false
			}
			r.emitEvent(queueConfig, "resource flavor", "update", &existingFlavor, err)

		}
		existingFlavorMap[kueueFlavor.Name] = kueueFlavor
	}

	for _, existingFlavor := range existingFlavors.Items {
		if _, exists := existingFlavorMap[existingFlavor.Name]; !exists {
			logger.Info("Deleting ResourceFlavor", "name", existingFlavor.Name)
			err := r.Delete(ctx, &existingFlavor)
			if err != nil {
				logger.Error(err, "Failed to delete ResourceFlavor", "name", existingFlavor.Name)
				success = false
			}
			r.emitEvent(queueConfig, "resource flavor", "delete", &existingFlavor, err)
		}
	}

	return success
}

func (r *KaiwoQueueConfigReconciler) syncTopologies(
	ctx context.Context,
	queueConfig *kaiwo.KaiwoQueueConfig,
	existingTopologies *kueuev1alpha1.TopologyList,
) bool {
	logger := log.FromContext(ctx)

	success := true
	expectedTopologies := controllerutils.ConvertKaiwoToKueueTopologies(queueConfig.Spec.Topologies)
	existingTopologyMap := make(map[string]kueuev1alpha1.Topology)

	for _, kueueTopology := range expectedTopologies {
		existingTopology, found := controllerutils.FindTopology(existingTopologies.Items, kueueTopology.Name)
		if !found {
			logger.Info("Creating Topology", "name", kueueTopology.Name)
			if err := ctrl.SetControllerReference(queueConfig, &kueueTopology, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference", "name", kueueTopology.Name)
				success = false
				r.emitEvent(queueConfig, "topology", "owner reference", &kueueTopology, err)
				continue
			}

			err := r.Create(ctx, &kueueTopology)
			if err != nil {
				logger.Error(err, "Failed to create Topology", "name", kueueTopology.Name)
				success = false
			}
			r.emitEvent(queueConfig, "topology", "create", &kueueTopology, err)
		} else if !controllerutils.CompareTopologies(existingTopology, kueueTopology) {
			logger.Info("Updating Topology", "name", kueueTopology.Name)
			existingTopology.Spec = kueueTopology.Spec

			err := r.Update(ctx, &existingTopology)
			if err != nil {
				logger.Error(err, "Failed to update Topology", "name", kueueTopology.Name)
				success = false
			}
			r.emitEvent(queueConfig, "topology", "update", &existingTopology, err)
		}
		existingTopologyMap[kueueTopology.Name] = kueueTopology
	}

	for _, existingTopology := range existingTopologies.Items {
		if _, exists := existingTopologyMap[existingTopology.Name]; !exists {
			logger.Info("Deleting Topology", "name", existingTopology.Name)
			err := r.Delete(ctx, &existingTopology)
			if err != nil {
				logger.Error(err, "Failed to delete Topology", "name", existingTopology.Name)
				success = false
			}
			r.emitEvent(queueConfig, "topology", "delete", &existingTopology, err)
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
				r.emitEvent(queueConfig, "cluster queue", "owner reference", &kueueQueue, err)
				continue
			}
			err := r.Create(ctx, &kueueQueue)
			if err != nil {
				logger.Error(err, "Failed to create ClusterQueue", "name", kueueQueue.Name)
				success = false
			}
			r.emitEvent(queueConfig, "cluster queue", "create", &kueueQueue, err)

		} else if err != nil {
			logger.Error(err, "Failed to get ClusterQueue", "name", kueueQueue.Name)
			r.emitEvent(queueConfig, "cluster queue", "get", &kueueQueue, err)
			success = false
		} else if !controllerutils.CompareClusterQueues(*existingQueue, kueueQueue) {
			logger.Info("Updating ClusterQueue", "name", kueueQueue.Name)
			existingQueue.Spec = kueueQueue.Spec
			err := r.Update(ctx, existingQueue)
			if err != nil {
				logger.Error(err, "Failed to update ClusterQueue", "name", kueueQueue.Name)
				success = false
			}
			r.emitEvent(queueConfig, "cluster queue", "update", existingQueue, err)
		}
		expectedQueues[kueueQueue.Name] = kueueQueue
	}

	for _, existingQueue := range existingQueues.Items {
		if _, exists := expectedQueues[existingQueue.Name]; !exists {
			logger.Info("Deleting ClusterQueue", "name", existingQueue.Name)
			err := r.Delete(ctx, &existingQueue)
			if err != nil {
				logger.Error(err, "Failed to delete ClusterQueue", "name", existingQueue.Name)
				success = false
			}
			r.emitEvent(queueConfig, "cluster queue", "delete", &existingQueue, err)
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
			r.emitEvent(kaiwoQueueConfig, "cluster queue", "get", actualClusterQueue, err)
			continue
		}

		// If the ClusterQueue doesn't have any namespaces listed, ensure any existing LocalQueue for this ClusterQueue is not removed
		if len(clusterQueue.Namespaces) == 0 {
			if namespaceMap, ok := staleQueues[clusterQueue.Name]; ok {
				if len(namespaceMap) == 0 {
					delete(staleQueues, clusterQueue.Name)
				}
			}
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
					r.emitEvent(kaiwoQueueConfig, "local queue", "owner reference", localQueue, err)
					continue
				}
				err := r.Create(ctx, localQueue)
				if err != nil {
					logger.Error(err, "Failed to create LocalQueue", "name", clusterQueue.Name, "namespace", namespace)
					success = false
				} else {
					logger.Info("Created LocalQueue", "name", clusterQueue.Name, "namespace", namespace)
				}
				r.emitEvent(kaiwoQueueConfig, "local queue", "create", localQueue, err)

			default:
				// Unexpected error
				logger.Error(err, "Failed to get LocalQueue", "name", clusterQueue.Name, "namespace", namespace)
				r.emitEvent(kaiwoQueueConfig, "local queue", "get", nil, err)
				success = false
			}
		}
	}

	// Clean up stale LocalQueues
	for queueName, namespaceMap := range staleQueues {
		for namespace, localQueue := range namespaceMap {
			logger.Info("Deleting stale LocalQueue", "name", queueName, "namespace", namespace)
			err := r.Delete(ctx, &localQueue)
			if err != nil {
				logger.Error(err, "Failed to delete stale LocalQueue", "name", queueName, "namespace", namespace)
				success = false
			}
			r.emitEvent(kaiwoQueueConfig, "local queue", "delete", &localQueue, err)
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
				r.emitEvent(queueConfig, "workload priority class", "owner reference", &priorityClassSpec, err)
				continue
			}
			err := r.Create(ctx, &priorityClassSpec)
			if err != nil {
				logger.Error(err, "Failed to create WorkloadPriorityClass", "name", priorityClassSpec.Name)
				success = false
			}
			r.emitEvent(queueConfig, "workload priority class", "create", &priorityClassSpec, err)
		} else if err != nil {
			logger.Error(err, "Failed to get WorkloadPriorityClass", "name", priorityClassSpec.Name)
			success = false
			r.emitEvent(queueConfig, "workload priority class", "get", &priorityClassSpec, err)
		} else if !controllerutils.ComparePriorityClasses(*existingPriorityClass, priorityClassSpec) {
			logger.Info("Updating WorkloadPriorityClass", "name", priorityClassSpec.Name)
			existingPriorityClass.Value = priorityClassSpec.Value
			err := r.Update(ctx, existingPriorityClass)
			if err != nil {
				logger.Error(err, "Failed to update WorkloadPriorityClass", "name", priorityClassSpec.Name)
				success = false
			}
			r.emitEvent(queueConfig, "workload priority class", "update", existingPriorityClass, err)
		}
		expectedPriorityClasses[priorityClassSpec.Name] = priorityClassSpec
	}

	for _, existingPriorityClass := range existingPriorityClasses.Items {
		if _, exists := expectedPriorityClasses[existingPriorityClass.Name]; !exists {
			logger.Info("Deleting WorkloadPriorityClass", "name", existingPriorityClass.Name)
			err := r.Delete(ctx, &existingPriorityClass)
			if err != nil {
				logger.Error(err, "Failed to delete WorkloadPriorityClass", "name", existingPriorityClass.Name)
				success = false
			}
			r.emitEvent(queueConfig, "workload priority class", "delete", &existingPriorityClass, err)
		}
	}

	return success
}

// nodeResourceChanged returns true if any of the fields that we watch changed
func nodeResourceChanged(oldNode, newNode *corev1.Node) bool {
	if oldNode.Spec.Unschedulable != newNode.Spec.Unschedulable {
		return true
	}

	cpu := newNode.Status.Capacity.Cpu()
	if oldNode.Status.Capacity.Cpu().Cmp(*cpu) != 0 {
		return true
	}

	memory := newNode.Status.Capacity.Memory()
	if oldNode.Status.Capacity.Memory().Cmp(*memory) != 0 {
		return true
	}

	if !reflect.DeepEqual(oldNode.Labels, newNode.Labels) {
		return true
	}

	return false
}

func (r *KaiwoQueueConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := log.Log.WithName("SetupWithManager")
	r.Recorder = mgr.GetEventRecorderFor("kaiwoqueueconfig-controller")

	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		ctx, err := common.GetContextWithConfig(ctx, mgr.GetClient())
		if err != nil {
			return fmt.Errorf("could not get config: %w", err)
		}
		logger.Info("Ensuring default KaiwoQueueConfig exists on startup...")
		config := common.ConfigFromContext(ctx)
		if err := r.CreateTopology(ctx); err != nil {
			logger.Error(err, "Failed to create default topology on startup")
		}
		if err = r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName, config.DefaultClusterQueueCohortName); err != nil {
			logger.Error(err, "Failed to ensure default KaiwoQueueConfig on startup")
			return err
		}
		return nil
	})); err != nil {
		return err
	}

	// Create a predicate to filter when to run the reconciliation on node changes
	nodePred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*corev1.Node)
			newNode := e.ObjectNew.(*corev1.Node)
			return nodeResourceChanged(oldNode, newNode)
		},
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
			builder.WithPredicates(nodePred),
		).
		Named("kaiwoqueueconfig").
		Complete(r)
}

func (r *KaiwoQueueConfigReconciler) CreateTopology(ctx context.Context) error {
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	for i := range nodes.Items {
		node := &nodes.Items[i]
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}

		updated := false
		if _, exists := node.Labels[common.DefaultTopologyBlockLabel]; !exists {
			node.Labels[common.DefaultTopologyBlockLabel] = "block-a"
			updated = true
		}
		if _, exists := node.Labels[common.DefaultTopologyRackLabel]; !exists {
			node.Labels[common.DefaultTopologyRackLabel] = "rack-a"
			updated = true
		}

		if updated {
			if err := r.Update(ctx, node); err != nil {
				return fmt.Errorf("failed to label node %s: %w", node.Name, err)
			}
		}
	}

	return nil
}

func (r *KaiwoQueueConfigReconciler) EnsureKaiwoQueueConfig(ctx context.Context, kaiwoQueueConfigName string, clusterQueueName string, cohort string) error {
	logger := log.FromContext(ctx)

	// Generate new flavors and clusterQueue
	newResourceFlavors, nodePoolResources, err := controllerutils.CreateDefaultResourceFlavors(ctx, r.Client)
	if err != nil {
		logger.Error(err, "Failed to create default resource flavors")
		return err
	}

	newClusterQueue := controllerutils.CreateClusterQueue(nodePoolResources, clusterQueueName, cohort)

	newTopologies, err := controllerutils.CreateDefaultTopology(ctx, r.Client)
	if err != nil {
		logger.Error(err, "Failed to create default topology")
		return err
	}

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
				Topologies:      newTopologies,
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

	topologyMap := make(map[string]kaiwo.Topology)
	for _, t := range existingConfig.Spec.Topologies {
		topologyMap[t.Name] = t
	}
	for _, t := range newTopologies {
		topologyMap[t.Name] = t
	}
	mergedTopologies := make([]kaiwo.Topology, 0, len(topologyMap))
	for _, t := range topologyMap {
		mergedTopologies = append(mergedTopologies, t)
	}

	existingConfig.Spec.ResourceFlavors = mergedFlavors
	existingConfig.Spec.ClusterQueues = mergedQueues
	existingConfig.Spec.Topologies = mergedTopologies

	if err := r.Update(ctx, &existingConfig); err != nil {
		logger.Error(err, "Failed to update KaiwoQueueConfig with merged values")
		return err
	}

	logger.Info("Successfully updated KaiwoQueueConfig", "name", existingConfig.Name)
	return nil
}
