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
// with service-level overrides. Service fields take precedence when explicitly set.
func Resolve(service *aimv1alpha1.AIMService, runtime *aimv1alpha1.AIMRuntimeRoutingConfig) ResolvedRouting {
	var resolved ResolvedRouting
	var serviceRouting *aimv1alpha1.AIMServiceRouting

	if service != nil {
		serviceRouting = service.Spec.Routing
	}

	// Start with runtime config defaults
	if runtime != nil {
		if runtime.Enabled != nil {
			resolved.Enabled = *runtime.Enabled
		}
		if runtime.GatewayRef != nil {
			resolved.GatewayRef = runtime.GatewayRef.DeepCopy()
		}
		resolved.PathTemplate = runtime.PathTemplate
	}

	// Override with service-level config when present
	if serviceRouting != nil {
		// Service Enabled always takes precedence (even if false)
		resolved.Enabled = serviceRouting.Enabled

		// Override GatewayRef if service specifies one
		if serviceRouting.GatewayRef != nil {
			resolved.GatewayRef = serviceRouting.GatewayRef.DeepCopy()
		}

		// Service annotations (no merging, just use service annotations)
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
