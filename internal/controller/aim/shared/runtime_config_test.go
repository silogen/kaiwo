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
	testTeamPlatform         = "platform"
	testCostCenterEng        = "engineering"
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
		return
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
		return
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
		return
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
		return
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
		return
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

func TestPropagateLabels_Disabled(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{
		"team":        testTeamPlatform,
		"cost-center": testCostCenterEng,
		"environment": "production",
	})

	child := &aimv1alpha1.AIMService{}

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: false,
			Match:   []string{"team", "cost-center"},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 0 {
		t.Errorf("expected no labels to be propagated when disabled, got %v", childLabels)
	}
}

func TestPropagateLabels_NilConfig(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{"team": "platform"})

	child := &aimv1alpha1.AIMService{}

	PropagateLabels(parent, child, nil)

	childLabels := child.GetLabels()
	if len(childLabels) != 0 {
		t.Errorf("expected no labels when config is nil, got %v", childLabels)
	}
}

func TestPropagateLabels_ExactMatch(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{
		"team":        testTeamPlatform,
		"cost-center": testCostCenterEng,
		"environment": "production",
	})

	child := &aimv1alpha1.AIMService{}

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: true,
			Match:   []string{"team", "cost-center"},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(childLabels))
	}
	if childLabels["team"] != testTeamPlatform {
		t.Errorf("expected team=%s, got %s", testTeamPlatform, childLabels["team"])
	}
	if childLabels["cost-center"] != testCostCenterEng {
		t.Errorf("expected cost-center=%s, got %s", testCostCenterEng, childLabels["cost-center"])
	}
	if _, exists := childLabels["environment"]; exists {
		t.Error("environment label should not be propagated")
	}
}

func TestPropagateLabels_WildcardMatch(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{
		"org.acme/team":         testTeamPlatform,
		"org.acme/cost-center":  testCostCenterEng,
		"org.acme/environment":  "production",
		"kubernetes.io/managed": "true",
	})

	child := &aimv1alpha1.AIMService{}

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: true,
			Match:   []string{"org.acme/*"},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 3 {
		t.Errorf("expected 3 labels matching org.acme/*, got %d: %v", len(childLabels), childLabels)
	}
	if childLabels["org.acme/team"] != testTeamPlatform {
		t.Errorf("expected org.acme/team=%s, got %s", testTeamPlatform, childLabels["org.acme/team"])
	}
	if _, exists := childLabels["kubernetes.io/managed"]; exists {
		t.Error("kubernetes.io/managed label should not be propagated")
	}
}

func TestPropagateLabels_MultiplePatterns(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{
		"org.acme/team":          testTeamPlatform,
		"cost-center":            testCostCenterEng,
		"environment":            "production",
		"app.kubernetes.io/name": "myapp",
	})

	child := &aimv1alpha1.AIMService{}

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: true,
			Match:   []string{"org.acme/*", "cost-center", "app.kubernetes.io/*"},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 3 {
		t.Errorf("expected 3 labels, got %d: %v", len(childLabels), childLabels)
	}
	if childLabels["org.acme/team"] != testTeamPlatform {
		t.Error("org.acme/team should be propagated")
	}
	if childLabels["cost-center"] != testCostCenterEng {
		t.Error("cost-center should be propagated")
	}
	if childLabels["app.kubernetes.io/name"] != "myapp" {
		t.Error("app.kubernetes.io/name should be propagated")
	}
	if _, exists := childLabels["environment"]; exists {
		t.Error("environment label should not be propagated")
	}
}

func TestPropagateLabels_PreservesExistingChildLabels(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{
		"team":        testTeamPlatform,
		"cost-center": testCostCenterEng,
	})

	child := &aimv1alpha1.AIMService{}
	child.SetLabels(map[string]string{
		"team":           "infrastructure", // Should not be overwritten
		"child-specific": "value",
	})

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: true,
			Match:   []string{"team", "cost-center"},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(childLabels))
	}
	if childLabels["team"] != "infrastructure" {
		t.Errorf("existing team label should not be overwritten, got %s", childLabels["team"])
	}
	if childLabels["cost-center"] != testCostCenterEng {
		t.Errorf("cost-center should be added, got %s", childLabels["cost-center"])
	}
	if childLabels["child-specific"] != "value" {
		t.Error("child-specific label should be preserved")
	}
}

func TestPropagateLabels_NoParentLabels(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}

	child := &aimv1alpha1.AIMService{}
	child.SetLabels(map[string]string{"existing": "value"})

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: true,
			Match:   []string{"team"},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 1 {
		t.Errorf("expected 1 label, got %d", len(childLabels))
	}
	if childLabels["existing"] != "value" {
		t.Error("existing label should be preserved")
	}
}

func TestPropagateLabels_EmptyMatchPatterns(t *testing.T) {
	parent := &aimv1alpha1.AIMService{}
	parent.SetLabels(map[string]string{"team": "platform"})

	child := &aimv1alpha1.AIMService{}

	config := &aimv1alpha1.AIMRuntimeConfigCommon{
		LabelPropagation: &aimv1alpha1.AIMRuntimeConfigLabelPropagationSpec{
			Enabled: true,
			Match:   []string{},
		},
	}

	PropagateLabels(parent, child, config)

	childLabels := child.GetLabels()
	if len(childLabels) != 0 {
		t.Errorf("expected no labels when match is empty, got %v", childLabels)
	}
}

func TestMatchesAnyPattern_ExactMatch(t *testing.T) {
	if !matchesAnyPattern("team", []string{"team", "cost-center"}) {
		t.Error("expected exact match for 'team'")
	}
}

func TestMatchesAnyPattern_WildcardMatch(t *testing.T) {
	if !matchesAnyPattern("org.acme/team", []string{"org.acme/*"}) {
		t.Error("expected wildcard match for 'org.acme/team'")
	}
}

func TestMatchesAnyPattern_NoMatch(t *testing.T) {
	if matchesAnyPattern("environment", []string{"team", "cost-center"}) {
		t.Error("expected no match for 'environment'")
	}
}

func TestMatchesAnyPattern_InvalidPattern(t *testing.T) {
	// filepath.Match treats [ as a special character, so this is an invalid pattern
	if matchesAnyPattern("team", []string{"team[invalid"}) {
		t.Error("expected no match for invalid pattern")
	}
}
