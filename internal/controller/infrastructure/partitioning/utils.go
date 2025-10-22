/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to do so, subject to the following
conditions:

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

package partitioning

import (
	"context"
	"fmt"
	"sort"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/silogen/kaiwo/apis/infrastructure/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

type reconcileErrors struct {
	ObserveErr  error
	PlanErr     error
	ApplyErr    error
	CleanupErr  error
	FinalizeErr error
}

func (e reconcileErrors) HasError() bool {
	return e.ObserveErr != nil ||
		e.PlanErr != nil ||
		e.ApplyErr != nil ||
		e.CleanupErr != nil ||
		e.FinalizeErr != nil
}

func applyDesiredState(
	ctx context.Context,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	desired []client.Object,
	fieldOwner string,
) error {
	if len(desired) == 0 {
		return nil
	}

	for _, obj := range desired {
		if err := stampGVK(obj, scheme); err != nil {
			return fmt.Errorf("failed to stamp GVK: %w", err)
		}
	}

	sorted := sortObjects(desired)
	logger := log.FromContext(ctx)

	for _, obj := range sorted {
		gvk := obj.GetObjectKind().GroupVersionKind()
		key := client.ObjectKeyFromObject(obj)

		baseutils.Debug(logger, "Applying object",
			"gvk", gvk.String(),
			"namespace", key.Namespace,
			"name", key.Name,
		)

		if err := k8sClient.Patch(
			ctx,
			obj,
			client.Apply,
			client.ForceOwnership,
			client.FieldOwner(fieldOwner),
		); err != nil {
			return fmt.Errorf("failed to apply %s %s: %w", gvk.Kind, key.Name, err)
		}
	}

	return nil
}

func stampGVK(obj client.Object, scheme *runtime.Scheme) error {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return fmt.Errorf("cannot find GVK for %T: %w", obj, err)
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no GVK registered for %T", obj)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}

func sortObjects(objects []client.Object) []client.Object {
	sorted := make([]client.Object, len(objects))
	copy(sorted, objects)

	sort.Slice(sorted, func(i, j int) bool {
		objI := sorted[i]
		objJ := sorted[j]

		gvkI := objI.GetObjectKind().GroupVersionKind().String()
		gvkJ := objJ.GetObjectKind().GroupVersionKind().String()

		if gvkI != gvkJ {
			return gvkI < gvkJ
		}

		nsI := objI.GetNamespace()
		nsJ := objJ.GetNamespace()

		if nsI != nsJ {
			return nsI < nsJ
		}

		return objI.GetName() < objJ.GetName()
	})

	return sorted
}

func patchNodePartitioningStatus(
	ctx context.Context,
	k8sClient client.Client,
	np *infrastructurev1alpha1.NodePartitioning,
	originalStatus *infrastructurev1alpha1.NodePartitioningStatus,
) error {
	desiredStatus := np.Status.DeepCopy()
	if desiredStatus == nil {
		desiredStatus = &infrastructurev1alpha1.NodePartitioningStatus{}
	}

	if originalStatus != nil && apiequality.Semantic.DeepEqual(originalStatus, desiredStatus) {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest infrastructurev1alpha1.NodePartitioning
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(np), &latest); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		targetStatus := desiredStatus.DeepCopy()
		targetStatus.ObservedGeneration = latest.Generation

		if apiequality.Semantic.DeepEqual(&latest.Status, targetStatus) {
			np.Status = latest.Status
			return nil
		}

		latest.Status = *targetStatus
		if err := k8sClient.Status().Update(ctx, &latest); err != nil {
			return err
		}

		np.Status = latest.Status
		return nil
	})
}

func patchPartitioningPlanStatus(
	ctx context.Context,
	k8sClient client.Client,
	plan *infrastructurev1alpha1.PartitioningPlan,
	originalStatus *infrastructurev1alpha1.PartitioningPlanStatus,
) error {
	desiredStatus := plan.Status.DeepCopy()
	if desiredStatus == nil {
		desiredStatus = &infrastructurev1alpha1.PartitioningPlanStatus{}
	}

	if originalStatus != nil && apiequality.Semantic.DeepEqual(originalStatus, desiredStatus) {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest infrastructurev1alpha1.PartitioningPlan
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(plan), &latest); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		targetStatus := desiredStatus.DeepCopy()
		targetStatus.ObservedGeneration = latest.Generation

		if apiequality.Semantic.DeepEqual(&latest.Status, targetStatus) {
			plan.Status = latest.Status
			return nil
		}

		latest.Status = *targetStatus
		if err := k8sClient.Status().Update(ctx, &latest); err != nil {
			return err
		}

		plan.Status = latest.Status
		return nil
	})
}
