// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package state

import (
	corev1 "k8s.io/api/core/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/silogen/kaiwo/internal/controller/aim/routingconfig"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/helpers"
)

// ServiceState captures the resolved desired state for an AIMService across vendors.
type ServiceState struct {
	Name               string
	Namespace          string
	RuntimeName        string
	ModelID            string
	Env                []corev1.EnvVar
	Resources          *corev1.ResourceRequirements
	Replicas           *int32
	ImagePullSecrets   []corev1.LocalObjectReference
	ServiceAccountName string
	Template           TemplateState
	ModelSource        *aimv1alpha1.AIMModelSource
	Routing            ServiceRoutingState
}

// ServiceRoutingState carries routing configuration resolved for the service.
type ServiceRoutingState struct {
	Enabled     bool
	PathPrefix  string
	GatewayRef  *gatewayapiv1.ParentReference
	Annotations map[string]string
}

// ServiceStateOptions exposes optional overrides when constructing the service view.
type ServiceStateOptions struct {
	RuntimeName string
	RoutePath   string
}

// NewServiceState projects the AIMService and template data into a vendor-neutral structure.
func NewServiceState(service *aimv1alpha1.AIMService, template TemplateState, opts ServiceStateOptions) ServiceState {
	// TODO handle caching

	modelID := template.SpecCommon.AIMImageName
	if template.ModelSource != nil {
		modelID = template.ModelSource.Name
	}

	state := ServiceState{
		Name:               service.Name,
		Namespace:          service.Namespace,
		RuntimeName:        opts.RuntimeName,
		ModelID:            modelID,
		Env:                helpers.CopyEnvVars(service.Spec.Env),
		ImagePullSecrets:   helpers.CopyPullSecrets(service.Spec.ImagePullSecrets),
		ServiceAccountName: template.ServiceAccountName(),
		Template:           template,
		ModelSource:        template.ModelSource,
	}

	if len(template.RuntimeConfigSpec.ImagePullSecrets) > 0 {
		state.ImagePullSecrets = mergePullSecretRefs(state.ImagePullSecrets, template.RuntimeConfigSpec.ImagePullSecrets)
	}

	switch {
	case service.Spec.Resources != nil:
		state.Resources = service.Spec.Resources.DeepCopy()
	case template.SpecCommon.Resources != nil:
		state.Resources = template.SpecCommon.Resources.DeepCopy()
	case template.ImageResources != nil:
		state.Resources = template.ImageResources.DeepCopy()
	}

	if state.RuntimeName == "" {
		state.RuntimeName = template.Name
	}

	if state.ModelID == "" {
		state.ModelID = template.SpecCommon.AIMImageName
	}

	if service.Spec.Replicas != nil {
		replicas := *service.Spec.Replicas
		state.Replicas = &replicas
	}

	resolvedRouting := routingconfig.Resolve(service, template.RuntimeConfigSpec.Routing)

	if resolvedRouting.Enabled {
		routing := ServiceRoutingState{
			Enabled:    true,
			PathPrefix: opts.RoutePath,
		}
		if routing.PathPrefix == "" {
			routing.PathPrefix = "/"
		}
		if resolvedRouting.GatewayRef != nil {
			routing.GatewayRef = resolvedRouting.GatewayRef.DeepCopy()
		}
		if service.Spec.Routing != nil && len(service.Spec.Routing.Annotations) > 0 {
			routing.Annotations = make(map[string]string, len(service.Spec.Routing.Annotations))
			for k, v := range service.Spec.Routing.Annotations {
				routing.Annotations[k] = v
			}
		}
		state.Routing = routing
	}

	return state
}

// mergePullSecretRefs merges image pull secrets from base and extras, avoiding duplicates.
// Extras take precedence when there's a name collision.
func mergePullSecretRefs(base []corev1.LocalObjectReference, extras []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if len(extras) == 0 {
		return base
	}
	if len(base) == 0 {
		return helpers.CopyPullSecrets(extras)
	}

	index := make(map[string]struct{}, len(base))
	for _, secret := range base {
		index[secret.Name] = struct{}{}
	}

	for _, secret := range extras {
		if _, exists := index[secret.Name]; exists {
			continue
		}
		base = append(base, secret)
	}

	return base
}
