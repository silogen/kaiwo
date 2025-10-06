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

package framework

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// ApplyDesiredState applies the desired set of objects via Server-Side Apply (SSA).
// Objects are applied in deterministic order: by GVK, then namespace, then name.
func ApplyDesiredState(
	ctx context.Context,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	desired []client.Object,
	config ApplyConfig,
) error {
	logger := log.FromContext(ctx)

	if len(desired) == 0 {
		return nil
	}

	// Ensure all objects have GVK set
	for _, obj := range desired {
		if err := stampGVK(obj, scheme); err != nil {
			return fmt.Errorf("failed to stamp GVK: %w", err)
		}
	}

	// Sort deterministically
	sorted := sortObjects(desired)

	// Apply each object via SSA
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
			client.FieldOwner(config.FieldOwner),
		); err != nil {
			return fmt.Errorf("failed to apply %s %s: %w", gvk.Kind, key.Name, err)
		}
	}

	// TODO: Implement pruning if config.EnablePruning is true
	// - List objects with inventory labels
	// - Delete those not in desired set

	return nil
}

// stampGVK ensures the object has its GVK set from the scheme
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

// sortObjects returns objects sorted by GVK, namespace, name for determinism
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
