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

package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

// RootOwnerResult holds the resolved root owner and the owner chain string.
type RootOwnerResult struct {
	Ref        kaiwo.WorkloadReference
	OwnerChain string
	Namespace  string
}

const maxOwnerDepth = 10

// ResolveRootOwner walks the controller ownerReferences chain from a pod up to
// the root owner (the resource with no controller owner). It uses the
// unstructured client so it works with arbitrary resource types without
// importing their Go types.
func ResolveRootOwner(ctx context.Context, c client.Client, namespace string, name string, kind string, apiVersion string, uid types.UID) (*RootOwnerResult, error) {
	var chain []string
	chain = append(chain, fmt.Sprintf("%s/%s", kind, name))

	currentNamespace := namespace
	currentGVK := gvkFromAPIVersionKind(apiVersion, kind)
	currentName := name

	for i := 0; i < maxOwnerDepth; i++ {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(currentGVK)
		if err := c.Get(ctx, client.ObjectKey{Namespace: currentNamespace, Name: currentName}, obj); err != nil {
			if errors.IsForbidden(err) {
				return nil, fmt.Errorf("cannot resolve owner chain: RBAC forbids reading %s %q in namespace %q — "+
					"add get permission for this resource to the operator ServiceAccount: %w",
					currentGVK.Kind, currentName, currentNamespace, err)
			}
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("owner %s %q not found in namespace %q: %w",
					currentGVK.Kind, currentName, currentNamespace, err)
			}
			return nil, fmt.Errorf("transient error resolving owner %s %q in namespace %q: %w",
				currentGVK.Kind, currentName, currentNamespace, err)
		}

		controllerRef := getControllerOwnerRef(obj.GetOwnerReferences())
		if controllerRef == nil {
			return &RootOwnerResult{
				Ref: kaiwo.WorkloadReference{
					APIVersion: obj.GetAPIVersion(),
					Kind:       obj.GetKind(),
					Name:       obj.GetName(),
					UID:        obj.GetUID(),
				},
				OwnerChain: strings.Join(chain, " -> "),
				Namespace:  currentNamespace,
			}, nil
		}

		currentGVK = gvkFromAPIVersionKind(controllerRef.APIVersion, controllerRef.Kind)
		currentName = controllerRef.Name
		chain = append(chain, fmt.Sprintf("%s/%s", controllerRef.Kind, controllerRef.Name))
	}

	return nil, fmt.Errorf("owner reference chain exceeded maximum depth of %d for %s/%s in %s", maxOwnerDepth, kind, name, namespace)
}

func getControllerOwnerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	return nil
}

func gvkFromAPIVersionKind(apiVersion, kind string) schema.GroupVersionKind {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		logger := log.Log.WithName("gpuworkload-resolver")
		logger.V(1).Info("failed to parse apiVersion, falling back to kind-only GVK",
			"apiVersion", apiVersion, "kind", kind, "error", err)
		return schema.GroupVersionKind{Kind: kind}
	}
	return gv.WithKind(kind)
}

// GpuWorkloadName generates a deterministic name for a GpuWorkload CR from
// the root owner: <lowercase-kind>-<name>-<first-8-chars-of-uid>.
func GpuWorkloadName(kind string, name string, uid types.UID) string {
	uidPrefix := string(uid)
	if len(uidPrefix) > 8 {
		uidPrefix = uidPrefix[:8]
	}
	return fmt.Sprintf("%s-%s-%s", strings.ToLower(kind), name, uidPrefix)
}
