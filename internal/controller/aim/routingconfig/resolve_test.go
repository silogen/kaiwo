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
	"testing"

	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestResolveUsesRuntimeDefaultsWhenServiceMissing(t *testing.T) {
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(true),
		GatewayRef: &gatewayapiv1.ParentReference{
			Name: "default-gateway",
		},
	}

	result := Resolve(nil, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to be enabled from runtime defaults")
	}
	if result.GatewayRef == nil {
		t.Fatalf("expected gateway ref to be inherited from runtime defaults")
	}
	if result.GatewayRef == runtimeCfg.GatewayRef {
		t.Fatalf("expected gateway ref to be deep copied, but pointers match")
	}
}

func TestResolvePrefersServiceOverrides(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled: false,
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(true),
	}

	result := Resolve(service, runtimeCfg)

	if result.Enabled {
		t.Fatalf("expected service override to disable routing")
	}
}

func TestResolveFallsBackGatewayWhenServiceMissing(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled: true,
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		GatewayRef: &gatewayapiv1.ParentReference{Name: "shared-gateway"},
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to remain enabled from service override")
	}
	if result.GatewayRef == nil {
		t.Fatalf("expected gateway ref to fall back to runtime config")
	}
	if result.GatewayRef == runtimeCfg.GatewayRef {
		t.Fatalf("expected gateway ref deep copy when falling back to runtime config")
	}
}
