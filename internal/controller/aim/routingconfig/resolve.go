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

package routingconfig

import (
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// ResolvedRouting captures the effective routing configuration after applying service overrides
// on top of runtime defaults.
type ResolvedRouting struct {
	Enabled      bool
	GatewayRef   *gatewayapiv1.ParentReference
	Annotations  map[string]string
	PathTemplate string
}

// Resolve determines the effective routing configuration by merging runtime config defaults
// with service-level overrides. The precedence order is:
// 1. Runtime config (base layer)
// 2. Service.Spec.RuntimeOverrides.Routing (middle layer - overrides runtime config)
// 3. Service.Spec.Routing (highest priority - overrides everything)
func Resolve(service *aimv1alpha1.AIMService, runtime *aimv1alpha1.AIMRuntimeRoutingConfig) ResolvedRouting {
	var resolved ResolvedRouting
	var serviceRouting *aimv1alpha1.AIMServiceRouting
	var runtimeOverrideRouting *aimv1alpha1.AIMRuntimeRoutingConfig

	if service != nil {
		serviceRouting = service.Spec.Routing
		if service.Spec.RuntimeOverrides != nil {
			runtimeOverrideRouting = service.Spec.RuntimeOverrides.Routing
		}
	}

	// Layer 1: Start with runtime config defaults
	if runtime != nil {
		if runtime.Enabled != nil {
			resolved.Enabled = *runtime.Enabled
		}
		if runtime.GatewayRef != nil {
			resolved.GatewayRef = runtime.GatewayRef.DeepCopy()
		}
		resolved.PathTemplate = runtime.PathTemplate
	}

	// Layer 2: Apply runtime overrides from service
	if runtimeOverrideRouting != nil {
		if runtimeOverrideRouting.Enabled != nil {
			resolved.Enabled = *runtimeOverrideRouting.Enabled
		}
		if runtimeOverrideRouting.GatewayRef != nil {
			resolved.GatewayRef = runtimeOverrideRouting.GatewayRef.DeepCopy()
		}
		if runtimeOverrideRouting.PathTemplate != "" {
			resolved.PathTemplate = runtimeOverrideRouting.PathTemplate
		}
	}

	// Layer 3: Apply service-level routing config (highest priority)
	if serviceRouting != nil {
		// Only override enabled if explicitly set (non-nil)
		// nil means inherit from previous layers
		if serviceRouting.Enabled != nil {
			resolved.Enabled = *serviceRouting.Enabled
		}

		// Override GatewayRef if service specifies one
		if serviceRouting.GatewayRef != nil {
			resolved.GatewayRef = serviceRouting.GatewayRef.DeepCopy()
		}

		// Service annotations (only service level has annotations)
		if len(serviceRouting.Annotations) > 0 {
			resolved.Annotations = make(map[string]string, len(serviceRouting.Annotations))
			for k, v := range serviceRouting.Annotations {
				resolved.Annotations[k] = v
			}
		}

		// Override PathTemplate if service specifies one
		if serviceRouting.PathTemplate != "" {
			resolved.PathTemplate = serviceRouting.PathTemplate
		}
	}

	return resolved
}
