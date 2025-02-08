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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

// KaiwoQueueConfigReconciler reconciles a KaiwoQueueConfig object
type KaiwoQueueConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoqueueconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoqueueconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=kaiwoqueueconfigs/finalizers,verbs=update

func (r *KaiwoQueueConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if the default KaiwoQueueConfig exists
	var queueConfig kaiwov1alpha1.KaiwoQueueConfig
	err := r.Get(ctx, client.ObjectKey{Name: "kaiwo"}, &queueConfig)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Default KaiwoQueueConfig does not exist, creating it...")
		err = r.CreateDefaultKaiwoQueueConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to create default KaiwoQueueConfig")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Failed to check for KaiwoQueueConfig existence")
		return ctrl.Result{}, err
	}

	// Handle user-defined KaiwoQueueConfig creation
	logger.Info("Handling KaiwoQueueConfig update", "name", queueConfig.Name)
	err = r.SyncKueueResources(ctx, &queueConfig)
	if err != nil {
		logger.Error(err, "Failed to sync Kueue resources for KaiwoQueueConfig")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KaiwoQueueConfigReconciler) CreateDefaultKaiwoQueueConfig(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Generate resource flavors and nodepool quotas
	resourceFlavors, nodePoolResources, err := CreateDefaultResourceFlavors(ctx, r.Client)
	if err != nil {
		logger.Error(err, "Failed to create default resource flavors")
		return err
	}

	// Convert Kaiwo `ResourceFlavorSpec` to Kueue `ResourceFlavor`
	var kueueResourceFlavors []kueuev1beta1.ResourceFlavor
	for _, rf := range resourceFlavors {
		kueueResourceFlavors = append(kueueResourceFlavors, kueuev1beta1.ResourceFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: rf.Name,
			},
			Spec: kueuev1beta1.ResourceFlavorSpec{
				NodeLabels: rf.Spec.NodeLabels,
			},
		})
	}

	// Create the default ClusterQueue using nodePoolResources instead of passing incorrect data
	clusterQueue := CreateClusterQueue(nodePoolResources)

	// Define the default KaiwoQueueConfig (Cluster-scoped)
	defaultQueueConfig := kaiwov1alpha1.KaiwoQueueConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kaiwo",
		},
		Spec: kaiwov1alpha1.KaiwoQueueConfigSpec{
			ClusterQueues:   []kueuev1beta1.ClusterQueue{clusterQueue},
			ResourceFlavors: kueueResourceFlavors,
		},
	}

	// Create the default KaiwoQueueConfig
	if err := r.Create(ctx, &defaultQueueConfig); err != nil {
		logger.Error(err, "Failed to create default KaiwoQueueConfig")
		return err
	}

	logger.Info("Successfully created default KaiwoQueueConfig", "name", defaultQueueConfig.Name)
	return nil
}

func (r *KaiwoQueueConfigReconciler) SyncKueueResources(ctx context.Context, queueConfig *kaiwov1alpha1.KaiwoQueueConfig) error {
	logger := log.FromContext(ctx)

	// Sync ResourceFlavors from Kueue
	for _, flavorSpec := range queueConfig.Spec.ResourceFlavors {
		resourceFlavor := &kueuev1beta1.ResourceFlavor{}
		err := r.Get(ctx, client.ObjectKey{Name: flavorSpec.Name}, resourceFlavor)

		if err != nil && errors.IsNotFound(err) {
			logger.Info("ResourceFlavor does not exist, skipping ownership claim", "name", flavorSpec.Name)
			continue
		} else if err != nil {
			logger.Error(err, "Failed to get ResourceFlavor", "name", flavorSpec.Name)
			return err
		}

		// Ensure ownership
		if err := ctrl.SetControllerReference(queueConfig, resourceFlavor, r.Scheme); err != nil {
			logger.Error(err, "Failed to set ownership for ResourceFlavor", "name", flavorSpec.Name)
			return err
		}

		logger.Info("Kaiwo now owns ResourceFlavor", "name", flavorSpec.Name)
	}

	// Sync ClusterQueues from Kueue
	for _, clusterQueueSpec := range queueConfig.Spec.ClusterQueues {
		clusterQueue := &kueuev1beta1.ClusterQueue{}
		err := r.Get(ctx, client.ObjectKey{Name: clusterQueueSpec.Name}, clusterQueue)

		if err != nil && errors.IsNotFound(err) {
			logger.Info("ClusterQueue does not exist, skipping ownership claim", "name", clusterQueueSpec.Name)
			continue
		} else if err != nil {
			logger.Error(err, "Failed to get ClusterQueue", "name", clusterQueueSpec.Name)
			return err
		}

		// Ensure ownership
		if err := ctrl.SetControllerReference(queueConfig, clusterQueue, r.Scheme); err != nil {
			logger.Error(err, "Failed to set ownership for ClusterQueue", "name", clusterQueueSpec.Name)
			return err
		}

		logger.Info("Kaiwo now owns ClusterQueue", "name", clusterQueueSpec.Name)
	}

	// Sync WorkloadPriorityClasses (assuming they are Kueue objects)
	for _, priorityClassSpec := range queueConfig.Spec.WorkloadPriorityClasses {
		priorityClass := &kueuev1beta1.WorkloadPriorityClass{}
		err := r.Get(ctx, client.ObjectKey{Name: priorityClassSpec.Name}, priorityClass)

		if err != nil && errors.IsNotFound(err) {
			logger.Info("WorkloadPriorityClass does not exist, skipping ownership claim", "name", priorityClassSpec.Name)
			continue
		} else if err != nil {
			logger.Error(err, "Failed to get WorkloadPriorityClass", "name", priorityClassSpec.Name)
			return err
		}

		// Ensure ownership
		if err := ctrl.SetControllerReference(queueConfig, priorityClass, r.Scheme); err != nil {
			logger.Error(err, "Failed to set ownership for WorkloadPriorityClass", "name", priorityClassSpec.Name)
			return err
		}

		logger.Info("Kaiwo now owns WorkloadPriorityClass", "name", priorityClassSpec.Name)
	}

	logger.Info("Successfully synced ownership for Kueue resources")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KaiwoQueueConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwov1alpha1.KaiwoQueueConfig{}).
		Named("kaiwoqueueconfig").
		Complete(r)
}
