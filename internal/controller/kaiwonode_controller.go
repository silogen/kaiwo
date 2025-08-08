/*
Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/retry"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	nodeutils "github.com/silogen/kaiwo/internal/controller/utils/nodes"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// KaiwoNodeReconciler operates on the Kaiwo nodes
type KaiwoNodeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Tasks    []nodeutils.ReconcileTask[*nodeutils.KaiwoNodeWrapper]
}

// +kubebuilder:rbac:groups=kaiwo.silogen.ai,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.kaiwo.silogen.ai,resources=kaiwoconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *KaiwoNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, err := common.GetContextWithConfig(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting context: %w", err)
	}
	logger := log.FromContext(ctx)
	baseutils.Debug(logger, "Running reconciliation")

	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get node: %w", err)
	} else if err != nil && errors.IsNotFound(err) {
		// If node does not exist, it has been deleted
		// The KaiwoNode will be deleted automatically, so we can safely return here
		return ctrl.Result{}, nil
	}

	kaiwoNode := &v1alpha1.KaiwoNode{}
	if err := r.Get(ctx, req.NamespacedName, kaiwoNode); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get Kaiwo node: %w", err)
	} else if err != nil && errors.IsNotFound(err) {
		kaiwoNode = nil
	}

	wrapper := &nodeutils.KaiwoNodeWrapper{
		Node:      node,
		KaiwoNode: kaiwoNode,
	}

	// Keep a copy of the original objects for later patching
	originalObjects := &nodeutils.KaiwoNodeWrapper{}
	originalObjects.Node = node.DeepCopy()

	if kaiwoNode != nil {
		originalObjects.KaiwoNode = kaiwoNode.DeepCopy()
	}

	result := ctrl.Result{}

	for _, task := range r.Tasks {
		baseutils.Debug(logger, fmt.Sprintf("Running task %s", task.Name()))
		res, err := task.Run(ctx, wrapper)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to run task %s: %w", task.Name(), err)
		}
		if res != nil {
			result = *res
			break
		}
	}

	if err := r.applyPatches(ctx, originalObjects, wrapper); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply patches: %w", err)
	}

	return result, nil
}

func (r *KaiwoNodeReconciler) applyPatches(ctx context.Context,
	original *nodeutils.KaiwoNodeWrapper,
	wrapper *nodeutils.KaiwoNodeWrapper,
) error {
	if original.KaiwoNode == nil {
		newCR := wrapper.KaiwoNode
		if err := r.Create(ctx, newCR); err != nil {
			return err
		}
	} else {
		// spec changes?
		if original.KaiwoNode == nil || !equality.Semantic.DeepEqual(original.KaiwoNode.Spec, wrapper.KaiwoNode.Spec) {
			if original.KaiwoNode != nil {
				wrapper.KaiwoNode.SetResourceVersion(original.KaiwoNode.GetResourceVersion())
			}
			if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return r.Patch(ctx, wrapper.KaiwoNode, client.MergeFrom(original.KaiwoNode))
			}); err != nil {
				return err
			}
		}
		// status changes?
		if original.KaiwoNode != nil && !equality.Semantic.DeepEqual(original.KaiwoNode.Status, wrapper.KaiwoNode.Status) {
			wrapper.KaiwoNode.SetResourceVersion(original.KaiwoNode.GetResourceVersion())
			if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return r.Status().Patch(ctx, wrapper.KaiwoNode, client.MergeFrom(original.KaiwoNode))
			}); err != nil {
				return err
			}
		}
	}

	if !equality.Semantic.DeepEqual(original.Node.Labels, wrapper.Node.Labels) ||
		!equality.Semantic.DeepEqual(original.Node.Spec.Taints, wrapper.Node.Spec.Taints) {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Patch(ctx, wrapper.Node, client.MergeFrom(original.Node))
		}); err != nil {
			return err
		}
	}

	return nil
}

func (r *KaiwoNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	nodeMapper := handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Name: obj.GetName()},
			}}
		},
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KaiwoNode{}).
		Watches(
			&corev1.Node{},
			nodeMapper,
		).
		Named("kaiwonode").
		Complete(r)
}

func NewKaiwoNodeReconciler(mgr ctrl.Manager) *KaiwoNodeReconciler {
	k8sClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	recorder := mgr.GetEventRecorderFor("kaiwonode-controller")
	return &KaiwoNodeReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: recorder,
		Tasks: []nodeutils.ReconcileTask[*nodeutils.KaiwoNodeWrapper]{
			&nodeutils.EnsureKaiwoNodeTask{
				Client: k8sClient,
				Scheme: scheme,
			},
			&nodeutils.UpdateKaiwoNodeTask{
				Client: k8sClient,
			},
			&nodeutils.NodeLabelsAndTaintsTask{
				Client: k8sClient,
			},
			&nodeutils.GpuPartitionTask{
				Client:   k8sClient,
				Recorder: recorder,
			},
		},
	}
}
