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

package shared

import (
	"testing"

	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

const (
	testClusterGateway       = "cluster-gateway"
	testNamespaceGateway     = "namespace-gateway"
	testClusterRouteTemplate = "/cluster/{.metadata.name}"
	testNsRouteTemplate      = "/namespace/{.metadata.name}"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestMergeRuntimeConfigSpecs_OnlyCluster(t *testing.T) {
	clusterSpec := aimv1alpha1.AIMClusterRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			DefaultStorageClassName: "cluster-storage",
			Routing: &aimv1alpha1.AIMRuntimeRoutingConfig{
				Enabled:      boolPtr(true),
				GatewayRef:   &gatewayapiv1.ParentReference{Name: testClusterGateway},
				PathTemplate: testClusterRouteTemplate,
			},
		},
	}

	namespaceSpec := aimv1alpha1.AIMRuntimeConfigSpec{}

	merged := mergeRuntimeConfigSpecs(clusterSpec, namespaceSpec)

	if merged.DefaultStorageClassName != "cluster-storage" {
		t.Errorf("expected cluster storage class, got %s", merged.DefaultStorageClassName)
	}
	if merged.Routing == nil {
		t.Fatal("expected routing config from cluster")
	}
	if *merged.Routing.Enabled != true {
		t.Error("expected routing enabled from cluster")
	}
	if merged.Routing.GatewayRef.Name != testClusterGateway {
		t.Errorf("expected cluster gateway, got %s", merged.Routing.GatewayRef.Name)
	}
}

func TestMergeRuntimeConfigSpecs_NamespaceOverridesStorage(t *testing.T) {
	clusterSpec := aimv1alpha1.AIMClusterRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			DefaultStorageClassName: "cluster-storage",
		},
	}

	namespaceSpec := aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			DefaultStorageClassName: "namespace-storage",
		},
	}

	merged := mergeRuntimeConfigSpecs(clusterSpec, namespaceSpec)

	if merged.DefaultStorageClassName != "namespace-storage" {
		t.Errorf("expected namespace storage to override, got %s", merged.DefaultStorageClassName)
	}
}

func TestMergeRuntimeConfigSpecs_MergesRouting(t *testing.T) {
	clusterSpec := aimv1alpha1.AIMClusterRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Routing: &aimv1alpha1.AIMRuntimeRoutingConfig{
				Enabled:      boolPtr(true),
				GatewayRef:   &gatewayapiv1.ParentReference{Name: testClusterGateway},
				PathTemplate: testClusterRouteTemplate,
			},
		},
	}

	namespaceSpec := aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Routing: &aimv1alpha1.AIMRuntimeRoutingConfig{
				PathTemplate: testNsRouteTemplate,
			},
		},
	}

	merged := mergeRuntimeConfigSpecs(clusterSpec, namespaceSpec)

	if merged.Routing == nil {
		t.Fatal("expected routing config to be merged")
	}
	// Enabled should come from cluster (namespace didn't set it)
	if merged.Routing.Enabled == nil || *merged.Routing.Enabled != true {
		t.Error("expected enabled from cluster config")
	}
	// GatewayRef should come from cluster (namespace didn't set it)
	if merged.Routing.GatewayRef == nil || merged.Routing.GatewayRef.Name != testClusterGateway {
		t.Error("expected gateway from cluster config")
	}
	// RouteTemplate should come from namespace (override)
	if merged.Routing.PathTemplate != testNsRouteTemplate {
		t.Errorf("expected namespace route template, got %s", merged.Routing.PathTemplate)
	}
}

func TestMergeRoutingConfig_BothPresent(t *testing.T) {
	clusterRouting := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(true),
		GatewayRef:   &gatewayapiv1.ParentReference{Name: testClusterGateway},
		PathTemplate: testClusterRouteTemplate,
	}

	namespaceRouting := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(false),
		PathTemplate: testNsRouteTemplate,
	}

	merged := mergeRoutingConfig(clusterRouting, namespaceRouting)

	if merged == nil {
		t.Fatal("expected merged routing config")
	}
	// Namespace overrides enabled
	if *merged.Enabled != false {
		t.Error("expected namespace enabled to override")
	}
	// Cluster gateway should remain (namespace didn't set it)
	if merged.GatewayRef.Name != testClusterGateway {
		t.Errorf("expected cluster gateway to remain, got %s", merged.GatewayRef.Name)
	}
	// Namespace route template overrides
	if merged.PathTemplate != testNsRouteTemplate {
		t.Errorf("expected namespace route template, got %s", merged.PathTemplate)
	}
}

func TestMergeRoutingConfig_NamespaceOverridesGateway(t *testing.T) {
	clusterRouting := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:    boolPtr(true),
		GatewayRef: &gatewayapiv1.ParentReference{Name: testClusterGateway},
	}

	namespaceRouting := &aimv1alpha1.AIMRuntimeRoutingConfig{
		GatewayRef: &gatewayapiv1.ParentReference{Name: testNamespaceGateway},
	}

	merged := mergeRoutingConfig(clusterRouting, namespaceRouting)

	if merged == nil {
		t.Fatal("expected merged routing config")
	}
	// Enabled from cluster (namespace didn't set it)
	if merged.Enabled == nil || *merged.Enabled != true {
		t.Error("expected cluster enabled value")
	}
	// Gateway from namespace (override)
	if merged.GatewayRef.Name != testNamespaceGateway {
		t.Errorf("expected namespace gateway override, got %s", merged.GatewayRef.Name)
	}
}

func TestMergeRoutingConfig_OnlyCluster(t *testing.T) {
	clusterRouting := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(true),
		GatewayRef:   &gatewayapiv1.ParentReference{Name: testClusterGateway},
		PathTemplate: testClusterRouteTemplate,
	}

	merged := mergeRoutingConfig(clusterRouting, nil)

	if merged == nil {
		t.Fatal("expected merged routing config")
	}
	if merged == clusterRouting {
		t.Error("expected deep copy, got same pointer")
	}
	if *merged.Enabled != true {
		t.Error("expected cluster enabled value")
	}
	if merged.GatewayRef.Name != testClusterGateway {
		t.Errorf("expected cluster gateway, got %s", merged.GatewayRef.Name)
	}
}

func TestMergeRoutingConfig_OnlyNamespace(t *testing.T) {
	namespaceRouting := &aimv1alpha1.AIMRuntimeRoutingConfig{
		Enabled:      boolPtr(false),
		PathTemplate: testNsRouteTemplate,
	}

	merged := mergeRoutingConfig(nil, namespaceRouting)

	if merged == nil {
		t.Fatal("expected merged routing config")
	}
	if merged == namespaceRouting {
		t.Error("expected deep copy, got same pointer")
	}
	if *merged.Enabled != false {
		t.Error("expected namespace enabled value")
	}
	if merged.PathTemplate != testNsRouteTemplate {
		t.Errorf("expected namespace route template, got %s", merged.PathTemplate)
	}
}

func TestMergeRoutingConfig_BothNil(t *testing.T) {
	merged := mergeRoutingConfig(nil, nil)

	if merged != nil {
		t.Error("expected nil when both inputs are nil")
	}
}
