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
	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
)

// BuildTemplateStateFromObservation constructs a TemplateState from the template specification, observation, and status.
// This is an adapter function that combines template metadata with observed resources.
func BuildTemplateStateFromObservation(
	name, namespace string,
	specCommon aimv1alpha1.AIMServiceTemplateSpecCommon,
	observation *TemplateObservation,
	runtimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec,
	status *aimv1alpha1.AIMServiceTemplateStatus,
) aimstate.TemplateState {
	base := aimstate.TemplateState{
		Name:              name,
		Namespace:         namespace,
		SpecCommon:        specCommon,
		RuntimeConfigSpec: runtimeConfigSpec,
		Status:            status,
	}

	if observation != nil {
		base.Image = observation.Image
		base.ImageResources = observation.ImageResources
		base.ImagePullSecrets = observation.ImagePullSecrets
	}

	return aimstate.NewTemplateState(base)
}

// BuildServingRuntimeFromState constructs a namespaced ServingRuntime from a TemplateState snapshot.
// This is an adapter function that maintains compatibility with the original signature.
func BuildServingRuntimeFromState(state aimstate.TemplateState, ownerRef metav1.OwnerReference) *servingv1alpha1.ServingRuntime {
	return BuildServingRuntime(state, ownerRef)
}
