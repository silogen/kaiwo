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
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func TestFindMatchingTemplateForDerivedSpecNamespaceMatch(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("example-image")},
			Overrides: &aimv1alpha1.AIMServiceOverrides{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					Metric:    ptr.To(aimv1alpha1.AIMMetricThroughput),
					Precision: ptr.To(aimv1alpha1.AIMPrecisionFP16),
				},
			},
		},
	}

	baseSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			ModelName:         "example-image",
			RuntimeConfigName: "default",
			AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
				GpuSelector: &aimv1alpha1.AIMGpuSelector{
					Model: "mi300",
					Count: 1,
				},
			},
		},
	}

	existing := BuildDerivedTemplate(service, "existing-derived", "example-image", baseSpec)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	match, err := findMatchingTemplateForDerivedSpec(context.Background(), k8sClient, service, "example-image", baseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match == nil || match.NamespaceTemplate == nil {
		t.Fatalf("expected namespace template match, got %+v", match)
	}
	if match.NamespaceTemplate.Name != existing.Name {
		t.Fatalf("expected template name %q, got %q", existing.Name, match.NamespaceTemplate.Name)
	}
}

func TestFindMatchingTemplateForDerivedSpecClusterMatch(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("cluster-image")},
			Overrides: &aimv1alpha1.AIMServiceOverrides{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					Metric: ptr.To(aimv1alpha1.AIMMetricLatency),
				},
			},
		},
	}

	baseSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			ModelName:         "cluster-image",
			RuntimeConfigName: "default",
			AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
				Metric: ptr.To(aimv1alpha1.AIMMetricLatency),
			},
		},
	}

	clusterTemplate := &aimv1alpha1.AIMClusterServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-existing",
		},
		Spec: aimv1alpha1.AIMClusterServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: baseSpec.AIMServiceTemplateSpecCommon,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterTemplate).
		Build()

	match, err := findMatchingTemplateForDerivedSpec(context.Background(), k8sClient, service, "cluster-image", baseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match == nil || match.ClusterTemplate == nil {
		t.Fatalf("expected cluster template match, got %+v", match)
	}
	if match.ClusterTemplate.Name != clusterTemplate.Name {
		t.Fatalf("expected cluster template %q, got %q", clusterTemplate.Name, match.ClusterTemplate.Name)
	}
}

func TestFindMatchingTemplateForDerivedSpecNoMatch(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-match",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("original-image")},
			Overrides: &aimv1alpha1.AIMServiceOverrides{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					Precision: ptr.To(aimv1alpha1.AIMPrecisionBF16),
				},
			},
		},
	}

	baseSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			ModelName:         "original-image",
			RuntimeConfigName: "default",
		},
	}

	otherTemplate := &aimv1alpha1.AIMServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "different-spec",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
				ModelName:         "original-image",
				RuntimeConfigName: "default",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(otherTemplate).
		Build()

	match, err := findMatchingTemplateForDerivedSpec(context.Background(), k8sClient, service, "original-image", baseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match != nil {
		t.Fatalf("expected no match, got %+v", match)
	}
}

func TestResolveTemplateNameForServiceUnoptimizedFiltering(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add aim to scheme: %v", err)
	}

	// Create a model
	model := &aimv1alpha1.AIMModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMModelSpec{
			Image: "test-image:latest",
		},
		Status: aimv1alpha1.AIMModelStatus{
			Status: aimv1alpha1.AIMModelStatusReady,
		},
	}

	// Create an unoptimized template
	unoptimizedTemplate := &aimv1alpha1.AIMServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unoptimized-template",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
				ModelName:         "test-model",
				RuntimeConfigName: "default",
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					GpuSelector: &aimv1alpha1.AIMGpuSelector{
						Model: "MI300X",
						Count: 1,
					},
				},
			},
		},
		Status: aimv1alpha1.AIMServiceTemplateStatus{
			Status: aimv1alpha1.AIMTemplateStatusReady,
			Profile: aimv1alpha1.AIMProfile{
				Metadata: aimv1alpha1.AIMProfileMetadata{
					Type: aimv1alpha1.AIMProfileTypeUnoptimized,
					GPU:  "MI300X",
				},
			},
		},
	}

	// Create a service with allowUnoptimized=false
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("test-model")},
			Template: aimv1alpha1.AIMServiceTemplateConfig{
				AllowUnoptimized: false,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model, unoptimizedTemplate).
		Build()

	_, status, err := ResolveTemplateNameForService(context.Background(), k8sClient, service)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the error message mentions unoptimized templates were filtered
	expectedSubstring := "unoptimized template(s) were filtered out because spec.template.allowUnoptimized is false"
	if status.SelectionMessage == "" {
		t.Fatalf("expected error message, got empty string")
	}
	if !contains(status.SelectionMessage, expectedSubstring) {
		t.Fatalf("expected error message to contain %q, got %q", expectedSubstring, status.SelectionMessage)
	}

	if status.SelectionReason != aimv1alpha1.AIMServiceReasonTemplateNotFound {
		t.Fatalf("expected SelectionReason %q, got %q", aimv1alpha1.AIMServiceReasonTemplateNotFound, status.SelectionReason)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
