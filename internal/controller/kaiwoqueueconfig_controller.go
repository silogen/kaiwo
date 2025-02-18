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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

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

	// Fetch the requested KaiwoQueueConfig
	var queueConfig kaiwov1alpha1.KaiwoQueueConfig
	err := r.Get(ctx, req.NamespacedName, &queueConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("KaiwoQueueConfig not found, ignoring reconciliation", "name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get KaiwoQueueConfig")
		return ctrl.Result{}, err
	}

	// **Check if Status is already correct before updating**
	previousStatus := queueConfig.Status.Status
	if previousStatus == "" {
		queueConfig.Status.Status = kaiwov1alpha1.StatusPending
	}

	// **Sync Kueue Resources**
	logger.Info("Syncing Kueue resources for KaiwoQueueConfig", "name", queueConfig.Name)
	err = r.SyncKueueResources(ctx, &queueConfig)

	if err != nil {
		logger.Error(err, "Failed to sync Kueue resources for KaiwoQueueConfig")
		queueConfig.Status.Status = kaiwov1alpha1.StatusFailed
	} else {
		queueConfig.Status.Status = kaiwov1alpha1.StatusReady
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
	if queueConfig.Status.Status == kaiwov1alpha1.StatusPending {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *KaiwoQueueConfigReconciler) SyncKueueResources(ctx context.Context, queueConfig *kaiwov1alpha1.KaiwoQueueConfig) error {
	logger := log.FromContext(ctx)

	success := true

	// Sync ResourceFlavors
	kueueFlavors := controllerutils.ConvertKaiwoToKueueResourceFlavors(queueConfig.Spec.ResourceFlavors)
	for _, kueueFlavor := range kueueFlavors {
		existingFlavor := &kueuev1beta1.ResourceFlavor{}
		err := r.Get(ctx, client.ObjectKey{Name: kueueFlavor.Name}, existingFlavor)

		if errors.IsNotFound(err) {
			logger.Info("Creating ResourceFlavor", "name", kueueFlavor.Name)
			if err := r.Create(ctx, &kueueFlavor); err != nil {
				logger.Error(err, "Failed to create ResourceFlavor", "name", kueueFlavor.Name)
				success = false
			}
		} else if err != nil {
			logger.Error(err, "Failed to get ResourceFlavor", "name", kueueFlavor.Name)
			success = false
		}
	}

	// Sync ClusterQueues
	for _, kaiwoQueue := range queueConfig.Spec.ClusterQueues {
		kueueQueue := controllerutils.ConvertKaiwoToKueueClusterQueue(kaiwoQueue)
		existingQueue := &kueuev1beta1.ClusterQueue{}
		err := r.Get(ctx, client.ObjectKey{Name: kueueQueue.Name}, existingQueue)

		if errors.IsNotFound(err) {
			logger.Info("Creating ClusterQueue", "name", kueueQueue.Name)
			if err := r.Create(ctx, &kueueQueue); err != nil {
				logger.Error(err, "Failed to create ClusterQueue", "name", kueueQueue.Name)
				success = false
			}
		} else if err != nil {
			logger.Error(err, "Failed to get ClusterQueue", "name", kueueQueue.Name)
			success = false
		}
	}

	// Sync WorkloadPriorityClasses
	for _, priorityClassSpec := range queueConfig.Spec.WorkloadPriorityClasses {
		existingPriorityClass := &kueuev1beta1.WorkloadPriorityClass{}
		err := r.Get(ctx, client.ObjectKey{Name: priorityClassSpec.Name}, existingPriorityClass)

		if errors.IsNotFound(err) {
			logger.Info("Creating WorkloadPriorityClass", "name", priorityClassSpec.Name)
			if err := r.Create(ctx, &priorityClassSpec); err != nil {
				logger.Error(err, "Failed to create WorkloadPriorityClass", "name", priorityClassSpec.Name)
				success = false
			}
		} else if err != nil {
			logger.Error(err, "Failed to get WorkloadPriorityClass", "name", priorityClassSpec.Name)
			success = false
		}
	}

	if success {
		logger.Info("Successfully synced all Kueue resources")
		return nil
	} else {
		return fmt.Errorf("failed to sync some Kueue resources")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *KaiwoQueueConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := log.Log.WithName("SetupWithManager")

	// Add a startup function to ensure default KaiwoQueueConfig exists
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		logger.Info("Ensuring default KaiwoQueueConfig exists on startup...")
		if err := r.EnsureDefaultKaiwoQueueConfig(ctx); err != nil {
			logger.Error(err, "Failed to ensure default KaiwoQueueConfig on startup")
			return err
		}
		logger.Info("Default KaiwoQueueConfig verified.")
		return nil
	})); err != nil {
		return err
	}

	// Register the controller with the manager
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiwov1alpha1.KaiwoQueueConfig{}).
		Named("kaiwoqueueconfig").
		Complete(r)
}

func (r *KaiwoQueueConfigReconciler) EnsureDefaultKaiwoQueueConfig(ctx context.Context) error {
	logger := log.FromContext(ctx)

	var queueConfig kaiwov1alpha1.KaiwoQueueConfig
	err := r.Get(ctx, client.ObjectKey{Name: controllerutils.DefaultKaiwoQueueConfigName}, &queueConfig)
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to check for existing KaiwoQueueConfig")
		return err
	}

	logger.Info("Default KaiwoQueueConfig does not exist. Creating it now...")

	if err := r.CreateDefaultKaiwoQueueConfig(ctx, controllerutils.DefaultKaiwoQueueConfigName); err != nil {
		logger.Error(err, "Failed to create default KaiwoQueueConfig")
		return err
	}

	logger.Info("Successfully created default KaiwoQueueConfig")
	return nil
}

func (r *KaiwoQueueConfigReconciler) CreateDefaultKaiwoQueueConfig(ctx context.Context, name string) error {
	logger := log.FromContext(ctx)

	resourceFlavors, nodePoolResources, err := controllerutils.CreateDefaultResourceFlavors(ctx, r.Client)
	if err != nil {
		logger.Error(err, "Failed to create default resource flavors")
		return err
	}

	clusterQueue := controllerutils.CreateClusterQueue(nodePoolResources, name)

	defaultQueueConfig := kaiwov1alpha1.KaiwoQueueConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: kaiwov1alpha1.KaiwoQueueConfigSpec{
			ClusterQueues:   []kaiwov1alpha1.ClusterQueue{clusterQueue},
			ResourceFlavors: resourceFlavors,
		},
	}

	logger.Info("Creating the following kaiwoQueueConfig", name, defaultQueueConfig)

	if err := r.Create(ctx, &defaultQueueConfig); err != nil {
		logger.Error(err, "Failed to create default KaiwoQueueConfig")
		return err
	}

	logger.Info("Successfully created default KaiwoQueueConfig", "name", defaultQueueConfig.Name)
	return nil
}
