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

package shared

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NodeGPUChangePredicate returns a predicate that triggers reconciles when GPU-related node attributes change.
func NodeGPUChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, ok := e.Object.(*corev1.Node)
			return ok
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, ok := e.Object.(*corev1.Node)
			return ok
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, okOld := e.ObjectOld.(*corev1.Node)
			newNode, okNew := e.ObjectNew.(*corev1.Node)
			if !okOld || !okNew {
				return false
			}
			return nodeGPUInfoChanged(oldNode, newNode)
		},
	}
}

func nodeGPUInfoChanged(oldNode, newNode *corev1.Node) bool {
	if labelsContainGPUChanges(oldNode.Labels, newNode.Labels) {
		return true
	}

	if resourcesContainGPUChanges(oldNode.Status.Capacity, newNode.Status.Capacity) {
		return true
	}

	if resourcesContainGPUChanges(oldNode.Status.Allocatable, newNode.Status.Allocatable) {
		return true
	}

	return false
}

func labelsContainGPUChanges(oldLabels, newLabels map[string]string) bool {
	visited := map[string]struct{}{}

	for key, oldVal := range oldLabels {
		if !isGPUKey(key) {
			continue
		}
		visited[key] = struct{}{}
		if newVal, ok := newLabels[key]; !ok || newVal != oldVal {
			return true
		}
	}

	for key, newVal := range newLabels {
		if !isGPUKey(key) {
			continue
		}
		if _, seen := visited[key]; seen {
			continue
		}
		if oldVal, ok := oldLabels[key]; !ok || oldVal != newVal {
			return true
		}
	}

	return false
}

func resourcesContainGPUChanges(oldResources, newResources corev1.ResourceList) bool {
	keys := map[corev1.ResourceName]struct{}{}

	for name := range oldResources {
		if isGPUResource(string(name)) {
			keys[name] = struct{}{}
		}
	}
	for name := range newResources {
		if isGPUResource(string(name)) {
			keys[name] = struct{}{}
		}
	}

	for name := range keys {
		oldQty, oldOK := oldResources[name]
		newQty, newOK := newResources[name]
		if oldOK != newOK {
			return true
		}
		if oldOK && newOK && oldQty.Cmp(newQty) != 0 {
			return true
		}
	}

	return false
}

func isGPUKey(key string) bool {
	return strings.HasPrefix(key, "amd.com/") || strings.HasPrefix(key, "nvidia.com/")
}
