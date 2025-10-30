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
		Enabled: boolPtr(false),
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
		Enabled: boolPtr(true),
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

func TestResolveMergesAnnotations(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled: boolPtr(true),
		Annotations: map[string]string{
			"service-annotation": "service-value",
		},
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(true),
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to be enabled")
	}
	if len(result.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(result.Annotations))
	}
	if result.Annotations["service-annotation"] != "service-value" {
		t.Fatalf("expected service annotation to be present")
	}
}

func TestResolveMergesPathTemplate(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      boolPtr(true),
		PathTemplate: "/custom/{.metadata.name}",
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(true),
		PathTemplate: "/default/{.metadata.name}",
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to be enabled")
	}
	if result.PathTemplate != "/custom/{.metadata.name}" {
		t.Fatalf("expected service route template to override runtime, got %s", result.PathTemplate)
	}
}

func TestResolveUsesRuntimePathTemplateWhenServiceEmpty(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled: boolPtr(true),
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(true),
		PathTemplate: "/default/{.metadata.name}",
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to be enabled")
	}
	if result.PathTemplate != "/default/{.metadata.name}" {
		t.Fatalf("expected runtime route template when service doesn't specify one, got %s", result.PathTemplate)
	}
}

func TestResolveServiceOverridesGatewayRef(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:    boolPtr(true),
		GatewayRef: &gatewayapiv1.ParentReference{Name: "service-gateway"},
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:    boolPtr(true),
		GatewayRef: &gatewayapiv1.ParentReference{Name: "runtime-gateway"},
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to be enabled")
	}
	if result.GatewayRef == nil {
		t.Fatalf("expected gateway ref to be present")
	}
	if result.GatewayRef.Name != "service-gateway" {
		t.Fatalf("expected service gateway ref to override runtime, got %s", result.GatewayRef.Name)
	}
}

func TestResolveCompleteServiceOverride(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      boolPtr(false),
		GatewayRef:   &gatewayapiv1.ParentReference{Name: "service-gateway"},
		PathTemplate: "/service/{.metadata.name}",
		Annotations: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(true),
		GatewayRef:   &gatewayapiv1.ParentReference{Name: "runtime-gateway"},
		PathTemplate: "/runtime/{.metadata.name}",
	}

	result := Resolve(service, runtimeCfg)

	// Service overrides all fields
	if result.Enabled {
		t.Fatalf("expected routing to be disabled from service override")
	}
	if result.GatewayRef.Name != "service-gateway" {
		t.Fatalf("expected service gateway ref, got %s", result.GatewayRef.Name)
	}
	if result.PathTemplate != "/service/{.metadata.name}" {
		t.Fatalf("expected service route template, got %s", result.PathTemplate)
	}
	if len(result.Annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(result.Annotations))
	}
}

func TestResolveInheritsEnabledWhenServiceDoesNotSpecify(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		// Enabled is nil - should inherit from runtime
		Annotations: map[string]string{
			"my-annotation": "my-value",
		},
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(true),
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected routing to be enabled via inheritance from runtime config")
	}
	if len(result.Annotations) != 1 {
		t.Fatalf("expected 1 annotation from service, got %d", len(result.Annotations))
	}
	if result.Annotations["my-annotation"] != "my-value" {
		t.Fatalf("expected service annotation to be present")
	}
}

func TestResolveInheritsEnabledFalseWhenServiceDoesNotSpecify(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		// Enabled is nil - should inherit from runtime
		GatewayRef: &gatewayapiv1.ParentReference{Name: "my-gateway"},
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(false),
	}

	result := Resolve(service, runtimeCfg)

	if result.Enabled {
		t.Fatalf("expected routing to be disabled via inheritance from runtime config")
	}
	if result.GatewayRef == nil || result.GatewayRef.Name != "my-gateway" {
		t.Fatalf("expected service gateway ref to be present")
	}
}

func TestResolveServiceExplicitlyDisablesOverridingRuntimeTrue(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled: boolPtr(false),
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(true),
	}

	result := Resolve(service, runtimeCfg)

	if result.Enabled {
		t.Fatalf("expected service explicit false to override runtime true")
	}
}

func TestResolveServiceExplicitlyEnablesOverridingRuntimeFalse(t *testing.T) {
	service := &aimv1alpha1.AIMService{}
	service.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled: boolPtr(true),
	}
	runtimeCfg := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled: boolPtr(false),
	}

	result := Resolve(service, runtimeCfg)

	if !result.Enabled {
		t.Fatalf("expected service explicit true to override runtime false")
	}
}
