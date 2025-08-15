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

	"github.com/silogen/kaiwo/pkg/common"

	"github.com/silogen/kaiwo/pkg/config"

	"github.com/silogen/kaiwo/pkg/cluster"

	"k8s.io/apimachinery/pkg/api/meta"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/client-go/tools/record"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	ctx, err := config.GetContextWithConfig(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not get config context: %w", err)
	}

	config := config.ConfigFromContext(ctx)

	//if err := common.EnsureClusterNodesLabelsAndTaints(ctx, r.Client); err != nil {
	//	return ctrl.Result{}, fmt.Errorf("could not ensure cluster nodes' taints and labels: %w", err)
	//}

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
			// stop here and requeue so queueConfig is fetched on the next loop
			return ctrl.Result{Requeue: true}, nil
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
		logger.Error(err, "Failed to sync KaiwoQueueConfig with one or more errors") // TODO how to report the errors?
		meta.SetStatusCondition(&queueConfig.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "SyncFailed", Message: err.Error()})
		queueConfig.Status.Status = kaiwo.QueueConfigStatusFailed
	} else {
		meta.SetStatusCondition(&queueConfig.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "SyncSucceeded", Message: "Kueue sync succeeded"})
		queueConfig.Status.Status = kaiwo.QueueConfigStatusReady
	}

	// **Only Update WorkloadStatus If It Has Changed**
	if previousStatus != queueConfig.Status.Status {
		if err := r.Status().Update(ctx, &queueConfig); err != nil {
			logger.Error(err, "Failed to update KaiwoQueueConfig status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, err
}

func (r *KaiwoQueueConfigReconciler) SyncKueueResources(ctx context.Context, kaiwoQueueConfig *kaiwo.KaiwoQueueConfig) error {
	syncErrors := make([]error, 0)

	// ResourceFlavors
	if err := controllerutils.SyncQueueResource(
		ctx,
		r.Client,
		r.Recorder,
		kaiwoQueueConfig,
		kaiwoQueueConfig.Spec.ResourceFlavors,
		controllerutils.ResourceFlavorConverter{},
		controllerutils.WithOwnerReference(kaiwoQueueConfig, r.Scheme),
	); err != nil {
		syncErrors = append(syncErrors, err)
	}

	// ClusterQueues
	if err := controllerutils.SyncQueueResource(
		ctx,
		r.Client,
		r.Recorder,
		kaiwoQueueConfig,
		kaiwoQueueConfig.Spec.ClusterQueues,
		controllerutils.ClusterQueueConverter{},
		controllerutils.WithOwnerReference(kaiwoQueueConfig, r.Scheme),
	); err != nil {
		syncErrors = append(syncErrors, err)
	}

	// LocalQueues
	existingClusterQueues := &kueuev1beta1.ClusterQueueList{}
	if err := r.List(ctx, existingClusterQueues); err != nil {
		syncErrors = append(syncErrors, err)
	} else {
		actualClusterQueues := map[string]*kueuev1beta1.ClusterQueue{}
		for _, queue := range existingClusterQueues.Items {
			actualClusterQueues[queue.Name] = &queue
		}

		kaiwoClusterQueueReferences := map[string]kaiwo.ClusterQueue{}
		for _, queue := range kaiwoQueueConfig.Spec.ClusterQueues {
			kaiwoClusterQueueReferences[queue.Name] = queue
		}

		var localQueuesToCreate []client.ObjectKey
		for _, expectedClusterQueue := range kaiwoQueueConfig.Spec.ClusterQueues {
			if len(expectedClusterQueue.Namespaces) == 0 {
				// Skip if the cluster queue does not define any namespaces (nothing to automatically create)
				continue
			}
			for _, namespace := range expectedClusterQueue.Namespaces {
				localQueuesToCreate = append(localQueuesToCreate, client.ObjectKey{
					Name:      expectedClusterQueue.Name,
					Namespace: namespace,
				})
			}
		}

		if err := controllerutils.SyncQueueResource(
			ctx,
			r.Client,
			r.Recorder,
			kaiwoQueueConfig,
			localQueuesToCreate,
			controllerutils.LocalQueueConverter{},
			controllerutils.WithDynamicOwnerReference(r.Scheme, func(obj client.Object) client.Object {
				localQueue := obj.(*kueuev1beta1.LocalQueue)
				return actualClusterQueues[localQueue.Name]
			}),
			controllerutils.WithStaleDeleteCheck(func(obj *kueuev1beta1.LocalQueue) bool {
				// Check if the cluster queue which this local queue belongs to lists any namespaces
				// If it doesn't, that means that the cluster queue does not limit namespaces, and users can
				// create local queues outside the KaiwoQueueConfig, so we shouldn't delete them
				kaiwoClusterQueueReference := kaiwoClusterQueueReferences[obj.Name]
				return len(kaiwoClusterQueueReference.Namespaces) > 0
			}),
		); err != nil {
			syncErrors = append(syncErrors, err)
		}
	}

	// WorkloadPriorityClasses
	if err := controllerutils.SyncQueueResource(
		ctx,
		r.Client,
		r.Recorder,
		kaiwoQueueConfig,
		kaiwoQueueConfig.Spec.WorkloadPriorityClasses,
		controllerutils.WorkloadPriorityClassConverter{},
		controllerutils.WithOwnerReference(kaiwoQueueConfig, r.Scheme),
	); err != nil {
		syncErrors = append(syncErrors, err)
	}

	// Topologies
	if err := controllerutils.SyncQueueResource(
		ctx,
		r.Client,
		r.Recorder,
		kaiwoQueueConfig,
		kaiwoQueueConfig.Spec.Topologies,
		controllerutils.TopologyConverter{},
		controllerutils.WithOwnerReference(kaiwoQueueConfig, r.Scheme),
	); err != nil {
		syncErrors = append(syncErrors, err)
	}

	if len(syncErrors) > 0 {
		return utilerrors.NewAggregate(syncErrors)
	}
	return nil
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

	if !equality.Semantic.DeepEqual(oldNode.Labels, newNode.Labels) {
		return true
	}

	return false
}

func (r *KaiwoQueueConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// logger := log.Log.WithName("SetupWithManager")
	r.Recorder = mgr.GetEventRecorderFor("kaiwoqueueconfig-controller")

	// FIX Driven by the node watcher
	//if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
	//	ctx, err := common.GetContextWithConfig(ctx, mgr.GetClient())
	//	if err != nil {
	//		return fmt.Errorf("could not get config: %w", err)
	//	}
	//	logger.Info("Ensuring default KaiwoQueueConfig exists on startup...")
	//	config := common.ConfigFromContext(ctx)
	//
	//	if err := common.EnsureClusterNodesLabelsAndTaints(ctx, r.Client); err != nil {
	//		return fmt.Errorf("could not ensure cluster nodes' taints and labels: %w", err)
	//	}
	//
	//	if err := r.CreateTopology(ctx); err != nil {
	//		logger.Error(err, "Failed to create default topology on startup")
	//	}
	//
	//	if err := r.EnsureKaiwoQueueConfig(ctx, common.KaiwoQueueConfigName, config.DefaultClusterQueueName, config.DefaultClusterQueueCohortName); err != nil {
	//		logger.Error(err, "Failed to ensure default KaiwoQueueConfig on startup")
	//		return err
	//	}
	//	return nil
	//})); err != nil {
	//	return err
	//}

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
		Watches(
			&kaiwo.KaiwoNode{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{Name: common.KaiwoQueueConfigName}},
				}
			}),
		).
		Named("kaiwoqueueconfig").
		Complete(r)
}

func (r *KaiwoQueueConfigReconciler) EnsureKaiwoQueueConfig(ctx context.Context, kaiwoQueueConfigName string, clusterQueueName string, cohort string) error {
	logger := log.FromContext(ctx)

	clusterCtx, err := cluster.GetClusterContext(ctx, r.Client)
	if err != nil {
		return fmt.Errorf("could not get cluster context: %w", err)
	}

	// Generate new flavors and clusterQueue
	newResourceFlavors, nodePoolResources, err := controllerutils.ConstructDefaultResourceFlavors(ctx, *clusterCtx)
	if err != nil {
		logger.Error(err, "Failed to create default resource flavors")
		return err
	}

	newClusterQueue := controllerutils.ConstructClusterQueue(nodePoolResources, clusterQueueName, cohort)

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
